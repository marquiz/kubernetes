/*
Copyright 2022 The Kubernetes Authors.

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

package v1

import (
	corev1 "k8s.io/api/core/v1"
)

// QOSResourceEquals returns true if the two QoS resource quotas are equivalent
func QOSResourceEquals(a, b corev1.QOSResourceQuota) bool {
	return qosResourceListEquals(a.Pod, b.Pod) && qosResourceListEquals(a.Container, b.Container)
}

func qosResourceListEquals(a, b []corev1.AllowedQOSResource) bool {
	if len(a) != len(b) {
		return false
	}

	// Do the simplest thing, don't try to be smart with different ordering for example
	for i, infoA := range a {
		infoB := b[i]
		if infoA.Name != infoB.Name {
			return false
		}

		if len(infoA.Classes) != len(infoB.Classes) {
			return false
		}

		for i, classA := range infoA.Classes {
			if classA != infoB.Classes[i] {
				return false
			}
		}
	}

	return true
}

// PodQOSResourcesDenied checks if the requested Pod QoS resources are allowed,
// returning details about denied QoS resources.
func PodQOSResourcesDenied(requests []corev1.PodQOSResourceRequest, limits []corev1.AllowedQOSResource) map[corev1.QOSResourceName]string {
	denied := map[corev1.QOSResourceName]string{}

	for _, req := range requests {
		if !QOSResourceAllowed(req.Name, req.Class, limits) {
			denied[req.Name] = req.Class
		}
	}
	return denied
}

// ContainerQOSResourcesDenied checks if the requested Container QoS resources
// are allowed, returning details about denied QoS resources.
func ContainerQOSResourcesDenied(requests []corev1.QOSResourceRequest, limits []corev1.AllowedQOSResource) map[corev1.QOSResourceName]string {
	denied := map[corev1.QOSResourceName]string{}

	for _, req := range requests {
		if !QOSResourceAllowed(req.Name, req.Class, limits) {
			denied[req.Name] = req.Class
		}
	}
	return denied
}

// QOSResourceAllowed returns if QoS resource assignment is allowed.
func QOSResourceAllowed(name corev1.QOSResourceName, class string, limits []corev1.AllowedQOSResource) bool {
	for _, limitedRes := range limits {
		if name == limitedRes.Name {
			if !allowedClassesContain(limitedRes.Classes, class) {
				return false
			}
		}
	}

	return true
}

func allowedClassesContain(elems []corev1.AllowedQOSResourceClass, name string) bool {
	for _, v := range elems {
		if v.Name == name {
			return true
		}
	}
	return false
}
