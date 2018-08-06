/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package allocator

import (
	"github.com/containernetworking/cni/pkg/types/current"
)

// Allocator is the interface for allocator
type Allocator interface {
	CustomizeGateway(ret *current.Result) (*current.Result, error)
	CustomizeRoutes(ret *current.Result) (*current.Result, error)
	CustomizeDNS(ret *current.Result) (*current.Result, error)
	AddServiceRoute(ret *current.Result, serviceClusterIPRange string, nodeIPs []string) (*current.Result, error)

	Allocate(id string) (*current.IPConfig, error)
}

// Cleaner is the interface for cleaner
type Cleaner interface {
	Clean(id string) error
}
