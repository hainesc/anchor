/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package config

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/types"
	"github.com/hainesc/anchor/pkg/runtime/k8s"
)

// OctopusConf represents the Octopus configuration.
type OctopusConf struct {
	types.NetConf
	Mode       string            `json:"mode"`
	MTU        int               `json:"mtu"`
	Octopus    map[string]string `json:"octopus"`
	Kubernetes k8s.Kubernetes    `json:"kubernetes"`
	Policy     k8s.Policy        `json:"policy"`
}

// IPAMConf represents the IPAM configuration.
type IPAMConf struct {
	Name string
	Type string `json:"type"`
	// etcd client
	Endpoints string `json:"etcd_endpoints"`
	// Used for k8s client
	Kubernetes k8s.Kubernetes `json:"kubernetes"`
	Policy     k8s.Policy     `json:"policy"`
	// etcd perm files
	CertFile      string   `json:"etcd_cert_file"`
	KeyFile       string   `json:"etcd_key_file"`
	TrustedCAFile string   `json:"etcd_ca_cert_file"`
	ServiceIPNet  string   `json:"service_ipnet"`
	NodeIPs       []string `json:"node_ips"`
	// Additional network config for pods
	Routes     []*types.Route `json:"routes,omitempty"`
	ResolvConf string         `json:"resolvConf,omitempty"`
}

// CNIConf represents the top-level network config.
type CNIConf struct {
	Name       string    `json:"name"`
	CNIVersion string    `json:"cniVersion"`
	Type       string    `json:"type"`
	Master     string    `json:"master"`
	IPAM       *IPAMConf `json:"ipam"`
}

// LoadOctopusConf loads config from bytes which read from config file for octopus.
func LoadOctopusConf(bytes []byte) (*OctopusConf, string, error) {
	n := &OctopusConf{}
	if err := json.Unmarshal(bytes, n); err != nil {
		return nil, "", fmt.Errorf("failed to load netconf: %v", err)
	}

	if n.Octopus == nil {
		return nil, "", fmt.Errorf(`"octopus" field is required. It specifies a list of interface names to virtualize`)
	}

	return n, n.CNIVersion, nil
}

// LoadIPAMConf loads config from bytes which read from config file for anchor.
func LoadIPAMConf(bytes []byte, envArgs string) (*IPAMConf, string, error) {
	n := CNIConf{}
	if err := json.Unmarshal(bytes, &n); err != nil {
		return nil, "", err
	}

	if n.IPAM == nil {
		return nil, "", fmt.Errorf("IPAM config missing 'ipam' key")
	}

	if n.IPAM.Endpoints == "" {
		return nil, "", fmt.Errorf("IPAM config missing 'etcd_endpoints' keys")
	}
	return n.IPAM, n.CNIVersion, nil
}
