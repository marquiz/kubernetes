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

// QoSResourceEquals returns true if the two QoS resource quotas are equivalent
func QoSResourceEquals(a, b corev1.QoSResourceQuota) bool {
	return qosResourceListEquals(a.Pod, b.Pod) && qosResourceListEquals(a.Container, b.Container)
}

func qosResourceListEquals(a, b []corev1.AllowedQoSResource) bool {
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

// QoSResourcesDenied checks if the requested QoS resources are allowed,
// returning details about denied QoS resources.
func QoSResourcesDenied(requests map[corev1.QoSResourceName]string, limits []corev1.AllowedQoSResource) map[corev1.QoSResourceName]string {
	denied := map[corev1.QoSResourceName]string{}

	for reqRes, reqClass := range requests {
		for _, limitedRes := range limits {
			if reqRes == limitedRes.Name {
				if !allowedClassesContain(limitedRes.Classes, reqClass) {
					denied[reqRes] = reqClass
				}
			}
		}
	}

	return denied
}

func allowedClassesContain(elems []corev1.AllowedQoSResourceClass, name string) bool {
	for _, v := range elems {
		if v.Name == name {
			return true
		}
	}
	return false
}

// QoSResourceNames returns a list of all QoS resource names in the array of AllowedQoSResource.
func QoSResourceNames(resources []corev1.AllowedQoSResource) []corev1.QoSResourceName {
	result := []corev1.QoSResourceName{}
	for _, resource := range resources {
		result = append(result, resource.Name)
	}
	return result
}
