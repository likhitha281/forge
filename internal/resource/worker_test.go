package resource

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWorkerResourcesReserveAndRelease(t *testing.T) {
	capacity := Vector{
		CPUCores: 8,
		MemoryMB: 32768,
		GPUs:     4,
	}

	resources, err := NewWorkerResources(capacity)
	if err != nil {
		t.Fatalf("NewWorkerResources() error: %v", err)
	}

	request := Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     2,
	}

	if err := resources.Reserve(request); err != nil {
		t.Fatalf("Reserve() error: %v", err)
	}

	wantAllocated := request

	if got := resources.Allocated(); !got.Equal(wantAllocated) {
		t.Fatalf("Allocated() = %v, want %v", got, wantAllocated)
	}

	wantAvailable := Vector{
		CPUCores: 4,
		MemoryMB: 24576,
		GPUs:     2,
	}

	if got := resources.Available(); !got.Equal(wantAvailable) {
		t.Fatalf("Available() = %v, want %v", got, wantAvailable)
	}

	if err := resources.Release(request); err != nil {
		t.Fatalf("Release() error: %v", err)
	}

	if !resources.Allocated().IsZero() {
		t.Fatalf(
			"resources still allocated after release: %v",
			resources.Allocated(),
		)
	}

	if got := resources.Available(); !got.Equal(capacity) {
		t.Fatalf("Available() = %v after release, want %v", got, capacity)
	}
}

func TestWorkerResourcesRejectsOverAllocation(t *testing.T) {
	resources, err := NewWorkerResources(Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     1,
	})
	if err != nil {
		t.Fatalf("NewWorkerResources() error: %v", err)
	}

	err = resources.Reserve(Vector{
		CPUCores: 2,
		MemoryMB: 4096,
		GPUs:     2,
	})

	if !errors.Is(err, ErrInsufficient) {
		t.Fatalf("Reserve() error = %v, want ErrInsufficient", err)
	}

	if !resources.Allocated().IsZero() {
		t.Fatalf(
			"failed reservation changed allocation: %v",
			resources.Allocated(),
		)
	}
}

func TestWorkerResourcesRejectsInvalidCapacity(t *testing.T) {
	_, err := NewWorkerResources(Vector{
		CPUCores: -1,
	})

	if !errors.Is(err, ErrInvalidCapacity) {
		t.Fatalf("error = %v, want ErrInvalidCapacity", err)
	}
}

func TestWorkerResourcesRejectsOverRelease(t *testing.T) {
	resources, err := NewWorkerResources(Vector{
		CPUCores: 8,
		MemoryMB: 16384,
		GPUs:     2,
	})
	if err != nil {
		t.Fatalf("NewWorkerResources() error: %v", err)
	}

	if err := resources.Reserve(Vector{
		CPUCores: 2,
		GPUs:     1,
	}); err != nil {
		t.Fatalf("Reserve() error: %v", err)
	}

	err = resources.Release(Vector{
		CPUCores: 4,
		GPUs:     1,
	})

	if err == nil {
		t.Fatal("Release() should reject releasing more than allocated")
	}
}

func TestConcurrentReservationDoesNotOverAllocate(t *testing.T) {
	resources, err := NewWorkerResources(Vector{
		CPUCores: 8,
		MemoryMB: 16384,
		GPUs:     4,
	})
	if err != nil {
		t.Fatalf("NewWorkerResources() error: %v", err)
	}

	request := Vector{
		CPUCores: 4,
		MemoryMB: 4096,
		GPUs:     4,
	}

	var successes int32

	var wg sync.WaitGroup

	const contenders = 20

	for i := 0; i < contenders; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := resources.Reserve(request); err == nil {
				atomic.AddInt32(&successes, 1)
			}
		}()
	}

	wg.Wait()

	if successes != 1 {
		t.Fatalf(
			"successful reservations = %d, want exactly 1",
			successes,
		)
	}

	want := Vector{
		CPUCores: 4,
		MemoryMB: 4096,
		GPUs:     4,
	}

	if got := resources.Allocated(); !got.Equal(want) {
		t.Fatalf("Allocated() = %v, want %v", got, want)
	}
}
