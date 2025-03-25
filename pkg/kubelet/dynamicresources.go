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
	"sort"
	"strings"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	v1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/klog/v2"
)

func dynamicRuntimeConfigToMachineInfo(drc *runtimeapi.DynamicRuntimeConfigResponse) (*cadvisorapi.MachineInfo, error) {
	machineInfo := &cadvisorapi.MachineInfo{}

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
	parseSystemAttributes(machineInfo, drc.SystemAttributes)
	parseResourceTopology(machineInfo, drc.ResourceTopology)

	return machineInfo, nil
}

func parseSystemAttributes(machineInfo *cadvisorapi.MachineInfo, attrs map[string]string) {
	if machineID, ok := attrs["machine-id"]; ok {
		machineInfo.MachineID = machineID
	}
	if bootID, ok := attrs["boot-id"]; ok {
		machineInfo.BootID = bootID
	}
	if systemUUID, ok := attrs["system-uuid"]; ok {
		machineInfo.SystemUUID = systemUUID
	}
}

func parseResourceTopology(machineInfo *cadvisorapi.MachineInfo, rt *runtimeapi.ResourceTopology) {
	// Create a lookup map of cpuinfo
	// NOTE: should be able to drop this when ditching cadvisor machineinfo for good
	cpuInfo := make(map[int64]*runtimeapi.ResourceCpuInfo)
	physicalCoresPerSocket := map[int64]map[int64]struct{}{}
	for _, cpu := range rt.CpuInfo {
		cpuInfo[cpu.Id] = cpu
		if _, ok := physicalCoresPerSocket[cpu.SocketId]; !ok {
			physicalCoresPerSocket[cpu.SocketId] = map[int64]struct{}{}
		}
		physicalCoresPerSocket[cpu.SocketId][cpu.CoreId] = struct{}{}
	}

	// NUMA nodes
	memTotal := uint64(0)
	hugePagesTotal := map[uint64]uint64{}
	for _, node := range rt.NumaNodeInfo {
		memory := getNumaMemory(node)

		hugePages := getNumaHugePages(node)

		distances := make([]uint64, len(node.Distance))
		for i, d := range node.Distance {
			distances[i] = uint64(d)
		}

		machineInfo.Topology = append(machineInfo.Topology, cadvisorapi.Node{
			Id:        int(node.Id),
			Cores:     getNumaCores(node, cpuInfo),
			HugePages: hugePages,
			Memory:    memory,
			Distances: distances,
		})
		memTotal += memory
		for _, hp := range hugePages {
			hugePagesTotal[hp.PageSize] = hugePagesTotal[hp.PageSize] + hp.NumPages
		}
	}

	// Backfill other fields
	machineInfo.NumCores = len(rt.CpuInfo)
	machineInfo.NumSockets = len(physicalCoresPerSocket)
	for _, cores := range physicalCoresPerSocket {
		machineInfo.NumPhysicalCores += len(cores)
	}
	machineInfo.MemoryCapacity = memTotal
	machineInfo.SwapCapacity = uint64(rt.SwapInfo.Capacity.Value())

	for pageSize, numPages := range hugePagesTotal {
		machineInfo.HugePages = append(machineInfo.HugePages, cadvisorapi.HugePagesInfo{
			PageSize: pageSize,
			NumPages: numPages,
		})
	}
}

func getNumaCores(numaNode *runtimeapi.ResourceNumaNodeInfo, cpuInfo map[int64]*runtimeapi.ResourceCpuInfo) []cadvisorapi.Core {
	cores := map[int64]cadvisorapi.Core{}
	for _, cpuID := range numaNode.CpuIds {
		cpu, ok := cpuInfo[cpuID]
		if !ok {
			klog.ErrorS(nil, "failed to find cpu info for numa node", "node", numaNode.Id, "cpu", cpuID)
			continue
		}
		core, ok := cores[cpu.SocketId]
		if !ok {
			core = cadvisorapi.Core{
				Id:       int(cpu.CoreId),
				SocketID: int(cpu.SocketId),
				Threads:  []int{int(cpu.Id)},
			}
		} else {
			core.Threads = append(core.Threads, int(cpu.Id))
			sort.Ints(core.Threads)
		}
		cores[cpu.CoreId] = core
	}

	c := make([]cadvisorapi.Core, 0, len(cores))
	for _, core := range cores {
		c = append(c, core)
	}
	sort.Slice(c, func(i, j int) bool {
		return c[i].Id < c[j].Id
	})

	return c
}

func getNumaMemory(numaNode *runtimeapi.ResourceNumaNodeInfo) uint64 {
	for _, res := range numaNode.Resources {
		if res.Name == string(v1.ResourceMemory) {
			return uint64(res.Capacity.Value())
		}
	}
	return 0
}

func getNumaHugePages(numaNode *runtimeapi.ResourceNumaNodeInfo) []cadvisorapi.HugePagesInfo {
	hugePages := []cadvisorapi.HugePagesInfo{}

	for _, res := range numaNode.Resources {
		name := string(res.Name)
		if strings.HasPrefix(name, v1.ResourceHugePagesPrefix) {
			q, err := resource.ParseQuantity(string(name)[len(v1.ResourceHugePagesPrefix):])
			if err != nil {
				klog.ErrorS(err, "failed to parse hugepage page size", "numaNode", numaNode.Id, "resource", name)
				continue
			}
			hugePages = append(hugePages, cadvisorapi.HugePagesInfo{
				PageSize: uint64(q.Value() / 1024),
				NumPages: uint64(res.Capacity.Value()),
			})
		}
	}
	return hugePages
}
