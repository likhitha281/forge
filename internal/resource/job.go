package resource

import (
	"errors"
	"fmt"
)

var (
	ErrNegativeJobCPU    = errors.New("job CPU cannot be negative")
	ErrNegativeJobMemory = errors.New("job memory cannot be negative")
)

// JobResources describes the legal resource configuration of a job.
//
// CPU and memory are fixed in the first version of Forge's malleable
// workload model. GPU allocation may vary within GPU.Min..GPU.Max.
type JobResources struct {
	CPUCores float64
	MemoryMB int64
	GPU      GPUEnvelope
}

func (r JobResources) Validate() error {
	if r.CPUCores < 0 {
		return ErrNegativeJobCPU
	}

	if r.MemoryMB < 0 {
		return ErrNegativeJobMemory
	}

	if err := r.GPU.Validate(); err != nil {
		return fmt.Errorf("invalid GPU envelope: %w", err)
	}

	return nil
}

// Minimum returns the smallest concrete allocation under which the
// workload can execute.
func (r JobResources) Minimum() Vector {
	return Vector{
		CPUCores: r.CPUCores,
		MemoryMB: r.MemoryMB,
		GPUs:     r.GPU.Min,
	}
}

// Preferred returns the workload's desired concrete allocation.
func (r JobResources) Preferred() Vector {
	return Vector{
		CPUCores: r.CPUCores,
		MemoryMB: r.MemoryMB,
		GPUs:     r.GPU.Preferred,
	}
}

// Maximum returns the largest concrete allocation Forge is permitted
// to assign to the workload.
func (r JobResources) Maximum() Vector {
	return Vector{
		CPUCores: r.CPUCores,
		MemoryMB: r.MemoryMB,
		GPUs:     r.GPU.Max,
	}
}

// Accepts reports whether allocation is a legal configuration for this job.
func (r JobResources) Accepts(allocation Vector) bool {
	if !almostEqual(r.CPUCores, allocation.CPUCores) {
		return false
	}

	if r.MemoryMB != allocation.MemoryMB {
		return false
	}

	return r.GPU.Contains(allocation.GPUs)
}

// InitialAllocation chooses a concrete allocation given the resources
// currently available on a worker.
//
// Policy:
//
//  1. CPU and memory must fit.
//  2. Prefer the requested GPU allocation.
//  3. If preferred does not fit, use as many GPUs as possible down to Min.
//  4. If Min cannot fit, the job cannot run.
//
// This is an INITIAL PLACEMENT policy only. It is deliberately not the
// transition-aware research scheduler.
func (r JobResources) InitialAllocation(
	available Vector,
) (Vector, bool) {
	if available.CPUCores < r.CPUCores ||
		available.MemoryMB < r.MemoryMB {
		return Vector{}, false
	}

	if available.GPUs < r.GPU.Min {
		return Vector{}, false
	}

	gpus := r.GPU.Preferred

	if available.GPUs < gpus {
		gpus = available.GPUs
	}

	if gpus > r.GPU.Max {
		gpus = r.GPU.Max
	}

	allocation := Vector{
		CPUCores: r.CPUCores,
		MemoryMB: r.MemoryMB,
		GPUs:     gpus,
	}

	return allocation, true
}

func almostEqual(a, b float64) bool {
	const epsilon = 1e-9

	diff := a - b
	if diff < 0 {
		diff = -diff
	}

	return diff < epsilon
}
