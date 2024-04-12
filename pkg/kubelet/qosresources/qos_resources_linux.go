//go:build linux
// +build linux

/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package qosresources

import (
	libcontainercgroups "github.com/opencontainers/runc/libcontainer/cgroups"

	v1 "k8s.io/api/core/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/kubernetes/pkg/features"
)

// GetAvailableQOSResources returns all the Kubernetes-managed QoS resources available on the node.
func GetAvailableQOSResources() v1.QOSResourceStatus {
	q := v1.QOSResourceStatus{}

	if libcontainercgroups.IsCgroup2UnifiedMode() {
		q.ContainerQOSResources = append(q.ContainerQOSResources,
			v1.QOSResourceInfo{
				Name: v1.QOSResourceOOMGroupKill,
				Classes: []v1.QOSResourceClassInfo{
					{
						Name: v1.QOSResourceClassOOMGroupKillDisabled,
					},
					{
						Name: v1.QOSResourceClassOOMGroupKillEnabled,
					},
				},
			})
	}
	return q
}

// GetContainerQOSResourceClass is a feature gate wrapper for getting the class of a single QoS resource
func GetContainerQOSResourceClass(c *v1.Container, name v1.QOSResourceName) string {
	if utilfeature.DefaultFeatureGate.Enabled(features.QOSResources) {
		for _, r := range c.Resources.QOSResources {
			if r.Name == name {
				return r.Class
			}
		}
	}
	return ""
}

// UpdatePodStatus updates pod status for Kubernetes-managed QoS resources.
func UpdatePodStatus(pod *v1.Pod, status *v1.PodStatus) {
	if utilfeature.DefaultFeatureGate.Enabled(features.QOSResources) {
		// Set defaults
		defaults := []v1.QOSResourceRequest{}
		if libcontainercgroups.IsCgroup2UnifiedMode() {
			defaults = append(defaults, v1.QOSResourceRequest{
				Name:  v1.QOSResourceOOMGroupKill,
				Class: v1.QOSResourceClassOOMGroupKillEnabled,
			})
		}

		// Simply mirror what was requested in PodSpec
		for i, c := range pod.Spec.Containers {
			for _, req := range defaults {
				setQOSResourceRequest(&status.ContainerStatuses[i].QOSResources, req)
			}
			for _, req := range c.Resources.QOSResources {
				if req.Name == v1.QOSResourceOOMGroupKill {
					setQOSResourceRequest(&status.ContainerStatuses[i].QOSResources, req)
				}
			}
		}
	}
}

func setQOSResourceRequest(reqs *[]v1.QOSResourceRequest, req v1.QOSResourceRequest) {
	for i := range *reqs {
		if (*reqs)[i].Name == req.Name {
			(*reqs)[i] = req
			return
		}
	}
	*reqs = append(*reqs, req)
}
