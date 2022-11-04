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
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
)

type QoSResources map[corev1.QoSResourceName]QoSResourceClasses

// QoSResourceClasses stores a set of classes (of one type of QoS resource)
// plus their capacities. Nil pointer implies infinite capacity.
type QoSResourceClasses map[string]*int64

// PodQoSResources calculates the QoS resource usage of a pod
func CalculatePodQoSResourceUsage(pod *v1.Pod) (podres, containerres QoSResources) {
	podres, containerres = QoSResources{}, QoSResources{}

	// Handle pod-level QoS resources
	podres.AddQoSResources(pod.Spec.Resources.QoSResources)

	for _, c := range pod.Spec.Containers {
		containerres.AddQoSResources(c.Resources.QoSResources)
	}

	for _, c := range pod.Spec.InitContainers {
		containerres.SetMaxQoSResources(c.Resources.QoSResources)
	}

	return podres, containerres
}

func ConvertQoSResourceStatus(in []corev1.QoSResourceInfo) QoSResources {
	out := make(QoSResources, len(in))
	for _, cr := range in {
		classes := make(QoSResourceClasses, len(cr.Classes))
		for _, c := range cr.Classes {
			if c.Capacity > 0 {
				pc := new(int64)
				*pc = c.Capacity
				classes[c.Name] = pc
			} else {
				classes[c.Name] = nil
			}
		}
		out[cr.Name] = classes
	}
	return out
}

func (r *QoSResources) AddQoSResources(crl map[corev1.QoSResourceName]string) {
	if r == nil {
		return
	}

	for name, class := range crl {
		r.Add(name, class, 1)
	}
}

func (r *QoSResources) SetMaxQoSResources(crl map[corev1.QoSResourceName]string) {
	if r == nil {
		return
	}

	for name, class := range crl {
		if (*r)[name] == nil ||
			(*r)[name][class] == nil ||
			*(*r)[name][class] == 0 {
			r.Add(name, class, 1)
		}
	}
}

// GetCapacity gets the capacity of one class of a QoS resource. It returns
// two booleans and an integer. The first boolean tells whether the resource
// type and the class exist and the second whether it has capacity defined or
// not. Integer specifies the capacity and is only valid if both booleans are
// true.
func (r *QoSResources) GetCapacity(name corev1.QoSResourceName, class string) (bool, bool, int64) {
	if r == nil || *r == nil {
		return false, false, 0
	}
	if _, ok := (*r)[name]; !ok {
		// Resource does not exist
		return false, false, 0
	}
	if capa, ok := (*r)[name][class]; ok {
		if capa != nil {
			return true, true, *capa
		}
		return true, false, 0
	}
	// Class does not exist
	return false, false, 0
}

func (r *QoSResources) Add(name corev1.QoSResourceName, class string, amount int64) {
	if r == nil {
		return
	}
	if *r == nil {
		*r = make(QoSResources)
	}
	if (*r)[name] == nil {
		(*r)[name] = make(QoSResourceClasses)
	}
	if (*r)[name][class] == nil {
		(*r)[name][class] = new(int64)
	}
	*(*r)[name][class] += amount
}

func (r *QoSResources) Sum(r2 QoSResources) {
	if r == nil || r2 == nil {
		return
	}
	for resName, cr := range r2 {
		for clsName, vp := range cr {
			if vp != nil {
				r.Add(resName, clsName, *vp)
			}
		}
	}
}
