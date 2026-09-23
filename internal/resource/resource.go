package resource

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrNegativeCPU    = errors.New("cpu cores cannot be negative")
	ErrNegativeMemory = errors.New("memory cannot be negative")
	ErrNegativeGPU    = errors.New("gpu count cannot be negative")
	ErrInsufficient   = errors.New("insufficient resources")
)

const epsilon = 1e-9

// Vector represents a quantity of compute resources.
//
// CPUCores may be fractional. For example, 0.5 represents half of one
// logical CPU core.
//
// MemoryMB is stored as an integer number of MiB-like units. Forge keeps
// resource arithmetic numeric internally rather than storing values such
// as "8GB".
//
// GPUs is initially only a count. Device identity, model, topology, and
// GPU memory are intentionally deferred to later versions of Forge.
type Vector struct {
	CPUCores float64
	MemoryMB int64
	GPUs     int32
}

// Validate checks whether a resource vector is valid.
func (v Vector) Validate() error {
	if math.IsNaN(v.CPUCores) || math.IsInf(v.CPUCores, 0) {
		return fmt.Errorf("invalid cpu cores: %v", v.CPUCores)
	}

	if v.CPUCores < 0 {
		return ErrNegativeCPU
	}

	if v.MemoryMB < 0 {
		return ErrNegativeMemory
	}

	if v.GPUs < 0 {
		return ErrNegativeGPU
	}

	return nil
}

// IsZero reports whether the vector contains no resources.
func (v Vector) IsZero() bool {
	return math.Abs(v.CPUCores) < epsilon &&
		v.MemoryMB == 0 &&
		v.GPUs == 0
}

// Fits reports whether available resources can satisfy requested resources.
func (v Vector) Fits(requested Vector) bool {
	return v.CPUCores+epsilon >= requested.CPUCores &&
		v.MemoryMB >= requested.MemoryMB &&
		v.GPUs >= requested.GPUs
}

// Add returns the element-wise sum of two resource vectors.
func (v Vector) Add(other Vector) Vector {
	return Vector{
		CPUCores: v.CPUCores + other.CPUCores,
		MemoryMB: v.MemoryMB + other.MemoryMB,
		GPUs:     v.GPUs + other.GPUs,
	}
}

// Subtract returns v - other.
//
// Subtract rejects operations that would produce negative resources.
func (v Vector) Subtract(other Vector) (Vector, error) {
	if !v.Fits(other) {
		return Vector{}, ErrInsufficient
	}

	result := Vector{
		CPUCores: v.CPUCores - other.CPUCores,
		MemoryMB: v.MemoryMB - other.MemoryMB,
		GPUs:     v.GPUs - other.GPUs,
	}

	// Avoid tiny floating-point remnants such as
	// 1.0000000001 - 1.0.
	if math.Abs(result.CPUCores) < epsilon {
		result.CPUCores = 0
	}

	return result, nil
}

// Equal reports whether two resource vectors represent the same resources.
func (v Vector) Equal(other Vector) bool {
	return math.Abs(v.CPUCores-other.CPUCores) < epsilon &&
		v.MemoryMB == other.MemoryMB &&
		v.GPUs == other.GPUs
}

func (v Vector) String() string {
	return fmt.Sprintf(
		"cpu=%.2f memory=%dMB gpu=%d",
		v.CPUCores,
		v.MemoryMB,
		v.GPUs,
	)
}
