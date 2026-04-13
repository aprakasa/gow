// Package system provides hardware detection for the resource allocator.
package system

import (
	"runtime"

	"github.com/shirou/gopsutil/v4/mem"
)

// Specs holds the server hardware attributes the allocator needs.
type Specs struct {
	TotalRAMMB uint64
	CPUCores   int
}

// Detect reads total physical RAM and CPU core count from the host.
// It uses gopsutil for RAM (which works inside containers and across
// OS platforms) and runtime.NumCPU for cores.
func Detect() (Specs, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return Specs{}, err
	}
	return Specs{
		TotalRAMMB: vm.Total / 1024 / 1024,
		CPUCores:   runtime.NumCPU(),
	}, nil
}
