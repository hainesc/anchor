/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package app

import (
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"

	"github.com/coreos/etcd/pkg/transport"

	"github.com/hainesc/anchor/internal/pkg/config"
	"github.com/hainesc/anchor/pkg/allocator/anchor"
	"github.com/hainesc/anchor/pkg/runtime/k8s"
	"github.com/hainesc/anchor/pkg/store/etcd"
)

// CmdAdd allocates IP for pod
func CmdAdd(args *skel.CmdArgs) error {
	ipamConf, confVersion, err := config.LoadIPAMConf(args.StdinData, args.Args)
	if err != nil { // Error in config file.
		return err
	}

	alloc, err := newAllocator(args, ipamConf)
	if err != nil { // Error during init Allocator
		return err
	}

	// Init result here, which will be printed in json format.
	result := &current.Result{}

	// Handle customized network configurations.
	if result, err = alloc.CustomizeGateway(result); err != nil {
		return err
	}
	if result, err = alloc.CustomizeRoutes(result); err != nil {
		return err
	}
	if result, err = alloc.CustomizeDNS(result); err != nil {
		return err
	}
	// Add an item of route:
	//   service-cluster-ip-range -> Node IP
	if result, err = alloc.AddServiceRoute(result, ipamConf.ServiceIPNet, ipamConf.NodeIPs); err != nil {
		return err
	}

	ipConf, err := alloc.Allocate(args.ContainerID)
	if err != nil {
		return err
	}
	result.IPs = append(result.IPs, ipConf)
	return types.PrintResult(result, confVersion)
}

// CmdDel deletes IP for pod
func CmdDel(args *skel.CmdArgs) error {
	ipamConf, _, err := config.LoadIPAMConf(args.StdinData, args.Args)
	if err != nil { // Error in config file.
		return err
	}

	cleaner, err := newCleaner(args, ipamConf)
	return cleaner.Clean(args.ContainerID)
}

func newAllocator(args *skel.CmdArgs, conf *config.IPAMConf) (*anchor.Allocator, error) {
	tlsInfo := &transport.TLSInfo{
		CertFile:      conf.CertFile,
		KeyFile:       conf.KeyFile,
		TrustedCAFile: conf.TrustedCAFile,
	}
	tlsConfig, _ := tlsInfo.ClientConfig()
	// Use etcd as store
	store, err := etcd.NewEtcdClient(conf.Name,
		strings.Split(conf.Endpoints, ","),
		tlsConfig)
	defer store.Close()
	if err != nil {
		return nil, err
	}

	// 1. Get K8S_POD_NAME and K8S_POD_NAMESPACE.
	k8sArgs := k8s.Args{}
	if err := types.LoadArgs(args.Args, &k8sArgs); err != nil {
		return nil, err
	}

	// 2. Get conf for k8s client and create a k8s_client
	runtime, err := k8s.NewK8sClient(conf.Kubernetes, conf.Policy)
	if err != nil {
		return nil, err
	}

	// 3. Get annotations from k8s_client via K8S_POD_NAME and K8S_POD_NAMESPACE.
	label, annot, err := k8s.GetK8sPodInfo(runtime, string(k8sArgs.PodName),
		string(k8sArgs.PodNamespace))
	if err != nil {
		return nil, err
	}

	customized := make(map[string]string)
	for k, v := range label {
		customized[k] = v
	}
	for k, v := range annot {
		customized[k] = v
	}

	// It is friendly to show which controller the pods controled by.
	// TODO: maybe it is meaningless. the pod name starts with the controller name.
	controllerName, _ := k8s.ResourceControllerName(runtime, string(k8sArgs.PodName), string(k8sArgs.PodNamespace))
	if controllerName != "" {
		customized["cni.anchor.org/controller"] = controllerName
	}

	return anchor.NewAllocator(store, string(k8sArgs.PodName),
		string(k8sArgs.PodNamespace), customized)
}

func newCleaner(args *skel.CmdArgs, ipamConf *config.IPAMConf) (*anchor.Cleaner, error) {
	tlsInfo := &transport.TLSInfo{
		CertFile:      ipamConf.CertFile,
		KeyFile:       ipamConf.KeyFile,
		TrustedCAFile: ipamConf.TrustedCAFile,
	}
	tlsConfig, _ := tlsInfo.ClientConfig()

	store, err := etcd.NewEtcdClient(ipamConf.Name,
		strings.Split(ipamConf.Endpoints, ","), tlsConfig)
	defer store.Close()
	if err != nil {
		return nil, err
	}

	// Read pod name and namespace from args
	k8sArgs := k8s.Args{}
	if err := types.LoadArgs(args.Args, &k8sArgs); err != nil {
		return nil, err
	}

	return anchor.NewCleaner(store,
		string(k8sArgs.PodName),
		string(k8sArgs.PodNamespace))
}
