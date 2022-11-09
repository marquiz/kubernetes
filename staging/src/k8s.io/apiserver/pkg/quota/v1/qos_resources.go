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

// SumQoSResources sums two QoS resource quotas together
func SumQoSResources(a, b []corev1.AllowedQoSResource) []corev1.AllowedQoSResource {
	out := make([]corev1.AllowedQoSResource, len(a))
	for i := range a {
		out[i] = *a[i].DeepCopy()
	}

	for _, res := range b {
		for _, cls := range res.Classes {
			AddQoSResource(&out, res.Name, cls.Name, cls.Capacity)
		}
	}
	return out
}

// MaskQoSResources masks out from A QoS resources that don't exist in B
func MaskQoSResources(a, b []corev1.AllowedQoSResource) []corev1.AllowedQoSResource {
	out := []corev1.AllowedQoSResource{}

	for _, res := range a {
		for _, cls := range res.Classes {
			_, found := GetQoSResourceCapacity(b, res.Name, cls.Name)
			if found {
				AddQoSResource(&out, res.Name, cls.Name, cls.Capacity)
			}
		}
	}
	return out
}

// AddQoSResources add QoS resources to a quota object
func AddQoSResources(quota *[]corev1.AllowedQoSResource, requests map[corev1.QoSResourceName]string) {
	for reqRes, reqClass := range requests {
		AddQoSResource(quota, reqRes, reqClass, 1)
	}
}

func AddQoSResource(quota *[]corev1.AllowedQoSResource, name corev1.QoSResourceName, class string, amount int64) {
	for i, quotaRes := range *quota {
		if name == quotaRes.Name {
			AddQoSResourceClass(&(*quota)[i].Classes, class, amount)
			return
		}
	}
	newItem := corev1.AllowedQoSResource{
		Name: name,
		Classes: []corev1.AllowedQoSResourceClass{
			{Name: class, Capacity: amount}}}

	*quota = append(*quota, newItem)
}

func AddQoSResourceClass(quota *[]corev1.AllowedQoSResourceClass, request string, amount int64) {
	for i, quotaClass := range *quota {
		if request == quotaClass.Name {
			quotaClass.Capacity += amount
			(*quota)[i] = quotaClass
			return
		}
	}
	*quota = append(*quota, corev1.AllowedQoSResourceClass{Name: request, Capacity: amount})
}

func MaxQoSResources(quota *[]corev1.AllowedQoSResource, requests map[corev1.QoSResourceName]string) {
	for reqRes, reqClass := range requests {
		MaxQoSResource(quota, reqRes, reqClass)
	}
}

func MaxQoSResource(quota *[]corev1.AllowedQoSResource, name corev1.QoSResourceName, class string) {
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
	AddQoSResource(quota, name, class, 1)
}

func GetQoSResourceCapacity(quota []corev1.AllowedQoSResource, resName corev1.QoSResourceName, className string) (int64, bool) {
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
