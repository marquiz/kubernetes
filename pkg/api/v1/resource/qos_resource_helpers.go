/*
Copyright 2023 The Kubernetes Authors.

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

package resource

import (
	v1 "k8s.io/api/core/v1"
)

// QOSResourcesTotal stores the total amount of QoS resources. It is a helper
// type for easier lookups and modifying the data.
type QOSResourcesTotal map[v1.QOSResourceName]QOSResourceTotal

// QOSResourceTotal stores the total amount of one QoS resource type. That is
// the set of classes (of that QoS resource type) and the total amount of each
// class.
type QOSResourceTotal map[string]int64

// QOSResourcesTotalFromInfo converts a list of QOSResourceInfo into an
// instance of QOSResourcesTotal.
func QOSResourcesTotalFromInfo(in []v1.QOSResourceInfo) QOSResourcesTotal {
	out := make(QOSResourcesTotal, len(in))
	for _, qr := range in {
		classes := make(QOSResourceTotal, len(qr.Classes))
		for _, c := range qr.Classes {
			classes[c.Name] = c.Capacity
		}
		out[qr.Name] = classes
	}
	return out
}

// PodQOSResourceRequests calculates the total amount of all QoS resources requested by a Pod.
func PodQOSResourceRequests(pod *v1.Pod) (podReqs, containerReqs QOSResourcesTotal) {
	podReqs = make(QOSResourcesTotal)
	containerReqs = make(QOSResourcesTotal)

	podReqs.AddPodQOSResources(pod.Spec.QOSResources)
	for _, container := range pod.Spec.Containers {
		containerReqs.AddContainerQOSResources(container.Resources.QOSResources)
	}

	// take max_resource(sum_pod, any_init_container)
	for _, container := range pod.Spec.InitContainers {
		containerReqs.SetMaxContainerQOSResources(container.Resources.QOSResources)
	}
	return podReqs, containerReqs
}

// AddPodQOSResources adds a list of pod-level QoS resource requests into the total.
func (r *QOSResourcesTotal) AddPodQOSResources(qrl []v1.PodQOSResourceRequest) {
	if r == nil {
		return
	}

	for _, qr := range qrl {
		r.add(qr.Name, qr.Class, 1)
	}
}

// AddPodQOSResources adds a list of container-level QoS resource requests into the total.
func (r *QOSResourcesTotal) AddContainerQOSResources(qrl []v1.QOSResourceRequest) {
	if r == nil {
		return
	}

	for _, qr := range qrl {
		r.add(qr.Name, qr.Class, 1)
	}
}

// SetMaxContainerQOSResources sets each value to the greater value found in
// the two totals.
func (r *QOSResourcesTotal) SetMaxContainerQOSResources(qrl []v1.QOSResourceRequest) {
	if r == nil {
		return
	}

	for _, qr := range qrl {
		if (*r)[qr.Name] == nil ||
			(*r)[qr.Name][qr.Class] == 0 {
			r.add(qr.Name, qr.Class, 1)
		}
	}
}

// GetAmount gets the total amount of one class of a QoS resource. It returns a
// boolean and an integer. The boolean tells whether the resource type and the
// class exist. The integer is the amount and is only valid if the boolean is
// true.
func (r *QOSResourcesTotal) GetAmount(name v1.QOSResourceName, class string) (bool, int64) {
	if r == nil || *r == nil {
		return false, 0
	}
	if _, ok := (*r)[name]; !ok {
		// QoS resource does not exist
		return false, 0
	}
	if amount, ok := (*r)[name][class]; ok {
		return true, amount
	}
	// Class does not exist
	return false, 0
}

// Sum adds together two QOSResourcesTotal instances.
func (r *QOSResourcesTotal) Sum(r2 *QOSResourcesTotal, add bool) {
	if r == nil || r2 == nil {
		return
	}
	for resName, resTotal := range *r2 {
		for clsName, clsAmount := range resTotal {
			if add {
				r.add(resName, clsName, clsAmount)
			} else {
				r.add(resName, clsName, -1*clsAmount)
			}
		}
	}
}

// Clone creates a (deep) copy of the QOSResourcesTotal instance.
func (r *QOSResourcesTotal) Clone() *QOSResourcesTotal {
	if r == nil {
		return nil
	}
	out := make(QOSResourcesTotal, len(*r))
	for k, v := range *r {
		classes := make(QOSResourceTotal, len(v))
		for c, amount := range v {
			classes[c] = amount
		}
		out[k] = classes
	}
	return &out
}

// add increases total of one resource/class by the given amount.
func (r *QOSResourcesTotal) add(name v1.QOSResourceName, class string, amount int64) {
	if r == nil {
		return
	}
	if *r == nil {
		*r = make(QOSResourcesTotal)
	}
	if (*r)[name] == nil {
		(*r)[name] = make(QOSResourceTotal)
	}
	(*r)[name][class] = (*r)[name][class] + amount
}
