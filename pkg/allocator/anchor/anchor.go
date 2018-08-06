/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package anchor

import (
	"fmt"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/hainesc/anchor/pkg/allocator"
	"github.com/hainesc/anchor/pkg/store"
	"net"
	"strings"
)

// Allocator is the allocator for anchor.
type Allocator struct {
	store      store.Store
	pod        string
	namespace  string
	customized map[string]string
	subnet     *net.IPNet
	gateway    net.IP
}

const (
	customizeGatewayKey = "cni.anchor.org/gateway"
	customizeRoutesKey  = "cni.anchor.org/routes"
	customizeSubnetKey  = "cni.anchor.org/subnet"
	customizeRangeKey   = "cni.anchor.org/range"
)

// AnchorAllocator implements the Allocator interface
var _ allocator.Allocator = &Allocator{}

// NewAllocator news a allocator
func NewAllocator(store store.Store,
	pod, namespace string,
	customized map[string]string) (*Allocator, error) {
	var subnet *net.IPNet
	var gw net.IP
	var err error // declaration here for avoid := which also create a new var
	if customized[customizeSubnetKey] != "" {
		_, subnet, err = net.ParseCIDR(customized[customizeSubnetKey])
		if err != nil {
			return nil, fmt.Errorf("invalid format of subnet in annotations")
		}
	}
	if customized[customizeRangeKey] != "" {
		// TODO: Caculate the subnet
		return nil, fmt.Errorf("customized range not implentmented")
	}

	if customized[customizeGatewayKey] != "" {
		gw = net.ParseIP(customized[customizeSubnetKey])
		if gw == nil {
			return nil, fmt.Errorf("invalid format of gateway in annotations")
		}
	} else {
		// Maybe no lock here is better.
		store.Lock()
		defer store.Unlock()
		gw = store.RetrieveGateway(subnet)
		if gw == nil {
			return nil, fmt.Errorf("failed to retrieve gateway for %s", subnet.String())
		}
	}

	if !subnet.Contains(gw) {
		return nil, fmt.Errorf("gateway %s not in network %s", gw.String(), subnet.String())
	}
	return &Allocator{
		store:      store,
		pod:        pod,
		namespace:  namespace,
		customized: customized,
		subnet:     subnet,
		gateway:    gw,
	}, nil
}

// CustomizeGateway adds default route for pod if customizeGatewayKey is set.
func (a *Allocator) CustomizeGateway(ret *current.Result) (*current.Result, error) {
	// We do nothing here because we has set gateway in func NewAllocator.
	/*
		if customizeGateway := net.ParseIP(a.customized[customizeGatewayKey]);
		customizeGateway != nil {
			ret.Routes = append(ret.Routes, &types.Route{
				Dst: net.IPNet{
					IP:   net.IPv4zero,
					Mask: net.IPv4Mask(0, 0, 0, 0),
				},
				GW: customizeGateway,
			})
			a.gatewayCustomized = true
		}
	*/
	return ret, nil
}

// CustomizeRoutes adds routes if customizeRouteKey is set.
// The format of input should as: 10.0.1.0/24,10.0.1.1;10.0.5.0/24,10.0.5.1
// The outer delimiter is semicolon(;) and the inner delimiter is comma(,)
func (a *Allocator) CustomizeRoutes(ret *current.Result) (*current.Result, error) {
	// Config route for default first
	ret.Routes = append(ret.Routes, &types.Route{
		Dst: net.IPNet{
			IP:   net.IPv4zero,
			Mask: net.IPv4Mask(0, 0, 0, 0),
		},
		GW: a.gateway,
	})

	if customizeRoute := a.customized[customizeRoutesKey]; customizeRoute != "" {
		routes := strings.Split(customizeRoute, ";")
		for _, r := range routes {
			_, dst, err := net.ParseCIDR(strings.Split(r, ",")[0])
			if err != nil {
				return nil, fmt.Errorf("invalid format of customized route in %s", r)
			}
			gw := net.ParseIP(strings.Split(r, ",")[1])
			if gw == nil {
				return nil, fmt.Errorf("invalid format of customized route in %s", r)
			}

			if !a.subnet.Contains(gw) {
				return nil, fmt.Errorf("gateway %s not in network %s", gw.String(), a.subnet.String())
			}
			ret.Routes = append(ret.Routes, &types.Route{
				Dst: *dst,
				GW:  gw,
			})
		}
	}

	return ret, nil
}

// CustomizeDNS configs the DNS for pod, but not implemented now.
func (a *Allocator) CustomizeDNS(ret *current.Result) (*current.Result, error) {
	// Recently, k8s does nothing even our result contains DNS info.
	// So we do nothing, just a function interface here.
	return ret, nil
}

// AddServiceRoute adds route for serice cluster ip range.
func (a *Allocator) AddServiceRoute(ret *current.Result,
	serviceClusterIPRange string,
	nodeIPs []string) (*current.Result, error) {
	if serviceClusterIPRange == "" {
		return ret, nil
	}
	_, dst, err := net.ParseCIDR(serviceClusterIPRange)
	if err != nil {
		return nil, fmt.Errorf("invalid format of service cluster ip range %s", serviceClusterIPRange)
	}

	for _, nodeIP := range nodeIPs {
		if a.subnet.Contains(net.ParseIP(nodeIP)) {
			ret.Routes = append(ret.Routes, &types.Route{
				Dst: *dst,
				GW:  net.ParseIP(nodeIP),
			})
			break
		}
		// If none of nodeIP contains in subnet, just break.
	}
	return ret, nil
}

// Allocate allocates IP for the pod.
func (a *Allocator) Allocate(id string) (*current.IPConfig, error) {
	a.store.Lock()
	defer a.store.Unlock()

	ips, err := a.store.RetrieveAllocated(a.namespace, a.subnet)
	if err != nil {
		return nil, err
	}
	for _, ipRange := range *ips {
		ipRange.Gateway = a.gateway
		if err = ipRange.Canonicalize(); err != nil {
			return nil, err
		}
	}
	used, err := a.store.RetrieveUsed(a.namespace, a.subnet)
	if err != nil {
		return nil, err
	}

	for _, usedRange := range *used {
		usedRange.Gateway = a.gateway
		if err = usedRange.Canonicalize(); err != nil {
			return nil, err
		}
	}
	for _, r := range *ips {
		var iter net.IP
		for iter = r.RangeStart; !iter.Equal(ip.NextIP(r.RangeEnd)); iter = ip.NextIP(iter) {

			avail := true
			for _, usedRange := range *used {
				var u net.IP
				for u = usedRange.RangeStart; !u.Equal(ip.NextIP(usedRange.RangeEnd)); u = ip.NextIP(u) {
					if iter.Equal(u) {
						avail = false
						break
					}
				}
			}
			if avail {
				// Get subnet and gateway information
				if err != nil {
					// TODO: check
					continue
				}
				if iter.Equal(a.gateway) {
					// TODO: check
					continue
				}
				// TODO:

				controllerName := a.customized["cni.anchor.org/controller"]
				if controllerName == "" {
					controllerName = "unknown"
				}
				_, err = a.store.Reserve(id, iter, a.pod, a.namespace, controllerName)
				if err != nil {
					continue
				}

				return &current.IPConfig{
					Version: "4",
					Address: net.IPNet{IP: iter, Mask: a.subnet.Mask},
					Gateway: a.gateway,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("can not allcate IP for pod named, %s", a.pod)
}

// Cleaner is the cleaner for anchor.
type Cleaner struct {
	store     store.Store
	pod       string
	namespace string
}

// AnchorCleaner implements the Cleaner interface
var _ allocator.Cleaner = &Cleaner{}

// NewCleaner news a cleaner for anchor.
func NewCleaner(store store.Store, pod, namespace string) (*Cleaner, error) {
	return &Cleaner{
		store:     store,
		pod:       pod,
		namespace: namespace,
	}, nil
}

// Clean cleans the IP for the pod.
func (a *Cleaner) Clean(id string) error {
	a.store.Lock()
	defer a.store.Unlock()
	return a.store.Release(id)
}
