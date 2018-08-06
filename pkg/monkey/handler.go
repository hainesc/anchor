/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package monkey

import (
	"encoding/json"
	"github.com/hainesc/anchor/pkg/store/etcd"
	"log"
	"net/http"
	"strings"
)

// InUseHandler handlers the get request from front end and returns IPs in use.
// TODO: It should has a member named user_namespace which can filter the result.
type InUseHandler struct {
	etcd *etcd.Etcd
}

// NewInUseHandler news an InUsedHandler
func NewInUseHandler(etcd *etcd.Etcd) *InUseHandler {
	return &InUseHandler{
		etcd: etcd,
	}
}

// GatewayHandler handles the request for gateway information
type GatewayHandler struct {
	etcd *etcd.Etcd
}

// NewGatewayHandler news a GatewayHandler
func NewGatewayHandler(etcd *etcd.Etcd) *GatewayHandler {
	return &GatewayHandler{
		etcd: etcd,
	}
}

// AllocateHandler handles the request for allocate information
type AllocateHandler struct {
	etcd *etcd.Etcd
}

// NewAllocateHandler news a Allocator handler
func NewAllocateHandler(etcd *etcd.Etcd) *AllocateHandler {
	return &AllocateHandler{
		etcd: etcd,
	}
}

// ServeHTTP serves http
func (h *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Serve the resource.
		// TODO: just for test.
		log.Printf("receive a get mothod with parameter: ")
		gws, _ := h.etcd.AllGatewayMap()
		response, _ := json.Marshal(gws)
		// TODO: if error.
		w.Write(response)
	case http.MethodPost:
		// Create a new record.
		// curl -X POST -d "{\"subnet\": \"10.2.1.0/24\", \"gw\": \"10.2.1.1\"}" http://localhost:3000/api/v1/gateway
		log.Printf("receive a post mothod with parameter: ")
		// gms := make([]etcd.GatewayMap, 0)
		var gm etcd.GatewayMap
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&gm)
		if err != nil {
			// TODO:
			http.Error(w, "Invalid parameter.", 405)
		}

		// TODO: valid the input.
		// TODO: check if exists.
		log.Printf("%s: %s", gm.Subnet, gm.Gateway)

		err = h.etcd.InsertGatewayMap(gm)
		if err != nil {
			log.Printf(err.Error())
		}

	case http.MethodPut:
		// Update an existing record.
		log.Printf("receive a post mothod with parameter: ")
	case http.MethodDelete:
		// Remove the record.
		// curl -X DELETE -d "[{\"subnet\": \"10.2.1.0/24\", \"gw\": \"10.2.1.1\"}]" http://localhost:3000/api/v1/gateway
		log.Printf("receive a delete mothod with parameter: ")
		gms := make([]etcd.GatewayMap, 0)
		// var gms etcd.GatewayMap
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&gms)
		if err != nil {
			// TODO:
			http.Error(w, "Invalid parameter.", 405)
		}
		for _, gm := range gms {
			log.Printf("%s: %s", gm.Subnet, gm.Gateway)

		}
		err = h.etcd.DeleteGatewayMap(gms)
		if err != nil {
			log.Printf(err.Error())
		}
	// TODO: recently, angular delete method does not support body parameter, but it is in process. So we just use patch here. see: https://github.com/angular/angular/issues/19438
	// Remove this case when the support is done.
	case http.MethodPatch:
		// Remove the record.
		// curl -X DELETE -d "[{\"subnet\": \"10.2.1.0/24\", \"gw\": \"10.2.1.1\"}]" http://localhost:3000/api/v1/gateway
		log.Printf("receive a delete mothod with parameter: ")
		gms := make([]etcd.GatewayMap, 0)
		// var gms etcd.GatewayMap
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&gms)
		if err != nil {
			// TODO:
			http.Error(w, "Invalid parameter.", 405)
		}
		for _, gm := range gms {
			log.Printf("%s: %s", gm.Subnet, gm.Gateway)

		}
		err = h.etcd.DeleteGatewayMap(gms)
		if err != nil {
			log.Printf(err.Error())
		}
	default:
		// Give an error message.
		http.Error(w, "Invalid request method.", 405)
	}
}

// ServeHTTP serves http
func (h *AllocateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Serve the resource.
		// TODO: just for test.
		log.Printf("receive a get mothod with parameter: ")
		ams, _ := h.etcd.AllAllocate()
		response, _ := json.Marshal(ams)
		// TODO: if error.
		w.Write(response)
	case http.MethodPost:
		// Create a new record.
		log.Printf("receive a post mothod with parameter: ")
		// gms := make([]etcd.GatewayMap, 0)
		var am etcd.AllocateMap
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&am)
		if err != nil {
			// TODO:
			http.Error(w, "Invalid parameter.", 405)
		}

		// TODO: valid the input.
		// TODO: check if exists.
		log.Printf("%s: %s", am.Namespace, am.Allocate)

		err = h.etcd.InsertAllocateMap(am)
		if err != nil {
			log.Printf(err.Error())
		}

	case http.MethodPut:
		// Update an existing record.
		log.Printf("receive a post mothod with parameter: ")
	case http.MethodDelete:
		// Remove the record.
		log.Printf("receive a delete mothod with parameter: ")
		ams := make([]etcd.AllocateMap, 0)
		// var gms etcd.GatewayMap
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&ams)
		if err != nil {
			// TODO:
			http.Error(w, "Invalid parameter.", 505)
		}
		for _, am := range ams {
			log.Printf("%s: %s", am.Namespace, am.Allocate)

		}
		err = h.etcd.DeleteAllocateMap(ams)
		if err != nil {
			log.Printf(err.Error())
		}
	// TODO: recently, angular delete method does not support body parameter, but it is in process. So we just use patch here. see: https://github.com/angular/angular/issues/19438
	// Remove this case when the support is done.
	case http.MethodPatch:
		// Remove the record.
		log.Printf("receive a delete mothod with parameter: ")
		ams := make([]etcd.AllocateMap, 0)
		// var gms etcd.GatewayMap
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&ams)
		if err != nil {
			// TODO:
			http.Error(w, "Invalid parameter.", 505)
		}
		for _, am := range ams {
			log.Printf("%s: %s", am.Namespace, am.Allocate)

		}
		err = h.etcd.DeleteAllocateMap(ams)
		if err != nil {
			log.Printf(err.Error())
		}
	default:
		// Give an error message.
		http.Error(w, "Invalid request method.", 405)
	}
}

// TODO:
type ips struct {
	IP         string `json:"ip"`
	Pod        string `json:"pod"`
	Namespace  string `json:"ns"`
	Controller string `json:"ctrl"`
	// App string `json:"app"`
	// Service string `json:"svc"`
}

// TODO:
type gateway struct {
	Subnet string `json:"subnet"`
	Dst    string `json:"dst"`
}

// ServeHTTP serves http
func (h *InUseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	empty := []byte{}
	w.Header().Set("Content-Type", "application/json")
	// namespace := r.Header.Get("x-dce-tenant")
	namespace := "default"
	if namespace == "" {
		w.Write(empty)
		return
	}

	result := []ips{}
	// TODO: check header and find whether is admin role.
	ipsInArray, err := h.etcd.RetrieveUsedbyNamespace("default", true)
	if err != nil {
		// TODO: if err, we return an empty here.
		w.Write(empty)
	}

	for _, r := range *ipsInArray {
		parts := strings.Split(r, ",")
		result = append(result, ips{
			IP:         parts[0],
			Pod:        parts[1],
			Namespace:  parts[2],
			Controller: parts[4],
		})
	}
	resultInJSON, err := json.Marshal(result)
	if err != nil {
		log.Printf("empty, ", err.Error())
		w.Write(empty)
	}
	log.Printf("empty, ", string(resultInJSON))
	w.Write(resultInJSON)
}
