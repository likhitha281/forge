package worker

import (
	"os/exec"
	"testing"
	"time"

	"github.com/likhitha281/forge/internal/resource"
)

func TestRuntimeManagerLifecycle(t *testing.T) {
	manager := NewRuntimeManager()

	runtime := NewRuntime(
		"job-1",
		exec.Command("echo", "hello"),
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     2,
		},
		time.Now(),
	)

	if err := manager.Add(runtime); err != nil {
		t.Fatalf("Add(): %v", err)
	}

	if manager.Count() != 1 {
		t.Fatalf(
			"Count() = %d, want 1",
			manager.Count(),
		)
	}

	got, ok := manager.Get("job-1")

	if !ok {
		t.Fatal("expected runtime")
	}

	if got != runtime {
		t.Fatal(
			"Get() returned different runtime",
		)
	}

	removed, ok :=
		manager.Remove("job-1")

	if !ok {
		t.Fatal(
			"expected runtime to be removed",
		)
	}

	if removed != runtime {
		t.Fatal(
			"Remove() returned different runtime",
		)
	}

	if manager.Count() != 0 {
		t.Fatalf(
			"Count() = %d, want 0",
			manager.Count(),
		)
	}
}

func TestRuntimeManagerRejectsDuplicate(
	t *testing.T,
) {
	manager := NewRuntimeManager()

	first := NewRuntime(
		"job-1",
		nil,
		resource.Vector{},
		time.Now(),
	)

	second := NewRuntime(
		"job-1",
		nil,
		resource.Vector{},
		time.Now(),
	)

	if err := manager.Add(first); err != nil {
		t.Fatalf("first Add(): %v", err)
	}

	err := manager.Add(second)

	if err != ErrRuntimeAlreadyExists {
		t.Fatalf(
			"error = %v, want ErrRuntimeAlreadyExists",
			err,
		)
	}
}

func TestRuntimeAllocationUpdate(
	t *testing.T,
) {
	runtime := NewRuntime(
		"job-1",
		nil,
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     2,
		},
		time.Now(),
	)

	runtime.SetAllocation(
		resource.Vector{
			CPUCores: 2,
			MemoryMB: 2048,
			GPUs:     4,
		},
	)

	got := runtime.Allocation()

	if got.GPUs != 4 {
		t.Fatalf(
			"GPUs = %d, want 4",
			got.GPUs,
		)
	}
}

func TestRuntimeManagerSnapshot(
	t *testing.T,
) {
	manager :=
		NewRuntimeManager()

	runtimeA :=
		NewRuntime(
			"job-a",
			nil,
			resource.Vector{
				GPUs: 2,
			},
			time.Now(),
		)

	runtimeB :=
		NewRuntime(
			"job-b",
			nil,
			resource.Vector{
				GPUs: 4,
			},
			time.Now(),
		)

	if err := manager.Add(runtimeA); err != nil {
		t.Fatal(err)
	}

	if err := manager.Add(runtimeB); err != nil {
		t.Fatal(err)
	}

	snapshot :=
		manager.Snapshot()

	if len(snapshot) != 2 {
		t.Fatalf(
			"len(snapshot) = %d, want 2",
			len(snapshot),
		)
	}
}
