/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package k8s

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// NewK8sClient creates a k8s client
func NewK8sClient(kuber Kubernetes, policy Policy) (*kubernetes.Clientset, error) {
	// Some config can be passed in a kubeconfig file
	kubeconfig := kuber.Kubeconfig
	// Config can be overridden by config passed in explicitly in the network config.
	configOverrides := &clientcmd.ConfigOverrides{}

	// If an API root is given, make sure we're using using the name / port rather than
	// the full URL. Earlier versions of the config required the full `/api/v1/` extension,
	// so split that off to ensure compatibility.
	policy.K8sAPIRoot = strings.Split(policy.K8sAPIRoot, "/api/")[0]

	var overridesMap = []struct {
		variable *string
		value    string
	}{
		{&configOverrides.ClusterInfo.Server, policy.K8sAPIRoot},
		{&configOverrides.AuthInfo.ClientCertificate, policy.K8sClientCertificate},
		{&configOverrides.AuthInfo.ClientKey, policy.K8sClientKey},
		{&configOverrides.ClusterInfo.CertificateAuthority, policy.K8sCertificateAuthority},
		{&configOverrides.AuthInfo.Token, policy.K8sAuthToken},
	}

	// Using the override map above, populate any non-empty values.
	for _, override := range overridesMap {
		if override.value != "" {
			*override.variable = override.value
		}
	}

	// Also allow the K8sAPIRoot to appear under the "kubernetes" block in the network config.
	if kuber.K8sAPIRoot != "" {
		configOverrides.ClusterInfo.Server = kuber.K8sAPIRoot
	}

	// Use the kubernetes client code to load the kubeconfig file and combine it with the overrides.
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		configOverrides).ClientConfig()
	if err != nil {
		return nil, err
	}

	// Create the clientset
	return kubernetes.NewForConfig(config)
}

// GetK8sPodInfo gets the labels and annotations of the pod
func GetK8sPodInfo(client *kubernetes.Clientset, podName, podNamespace string) (labels map[string]string, annotations map[string]string, err error) {
	pod, err := client.CoreV1().Pods(string(podNamespace)).Get(podName, v1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}
	return pod.Labels, pod.Annotations, nil
}

// ResourceControllerName gets the name of ResourceController based on given reference.
func ResourceControllerName(client *kubernetes.Clientset, podName, namespace string) (
	string, error) {
	pod, err := client.CoreV1().Pods(string(namespace)).Get(podName, v1.GetOptions{})
	if err != nil {
		return "", err
	}

	for _, ref := range pod.OwnerReferences {
		if *ref.Controller {
			if strings.ToLower(ref.Kind) == "replicaset" {
				rs, err := client.AppsV1beta2().ReplicaSets(namespace).Get(ref.Name, v1.GetOptions{})
				if err != nil {
					return "", err
				}
				for _, r := range rs.OwnerReferences {
					if *r.Controller {
						return r.Name, nil
					}
				}
				return ref.Name, nil
			}
			return ref.Name, nil
		}
	}
	return "", fmt.Errorf("The pod %s has no controller", podName)
}
