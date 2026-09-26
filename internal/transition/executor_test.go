package transition

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/likhitha281/forge/internal/resource"
)

func testExecution() Execution {
	return Execution{
		TransitionID: "transition-1",
		JobID:        "job-1",
		WorkerID:     "worker-1",

		Source: resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     2,
		},

		Target: resource.Vector{
			CPUCores: 4,
			MemoryMB: 8192,
			GPUs:     4,
		},
	}
}

func TestSimulatedExecutor(t *testing.T) {
	executor := SimulatedExecutor{
		PrepareDuration:     1 * time.Millisecond,
		CheckpointDuration:  2 * time.Millisecond,
		ReconfigureDuration: 3 * time.Millisecond,
		RestoreDuration:     4 * time.Millisecond,
		ResumeDuration:      5 * time.Millisecond,

		CheckpointBytes: 1024,
		RestoreBytes:    1024,
	}

	if err := executor.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}

	execution := testExecution()
	ctx := context.Background()

	prepare, err := executor.Prepare(ctx, execution)
	if err != nil {
		t.Fatalf("Prepare(): %v", err)
	}

	checkpoint, err := executor.Checkpoint(ctx, execution)
	if err != nil {
		t.Fatalf("Checkpoint(): %v", err)
	}

	reconfigure, err := executor.Reconfigure(ctx, execution)
	if err != nil {
		t.Fatalf("Reconfigure(): %v", err)
	}

	restore, err := executor.Restore(ctx, execution)
	if err != nil {
		t.Fatalf("Restore(): %v", err)
	}

	resume, err := executor.Resume(ctx, execution)
	if err != nil {
		t.Fatalf("Resume(): %v", err)
	}

	result := ExecutionResult{
		Prepare:     prepare,
		Checkpoint:  checkpoint,
		Reconfigure: reconfigure,
		Restore:     restore,
		Resume:      resume,
	}

	if got, want :=
		result.TotalDuration(),
		15*time.Millisecond; got != want {
		t.Fatalf(
			"TotalDuration() = %v, want %v",
			got,
			want,
		)
	}

	if got, want :=
		result.TotalBytesMoved(),
		int64(2048); got != want {
		t.Fatalf(
			"TotalBytesMoved() = %d, want %d",
			got,
			want,
		)
	}
}

func TestSimulatedExecutorCancellation(t *testing.T) {
	executor := SimulatedExecutor{
		CheckpointDuration: time.Second,
	}

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	cancel()

	_, err := executor.Checkpoint(
		ctx,
		testExecution(),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"error = %v, want context.Canceled",
			err,
		)
	}
}

func TestSimulatedExecutorRejectsInvalidConfiguration(
	t *testing.T,
) {
	executor := SimulatedExecutor{
		CheckpointDuration: -1 * time.Second,
	}

	if !errors.Is(
		executor.Validate(),
		ErrInvalidSimulationConfig,
	) {
		t.Fatal(
			"expected ErrInvalidSimulationConfig",
		)
	}
}
