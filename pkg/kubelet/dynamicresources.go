/*
Copyright 2025 The Kubernetes Authors.

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

package kubelet

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"
	cpuset "k8s.io/utils/cpuset"
)

// zone is a helper for constructing a traversable tree of the resource topology.
type zone struct {
	*runtimeapi.ResourceTopologyZone

	children []*zone
}

func (z *zone) fillMachineInfo(machineInfo *cadvisorapi.MachineInfo) error {
	if z.ResourceTopologyZone == nil {
		return fmt.Errorf("zone %q has no resource topology", z.Name)
	}

	// Fill in the machine info
	//
	// The following are needed:
	// - For resource management:
	//   - NumCores
	//   - NumPhysicalCores (Windows perfstats only)
	//   - NumSockets
	//   - MemoryCapacity
	//   - SwapCapacity
	//   - HugePages
	//   - Topology
	// - For node status (should we still take from cadvisor?):
	//   - MachineID
	//   - SystemUUID
	//   - BootID

	// The following fields are left empty (not used by kubelet):
	// - Timestamp
	// - CPUVendorID
	// - CpuFrequency
	// - MemoryByType
	// - NVMInfo
	// - Filesystems
	// - DiskMap
	// - NetworkDevices
	// - CloudProvider
	// - InstanceType
	// - InstanceID

	// Get machine system attributes (if found)
	if machineID, ok := z.getAttribute("machine-id"); ok {
		machineInfo.MachineID = machineID
	}
	if bootID, ok := z.getAttribute("boot-id"); ok {
		machineInfo.BootID = bootID
	}
	if systemUUID, ok := z.getAttribute("system-uuid"); ok {
		machineInfo.SystemUUID = systemUUID
	}

	// Get capacity
	capacity := z.capacity()
	q := capacity[v1.ResourceCPU]
	machineInfo.NumCores = int(q.Value())
	q = capacity[v1.ResourceMemory]
	machineInfo.MemoryCapacity = uint64(q.Value())
	q = capacity["swap"]
	machineInfo.SwapCapacity = uint64(q.Value())

	// Huge pages
	machineInfo.HugePages = z.hugePages()

	// Topology
	machineInfo.Topology = z.numaNodes()
	sort.Slice(machineInfo.Topology, func(i, j int) bool {
		return machineInfo.Topology[i].Id < machineInfo.Topology[j].Id
	})

	// Backfill missing information
	sockets := map[int]struct{}{}
	for _, node := range machineInfo.Topology {
		machineInfo.NumPhysicalCores += len(node.Cores)
		for _, core := range node.Cores {
			sockets[core.SocketID] = struct{}{}
		}
	}
	machineInfo.NumSockets = len(sockets)

	return nil
}

func (z *zone) cpuidsFromAttributes() (cpuset.CPUSet, error) {
	if ids, ok := z.Attributes["cpu-ids"]; ok {
		return cpuset.Parse(ids)
	}

	return cpuset.New(), nil
}

func (z *zone) cpuSet() (cpuset.CPUSet, error) {
	cpus, err := z.cpuidsFromAttributes()
	if err != nil {
		return cpuset.CPUSet{}, err
	}
	for _, child := range z.children {
		childCPUs, err := child.cpuSet()
		if err != nil {
			return cpuset.CPUSet{}, err
		}
		cpus = cpus.Union(childCPUs)
	}
	return cpus, nil
}

func (z *zone) cores() []cadvisorapi.Core {
	// track the ids over recursion
	socketID := -1
	coreID := -1

	cores := []cadvisorapi.Core{}

	var traverse func(*zone)
	traverse = func(this *zone) {
		switch this.Type {
		case runtimeapi.ResourceTopologyZonePackage:
			socketID = this.getID(socketID + 1)

		case runtimeapi.ResourceTopologyZoneCore:
			coreID = this.getID(coreID + 1)
		}

		// Pick CPUs from whichever zone specifies CPU capacity
		// We expect that the CPU IDs are specified in the leaf zones
		if this.hasResource(v1.ResourceCPU) {
			cpus, err := this.cpuidsFromAttributes()
			if err != nil {
				klog.ErrorS(err, "failed to get cpuids from attributes", "zone", this.Name)
			} else {
				if socketID < 0 {
					socketID = 0
				}
				if coreID < 0 {
					coreID = 0
				}

				cores = append(cores, cadvisorapi.Core{
					Id:       coreID,
					SocketID: socketID,
					Threads:  cpus.List(),
				})
			}
			if len(this.children) > 0 {
				klog.InfoS("CPU capacity specified in zone with children", "zone", this.Name)
			}
		}

		for _, child := range this.children {
			traverse(child)
		}
	}

	traverse(z)

	return cores
}

func (z *zone) numaNodes() []cadvisorapi.Node {
	// track the node id over recursion
	nodeID := -1

	cores := z.cores()
	nodes := []cadvisorapi.Node{}

	var traverse func(*zone)
	traverse = func(this *zone) {
		switch this.Type {
		case runtimeapi.ResourceTopologyZoneNUMANode:
			nodeCores := this.cores()
			if len(nodeCores) == 0 {
				// If cores are not in the numa hierarchy, take local cpus from attributes
				cpus, err := this.cpuidsFromAttributes()
				if err != nil {
					klog.ErrorS(err, "failed to get cpuids from attributes", "zone", this.Name)
				} else {
					nodeCores = pickCores(cores, cpus)
				}
			}
			if len(nodeCores) == 0 {
				klog.InfoS("No cores found for numa node", "zone", this.Name)
			}

			capacity := this.capacity()
			q := capacity[v1.ResourceMemory]
			nodes = append(nodes, cadvisorapi.Node{
				Id:        this.getID(nodeID + 1),
				HugePages: this.hugePages(),
				Memory:    uint64(q.Value()),
				Cores:     nodeCores,
			})
		}

		for _, child := range this.children {
			traverse(child)
		}
	}

	traverse(z)

	return nodes
}

func pickCores(cores []cadvisorapi.Core, threads cpuset.CPUSet) []cadvisorapi.Core {
	ret := []cadvisorapi.Core{}
	for _, c := range cores {
		for _, t := range c.Threads {
			if threads.Contains(t) {
				ret = append(ret, c)
				break
			}
		}
	}
	return ret
}

func (z *zone) capacity() v1.ResourceList {
	capacity := v1.ResourceList{}
	for _, res := range z.Resources {
		if res.Capacity == nil {
			klog.InfoS("ResourceTopology resource has no capacity", "zone", z.Name, "resource", res.Name)
			continue
		}
		capacity[v1.ResourceName(res.Name)] = *res.Capacity
	}
	for _, child := range z.children {
		childCapacity := child.capacity()
		for name, quantity := range childCapacity {
			c := capacity[name]
			c.Add(quantity)
			capacity[name] = c
		}
	}

	return capacity
}

func (z *zone) hasResource(name v1.ResourceName) bool {
	for _, res := range z.Resources {
		if v1.ResourceName(res.Name) == name {
			return true
		}
	}
	return false
}

func (z *zone) getID(defaultID int) int {
	if idStr, ok := z.Attributes["id"]; ok {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			klog.ErrorS(err, "failed to parse zone id", "zone", z.Name, "id", idStr)
		} else {
			return id
		}
	}
	return defaultID
}

// TODO: Distances / Cost
func (z *zone) hugePages() []cadvisorapi.HugePagesInfo {
	hugePages := []cadvisorapi.HugePagesInfo{}

	// Note: capacity() counts all children so no recursion here
	capacity := z.capacity()
	for name, quantity := range capacity {
		if strings.HasPrefix(string(name), v1.ResourceHugePagesPrefix) {
			q, err := resource.ParseQuantity(string(name)[len(v1.ResourceHugePagesPrefix):])
			if err != nil {
				klog.ErrorS(err, "failed to parse hugepage page size", "zone", z.Name, "resource", name)
				continue
			}
			hugePages = append(hugePages, cadvisorapi.HugePagesInfo{
				PageSize: uint64(q.Value() / 1024),
				NumPages: uint64(quantity.Value()),
			})
		}
	}
	return hugePages
}

// getAttribute recursively searches for an attribute. Useful for system attributes.
func (z *zone) getAttribute(name string) (string, bool) {
	if v, ok := z.Attributes[name]; ok {
		return v, true
	}
	for _, child := range z.children {
		if v, ok := child.getAttribute(name); ok {
			return v, true
		}
	}
	return "", false
}

func dynamicRuntimeConfigToMachineInfo(drc *runtimeapi.DynamicRuntimeConfigResponse) (*cadvisorapi.MachineInfo, error) {
	trees, err := parseResourceTopology(drc.GetResourceTopology())
	if err != nil {
		return nil, err
	}

	machineInfo := &cadvisorapi.MachineInfo{}
	switch len(trees) {
	case 0:
		return nil, fmt.Errorf("no resource topology zones found")
	case 1:
		if err := trees[0].fillMachineInfo(machineInfo); err != nil {
			return nil, fmt.Errorf("failed to fill machine info: %w", err)
		}
	default:
		return nil, fmt.Errorf("distinct resource topology trees found, multiple trees not supported!")
	}

	return machineInfo, nil
}

// parseResourceTopology parses a runtimeapi.ResourceTopology into trees of zones
func parseResourceTopology(rt *runtimeapi.ResourceTopology) ([]*zone, error) {
	trees := []*zone{}

	zones, err := resourceTopologyToZoneMap(rt)
	if err != nil {
		return nil, err
	}

	for _, zone := range zones {
		if zone.Parent != "" {
			parent, ok := zones[zone.Parent]
			if !ok {
				return nil, fmt.Errorf("zone %q has unknown parent %q", zone.Name, zone.Parent)
			}
			parent.children = append(parent.children, zone)
		} else {
			trees = append(trees, zone)
		}
	}
	return trees, nil
}

func resourceTopologyToZoneMap(rt *runtimeapi.ResourceTopology) (map[string]*zone, error) {
	m := make(map[string]*zone, len(rt.Zones))
	for _, z := range rt.Zones {
		if _, ok := m[z.Name]; ok {
			return nil, fmt.Errorf("duplicate zone name %q", z.Name)
		}
		m[z.Name] = &zone{ResourceTopologyZone: z}
	}
	return m, nil
}
