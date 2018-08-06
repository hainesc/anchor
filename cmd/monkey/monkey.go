/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package main

import (
	// "os"
	"github.com/coreos/etcd/pkg/transport"
	"github.com/hainesc/anchor/pkg/monkey"
	"github.com/hainesc/anchor/pkg/store/etcd"
	"log"
	"net/http"
	"strings"
)

func main() {
	// TODO: we should support read conf from env, then we can run it as a container and pass the conf via Environment.
	conf, err := monkey.LoadConf(".", "monkey.conf")
	if err != nil {
		log.Fatal(err.Error())
	}
	var store *etcd.Etcd
	if strings.Contains(conf.Endpoints, "https://") {
		tlsInfo := &transport.TLSInfo{
			CertFile:      conf.CertFile,
			KeyFile:       conf.KeyFile,
			TrustedCAFile: conf.TrustedCAFile,
		}
		tlsConfig, _ := tlsInfo.ClientConfig()
		store, err = etcd.NewEtcdClient("monkey", strings.Split(conf.Endpoints, ","), tlsConfig)
	} else {
		store, err = etcd.NewEtcdClientWithoutSSl("monkey", strings.Split(conf.Endpoints, ","))
	}
	defer store.Close()

	if err != nil {
		log.Fatal("Failed to connect to etcd, ", err.Error())
	}

	http.Handle("/", http.FileServer(http.Dir("./powder")))
	http.Handle("/api/v1/binding", monkey.NewInUseHandler(store))
	http.Handle("/api/v1/gateway", monkey.NewGatewayHandler(store))
	http.Handle("/api/v1/allocate", monkey.NewAllocateHandler(store))
	http.ListenAndServe(":8964", nil)
}
