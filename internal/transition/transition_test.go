package transition

import (
	"errors"
	"testing"
	"time"

	"github.com/likhitha281/forge/internal/resource"
)

func transitionRequirements() resource.JobResources {
	return resource.JobResources{
		CPUCores: 4,
		MemoryMB: 8192,
		GPU: resource.GPUEnvelope{
			Min:       1,
			Preferred: 4,
			Max:       8,
		},
	}
}

func TestNewTransition(t *testing.T) {
	now := time.Now()

	source := resource.Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     2,
	}

	target := resource.Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     4,
	}

	tr, err := New(
		"transition-1",
		"job-1",
		source,
		target,
		transitionRequirements(),
		now,
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if tr.State != StateRequested {
		t.Fatalf(
			"state = %s, want REQUESTED",
			tr.State,
		)
	}

	if !tr.Source.Equal(source) {
		t.Fatalf(
			"source = %v, want %v",
			tr.Source,
			source,
		)
	}

	if !tr.Target.Equal(target) {
		t.Fatalf(
			"target = %v, want %v",
			tr.Target,
			target,
		)
	}
}

func TestRejectSameAllocation(t *testing.T) {
	allocation := resource.Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     2,
	}

	_, err := New(
		"transition-1",
		"job-1",
		allocation,
		allocation,
		transitionRequirements(),
		time.Now(),
	)

	if !errors.Is(err, ErrSameAllocation) {
		t.Fatalf(
			"error = %v, want ErrSameAllocation",
			err,
		)
	}
}

func TestRejectTargetOutsideEnvelope(t *testing.T) {
	source := resource.Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     2,
	}

	target := resource.Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     10,
	}

	_, err := New(
		"transition-1",
		"job-1",
		source,
		target,
		transitionRequirements(),
		time.Now(),
	)

	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf(
			"error = %v, want ErrInvalidTarget",
			err,
		)
	}
}

func TestTransitionDuration(t *testing.T) {
	start := time.Now()
	end := start.Add(12 * time.Second)

	tr := Transition{
		StartedAt:   &start,
		CompletedAt: &end,
	}

	duration, ok := tr.Duration()

	if !ok {
		t.Fatal("expected duration")
	}

	if duration != 12*time.Second {
		t.Fatalf(
			"duration = %s, want 12s",
			duration,
		)
	}
}

func TestRequiredReservationGrow(t *testing.T) {
	source := resource.Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     2,
	}

	target := resource.Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     4,
	}

	got := RequiredReservation(
		source,
		target,
	)

	want := resource.Vector{
		GPUs: 2,
	}

	if !got.Equal(want) {
		t.Fatalf(
			"RequiredReservation() = %v, want %v",
			got,
			want,
		)
	}
}

func TestRequiredReservationShrink(t *testing.T) {
	source := resource.Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     4,
	}

	target := resource.Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     2,
	}

	got := RequiredReservation(
		source,
		target,
	)

	want := resource.Vector{}

	if !got.Equal(want) {
		t.Fatalf(
			"RequiredReservation() = %v, want zero",
			got,
		)
	}
}
