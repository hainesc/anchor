/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package etcd

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/hainesc/anchor/pkg/store"
	"github.com/hainesc/anchor/pkg/utils"
)

const (
	ipsPrefix     = "/anchor/cn/"
	gatewayPrefix = "/anchor/gw/"
	userPrefix    = "/anchor/ns/"
	lockKey       = "/anchor/lock"
)

// Etcd is a simple etcd-backed store
type Etcd struct {
	mutex *concurrency.Mutex
	kv    clientv3.KV
}

// Store implements the Store interface
var _ store.Store = &Etcd{}

// NewEtcdClient news a etcd client
func NewEtcdClient(network string, endPoints []string, tlsConfig *tls.Config) (*Etcd, error) {
	// We don't check the config here since clientv3 will do.
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endPoints,
		DialTimeout: 5 * time.Second,
		TLS:         tlsConfig,
	})
	if err != nil {
		return nil, err
	}

	session, err := concurrency.NewSession(cli)
	if err != nil {
		return nil, err
	}

	mutex := concurrency.NewMutex(session, lockKey)
	kv := clientv3.NewKV(cli)
	return &Etcd{mutex, kv}, nil
}

// NewEtcdClientWithoutSSl news a etcd client without ssl
func NewEtcdClientWithoutSSl(network string, endPoints []string) (*Etcd, error) {
	// We don't check the config here since clientv3 will do.
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endPoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	session, err := concurrency.NewSession(cli)
	if err != nil {
		return nil, err
	}

	mutex := concurrency.NewMutex(session, lockKey)
	kv := clientv3.NewKV(cli)
	return &Etcd{mutex, kv}, nil
}

// Lock locks the store
func (e *Etcd) Lock() error {
	return e.mutex.Lock(context.TODO())
}

// Unlock unlocks the store
func (e *Etcd) Unlock() error {
	return e.mutex.Unlock(context.TODO())
}

// Close closes the store
func (e *Etcd) Close() error {
	return nil
	// return s.Unlock()
}

// RetrieveGateway retrieves gateway for subnet.
func (e *Etcd) RetrieveGateway(subnet *net.IPNet) net.IP {
	resp, err := e.kv.Get(context.TODO(), gatewayPrefix + subnet.String())
	if err != nil || len(resp.Kvs) == 0 {
		return nil
	}
	return net.ParseIP(string(resp.Kvs[0].Value))
}

// RetrieveAllocated retrieves allocated IPs in subnet for namespace.
func (e *Etcd) RetrieveAllocated(namespace string, subnet *net.IPNet) (*utils.RangeSet, error) {
	resp, err := e.kv.Get(context.TODO(), userPrefix + namespace)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("no IP allocated for %s found in etcd", namespace)
	}
	// TODO:
	ret := utils.RangeSet{}
	return ret.Concat(string(resp.Kvs[0].Value), subnet)

}

// RetrieveUsed retrieves used IP in subnet for namespace.
func (e *Etcd) RetrieveUsed(namespace string, subnet *net.IPNet) (*utils.RangeSet, error) {
	resp, err := e.kv.Get(context.TODO(), ipsPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	// TODO: which return type is best? []string or RangeSet?
	// ret := make([]net.IP, 0)
	s := make([]string, 0)
	for _, item := range resp.Kvs {
		row := strings.Split(string(item.Value), ",")
		if row[2] == namespace {
			// ret = append(ret, net.ParseIP(row[0]))
			s = append(s, row[0])
		}
	}
	ret := utils.RangeSet{}
	// TODO:
	return ret.Concat(strings.Join(s, ","), subnet)
}

// Reserve writes the result to the store.
func (e *Etcd) Reserve(id string, ip net.IP, podName string, podNamespace string, controllerName string) (bool, error) {
	// TODO: lock
	if _, err := e.kv.Put(context.TODO(), ipsPrefix + id,
		ip.String() + "," + podName + "," + podNamespace + "," + controllerName); err != nil {
		return false, nil
	}

	return true, nil
}

// Release releases the IP which allocated to the container identified by id.
func (e *Etcd) Release(id string) error {
	_, err := e.kv.Delete(context.TODO(), ipsPrefix + id)
	return err
}

// GatewayMap is the map of subnet and gateway, used by monkey
type GatewayMap struct {
	Subnet  string `json:"subnet"`
	Gateway string `json:"gw"`
}

// AllocateMap is the map of dedicated IPs and the namespace, used by monkey
type AllocateMap struct {
	Allocate  string `json:"ips"`
	Namespace string `json:"ns"`
	// TODO: Add label to support explicate which IPs to use for a given pods.
	// Label     string `json:"label"`
}

// InUsedMap is the map of ContainerID and its IP, used by monkey
type InUsedMap struct {
	ContainerID string `json:"id"`
	IP          net.IP `json:"ip"`
	Pod         string `json:"pod"`
	Namespace   string `json:"ns"`
	App         string `json:"app,omitempty"`
	Service     string `json:"svc,omitempty"`
}

// AllGatewayMap gets all gateway map in the store
func (e *Etcd) AllGatewayMap() (*[]GatewayMap, error) {
	gms := make([]GatewayMap, 0)
	resp, err := e.kv.Get(context.TODO(), gatewayPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	// s := make([]string, 0)
	for _, item := range resp.Kvs {
		subnet := strings.TrimPrefix(string(item.Key), gatewayPrefix)
		gw := string(item.Value)
		_, _, err := net.ParseCIDR(subnet)
		if err != nil {
			// ivalid format, just omit.
			continue
		}

		if net.ParseIP(gw) == nil {
			// ivalid format, just omit.
			continue
		}
		gms = append(gms, GatewayMap{
			// Subnet:  *subnet,
			// Gateway: gateway,
			Subnet:  subnet,
			Gateway: gw,
		})
	}
	return &gms, nil
}

// InsertGatewayMap inserts a gateway map
func (e *Etcd) InsertGatewayMap(gm GatewayMap) error {
	if _, err := e.kv.Put(context.TODO(), gatewayPrefix+gm.Subnet, gm.Gateway); err != nil {

		return err
	}
	return nil
}

// DeleteGatewayMap deletes a gateway map
func (e *Etcd) DeleteGatewayMap(gms []GatewayMap) error {
	for _, gm := range gms {
		// if _, err := e.kv.Delete(context.TODO(), gm.Subnet.String()); err != nil {
		if _, err := e.kv.Delete(context.TODO(), gatewayPrefix+gm.Subnet); err != nil {
			// TODO: error when delete one item, should we just stop and return error?
			// If we omit one error, maybe all errors are omitted.
			return err
		}
	}
	return nil
}

// RetrieveUsedbyNamespace retrieves used IP in subnet for namespace.
func (e *Etcd) RetrieveUsedbyNamespace(namespace string, adminRole bool) (*[]string, error) {
	resp, err := e.kv.Get(context.TODO(), ipsPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	// TODO: which return type is best? []string or RangeSet?
	// ret := make([]net.IP, 0)
	s := make([]string, 0)
	if adminRole {
		for _, item := range resp.Kvs {
			s = append(s, string(item.Value))
		}
	} else {
		for _, item := range resp.Kvs {
			row := strings.Split(string(item.Value), ",")
			if row[2] == namespace {
				s = append(s, string(item.Value))
			}
		}
	}
	return &s, nil
}

// AllAllocate gets all allocate map
func (e *Etcd) AllAllocate() (*[]AllocateMap, error) {
	ams := make([]AllocateMap, 0)
	resp, err := e.kv.Get(context.TODO(), userPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	// s := make([]string, 0)
	for _, item := range resp.Kvs {
		ns := strings.TrimPrefix(string(item.Key), userPrefix)
		allocate := string(item.Value)
		ams = append(ams, AllocateMap{
			// Subnet:  *subnet,
			// Gateway: gateway,
			Allocate:  allocate,
			Namespace: ns,
		})
	}
	return &ams, nil

}

// InsertAllocateMap inserts a allocate map
func (e *Etcd) InsertAllocateMap(am AllocateMap) error {
	if _, err := e.kv.Put(context.TODO(), userPrefix+am.Namespace, am.Allocate); err != nil {

		return err
	}
	return nil
}

// DeleteAllocateMap deletes a allocate map
func (e *Etcd) DeleteAllocateMap(ams []AllocateMap) error {
	for _, am := range ams {
		// if _, err := e.kv.Delete(context.TODO(), gm.Subnet.String()); err != nil {
		if _, err := e.kv.Delete(context.TODO(), userPrefix+am.Namespace); err != nil {
			// TODO: error when delete one item, should we just stop and return error?
			// If we omit one error, maybe all errors are omitted.
			return err
		}
	}
	return nil
}
