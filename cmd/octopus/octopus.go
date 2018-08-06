/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package main

import (
	"errors"
	"fmt"
	"net"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"

	"github.com/hainesc/anchor/internal/app"
	"github.com/hainesc/anchor/internal/pkg/config"
	"github.com/hainesc/anchor/pkg/runtime/k8s"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"
)

const (
	// IPv4InterfaceArpProxySysctlTemplate represents a template for format output.
	IPv4InterfaceArpProxySysctlTemplate = "net.ipv4.conf.%s.proxy_arp"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func createMacvlan(conf *config.OctopusConf, ifName string, netns ns.NetNS, master string) (*current.Interface, error) {
	macvlan := &current.Interface{}

	mode, err := app.ModeFromString(conf.Mode)
	if err != nil {
		return nil, err
	}

	m, err := netlink.LinkByName(master)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup master %q: %v", master, err)
	}

	// due to kernel bug we have to create with tmpName or it might
	// collide with the name on the host and error out
	tmpName, err := ip.RandomVethName()
	if err != nil {
		return nil, err
	}

	mv := &netlink.Macvlan{
		LinkAttrs: netlink.LinkAttrs{
			MTU:         conf.MTU,
			Name:        tmpName,
			ParentIndex: m.Attrs().Index,
			Namespace:   netlink.NsFd(int(netns.Fd())),
		},
		Mode: mode,
	}

	if err := netlink.LinkAdd(mv); err != nil {
		return nil, fmt.Errorf("failed to create macvlan: %v", err)
	}

	err = netns.Do(func(_ ns.NetNS) error {
		// TODO: duplicate following lines for ipv6 support, when it will be added in other places
		ipv4SysctlValueName := fmt.Sprintf(IPv4InterfaceArpProxySysctlTemplate, tmpName)
		if _, err := sysctl.Sysctl(ipv4SysctlValueName, "1"); err != nil {
			// remove the newly added link and ignore errors, because we already are in a failed state
			_ = netlink.LinkDel(mv)
			return fmt.Errorf("failed to set proxy_arp on newly added interface %q: %v", tmpName, err)
		}

		err := ip.RenameLink(tmpName, ifName)
		if err != nil {
			_ = netlink.LinkDel(mv)
			return fmt.Errorf("failed to rename macvlan to %q: %v", ifName, err)
		}
		macvlan.Name = ifName

		// Re-fetch macvlan to get all properties/attributes
		contMacvlan, err := netlink.LinkByName(ifName)
		if err != nil {
			return fmt.Errorf("failed to refetch macvlan %q: %v", ifName, err)
		}
		macvlan.Mac = contMacvlan.Attrs().HardwareAddr.String()
		macvlan.Sandbox = netns.Path()

		return nil
	})
	if err != nil {
		return nil, err
	}

	return macvlan, nil
}

func cmdAdd(args *skel.CmdArgs) error {
	// n, cniVersion, err := loadConf(args.StdinData)
	n, cniVersion, err := config.LoadOctopusConf(args.StdinData)
	if err != nil {
		return err
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", netns, err)
	}
	defer netns.Close()

	// Get annotations of the pod and decide which host interface will be used.
	// 1. Get conf for k8s client and create a k8s_client
	k8sClient, err := k8s.NewK8sClient(n.Kubernetes, n.Policy)
	if err != nil {
		return err
	}

	// 2. Get K8S_POD_NAME and K8S_POD_NAMESPACE.
	k8sArgs := k8s.Args{}
	if err := types.LoadArgs(args.Args, &k8sArgs); err != nil {
		return err
	}

	// 3. Get annotations from k8s_client via K8S_POD_NAME and K8S_POD_NAMESPACE.
	_, annot, err := k8s.GetK8sPodInfo(k8sClient, string(k8sArgs.PodName), string(k8sArgs.PodNamespace))
	if err != nil {
		return fmt.Errorf("failed to read annotaions for pod " + err.Error())
	}
	subnet := annot["cni.anchor.org/subnet"]

	master := n.Octopus[subnet]
	if master == "" {
		if subnet == "" {
			return fmt.Errorf("failed to find annotation named cni.anchor.org/subnet")
		}
		return fmt.Errorf("Master interface not found for VLAN %s on this node", subnet)
	}
	macvlanInterface, err := createMacvlan(n, args.IfName, netns, master)
	if err != nil {
		return err
	}

	// Delete link if err to avoid link leak in this ns
	defer func() {
		if err != nil {
			netns.Do(func(_ ns.NetNS) error {
				return ip.DelLinkByName(args.IfName)
			})
		}
	}()

	// run the IPAM plugin and get back the config to apply
	r, err := ipam.ExecAdd(n.IPAM.Type, args.StdinData)
	if err != nil {
		return err
	}

	// Invoke ipam del if err to avoid ip leak
	defer func() {
		if err != nil {
			ipam.ExecDel(n.IPAM.Type, args.StdinData)
		}
	}()

	// Convert whatever the IPAM result was into the current Result type
	result, err := current.NewResultFromResult(r)
	if err != nil {
		return err
	}

	if len(result.IPs) == 0 {
		return errors.New("IPAM plugin returned missing IP config")
	}
	result.Interfaces = []*current.Interface{macvlanInterface}

	for _, ipc := range result.IPs {
		// All addresses apply to the container macvlan interface
		ipc.Interface = current.Int(0)
	}

	err = netns.Do(func(_ ns.NetNS) error {
		if err := ipam.ConfigureIface(args.IfName, result); err != nil {
			return err
		}

		contVeth, err := net.InterfaceByName(args.IfName)
		if err != nil {
			return fmt.Errorf("failed to look up %q: %v", args.IfName, err)
		}

		for _, ipc := range result.IPs {
			if ipc.Version == "4" {
				_ = arping.GratuitousArpOverIface(ipc.Address.IP, *contVeth)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	result.DNS = n.DNS

	return types.PrintResult(result, cniVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	n, _, err := config.LoadOctopusConf(args.StdinData)
	if err != nil {
		return err
	}

	err = ipam.ExecDel(n.IPAM.Type, args.StdinData)
	if err != nil {
		return err
	}

	if args.Netns == "" {
		return nil
	}

	// There is a netns so try to clean up. Delete can be called multiple times
	// so don't return an error if the device is already removed.
	err = ns.WithNetNSPath(args.Netns, func(_ ns.NetNS) error {
		if err := ip.DelLinkByName(args.IfName); err != nil {
			if err != ip.ErrLinkNotFound {
				return err
			}
		}
		return nil
	})

	return err
}

func main() {
	skel.PluginMain(cmdAdd, cmdDel, version.All)
}
