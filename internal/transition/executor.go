package transition

import (
	"context"
	"time"

	"github.com/likhitha281/forge/internal/resource"
)

// Execution describes everything an executor needs to perform one
// reconfiguration.
//
// Keeping this independent of PostgreSQL/gRPC means executors can later
// represent simulations, containers, distributed training frameworks, etc.
type Execution struct {
	TransitionID string
	JobID        string
	WorkerID     string

	Source resource.Vector
	Target resource.Vector
}

// StageResult records the observable cost of one transition stage.
type StageResult struct {
	Duration   time.Duration
	BytesMoved int64
}

// Executor performs the workload-specific mechanics of a transition.
//
// The transition runner owns state-machine progression. The Executor only
// performs the operation associated with each stage.
type Executor interface {
	Prepare(
		ctx context.Context,
		execution Execution,
	) (StageResult, error)

	Checkpoint(
		ctx context.Context,
		execution Execution,
	) (StageResult, error)

	Reconfigure(
		ctx context.Context,
		execution Execution,
	) (StageResult, error)

	Restore(
		ctx context.Context,
		execution Execution,
	) (StageResult, error)

	Resume(
		ctx context.Context,
		execution Execution,
	) (StageResult, error)
}

// ExecutionResult contains the measured cost of a complete transition.
type ExecutionResult struct {
	Prepare     StageResult
	Checkpoint  StageResult
	Reconfigure StageResult
	Restore     StageResult
	Resume      StageResult
}

func (r ExecutionResult) TotalDuration() time.Duration {
	return r.Prepare.Duration +
		r.Checkpoint.Duration +
		r.Reconfigure.Duration +
		r.Restore.Duration +
		r.Resume.Duration
}

func (r ExecutionResult) TotalBytesMoved() int64 {
	return r.Prepare.BytesMoved +
		r.Checkpoint.BytesMoved +
		r.Reconfigure.BytesMoved +
		r.Restore.BytesMoved +
		r.Resume.BytesMoved
}
