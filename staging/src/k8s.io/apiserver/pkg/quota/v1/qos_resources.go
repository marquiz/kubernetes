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

// SumQOSResources sums two QoS resource quotas together
func SumQOSResources(a, b []corev1.AllowedQOSResource) []corev1.AllowedQOSResource {
	out := make([]corev1.AllowedQOSResource, len(a))
	for i := range a {
		out[i] = *a[i].DeepCopy()
	}

	for _, res := range b {
		for _, cls := range res.Classes {
			AddQOSResource(&out, res.Name, cls.Name, cls.Capacity)
		}
	}
	return out
}

// MaskQOSResources masks out from A QoS resources that don't exist in B
func MaskQOSResources(a, b []corev1.AllowedQOSResource) []corev1.AllowedQOSResource {
	out := []corev1.AllowedQOSResource{}

	for _, res := range a {
		for _, cls := range res.Classes {
			_, found := GetQOSResourceCapacity(b, res.Name, cls.Name)
			if found {
				AddQOSResource(&out, res.Name, cls.Name, cls.Capacity)
			}
		}
	}
	return out
}

// AddPodQOSResources adds Pod QoS resources to a quota object.
func AddPodQOSResources(quota *[]corev1.AllowedQOSResource, requests []corev1.PodQOSResourceRequest) {
	for _, req := range requests {
		AddQOSResource(quota, req.Name, req.Class, 1)
	}
}

// AddContainerQOSResources adds Container QoS resources to a quota object.
func AddContainerQOSResources(quota *[]corev1.AllowedQOSResource, requests []corev1.QOSResourceRequest) {
	for _, req := range requests {
		AddQOSResource(quota, req.Name, req.Class, 1)
	}
}

// AddPodQOSResource adds an amount of one QoS resources/class to a quota object.
func AddQOSResource(quota *[]corev1.AllowedQOSResource, name corev1.QOSResourceName, class string, amount int64) {
	for i, quotaRes := range *quota {
		if name == quotaRes.Name {
			AddQOSResourceClass(&(*quota)[i].Classes, class, amount)
			return
		}
	}
	newItem := corev1.AllowedQOSResource{
		Name: name,
		Classes: []corev1.AllowedQOSResourceClass{
			{Name: class, Capacity: amount}}}

	*quota = append(*quota, newItem)
}

func AddQOSResourceClass(quota *[]corev1.AllowedQOSResourceClass, request string, amount int64) {
	for i, quotaClass := range *quota {
		if request == quotaClass.Name {
			quotaClass.Capacity += amount
			(*quota)[i] = quotaClass
			return
		}
	}
	*quota = append(*quota, corev1.AllowedQOSResourceClass{Name: request, Capacity: amount})
}

func MaxContainerQOSResources(quota *[]corev1.AllowedQOSResource, requests []corev1.QOSResourceRequest) {
	for _, req := range requests {
		MaxQOSResource(quota, req.Name, req.Class)
	}
}

func MaxQOSResource(quota *[]corev1.AllowedQOSResource, name corev1.QOSResourceName, class string) {
	for i, quotaRes := range *quota {
		if name == quotaRes.Name {
			for j, quotaClass := range quotaRes.Classes {
				if class == quotaClass.Name {
					if quotaClass.Capacity < 1 {
						(*quota)[i].Classes[j].Capacity = 1
					}
					return
				}
			}
		}
	}
	// Was not found -> add it (capacity will be set to 1)
	AddQOSResource(quota, name, class, 1)
}

func GetQOSResourceCapacity(quota []corev1.AllowedQOSResource, resName corev1.QOSResourceName, className string) (int64, bool) {
	for _, res := range quota {
		if resName == res.Name {
			for _, cls := range res.Classes {
				if className == cls.Name {
					return cls.Capacity, true
				}
			}
		}
	}
	// Not found
	return 0, false
}

// ContainerQOSResourcesDenied checks if the requested Container QoS resources
// are allowed, returning details about denied QoS resources.
func ContainerQOSResourcesDenied(requests []corev1.QOSResourceRequest, limits []corev1.AllowedQOSResource) map[corev1.QOSResourceName]string {
	denied := map[corev1.QOSResourceName]string{}

	for _, req := range requests {
		if !IsQOSResourceAllowed(req.Name, req.Class, limits) {
			denied[req.Name] = req.Class
		}
	}
	return denied
}

// IsQOSResourceAllowed returns if QoS resource assignment is allowed.
func IsQOSResourceAllowed(name corev1.QOSResourceName, class string, limits []corev1.AllowedQOSResource) bool {
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
