package transition

import (
	"context"
	"errors"
	"time"
)

var ErrInvalidSimulationConfig = errors.New(
	"invalid simulated executor configuration",
)

// SimulatedExecutor provides deterministic transition costs.
//
// It gives us an execution backend for exercising the complete transition
// protocol without requiring real GPUs or checkpointable ML workloads.
type SimulatedExecutor struct {
	PrepareDuration     time.Duration
	CheckpointDuration  time.Duration
	ReconfigureDuration time.Duration
	RestoreDuration     time.Duration
	ResumeDuration      time.Duration

	CheckpointBytes int64
	RestoreBytes    int64
}

func (e SimulatedExecutor) Validate() error {
	if e.PrepareDuration < 0 ||
		e.CheckpointDuration < 0 ||
		e.ReconfigureDuration < 0 ||
		e.RestoreDuration < 0 ||
		e.ResumeDuration < 0 ||
		e.CheckpointBytes < 0 ||
		e.RestoreBytes < 0 {
		return ErrInvalidSimulationConfig
	}

	return nil
}

func (e SimulatedExecutor) Prepare(
	ctx context.Context,
	execution Execution,
) (StageResult, error) {
	return simulateStage(
		ctx,
		e.PrepareDuration,
		0,
	)
}

func (e SimulatedExecutor) Checkpoint(
	ctx context.Context,
	execution Execution,
) (StageResult, error) {
	return simulateStage(
		ctx,
		e.CheckpointDuration,
		e.CheckpointBytes,
	)
}

func (e SimulatedExecutor) Reconfigure(
	ctx context.Context,
	execution Execution,
) (StageResult, error) {
	return simulateStage(
		ctx,
		e.ReconfigureDuration,
		0,
	)
}

func (e SimulatedExecutor) Restore(
	ctx context.Context,
	execution Execution,
) (StageResult, error) {
	return simulateStage(
		ctx,
		e.RestoreDuration,
		e.RestoreBytes,
	)
}

func (e SimulatedExecutor) Resume(
	ctx context.Context,
	execution Execution,
) (StageResult, error) {
	return simulateStage(
		ctx,
		e.ResumeDuration,
		0,
	)
}

func simulateStage(
	ctx context.Context,
	duration time.Duration,
	bytesMoved int64,
) (StageResult, error) {
	if duration < 0 || bytesMoved < 0 {
		return StageResult{},
			ErrInvalidSimulationConfig
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return StageResult{}, ctx.Err()

	case <-timer.C:
		return StageResult{
			Duration:   duration,
			BytesMoved: bytesMoved,
		}, nil
	}
}
