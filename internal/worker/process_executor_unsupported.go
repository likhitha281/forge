//go:build !linux

package worker

import (
	"context"
	"errors"
	"time"

	transitionmodel "github.com/likhitha281/forge/internal/transition"
)

var ErrProcessExecutorUnsupported = errors.New(
	"process executor is supported only on linux",
)

type ProcessExecutor struct {
	Runtime *Runtime

	CheckpointPath string
	CheckpointDone string
	RestoreDone    string

	Timeout      time.Duration
	PollInterval time.Duration
}

func (e *ProcessExecutor) Prepare(
	ctx context.Context,
	execution transitionmodel.Execution,
) (transitionmodel.StageResult, error) {
	return transitionmodel.StageResult{},
		ErrProcessExecutorUnsupported
}

func (e *ProcessExecutor) Checkpoint(
	ctx context.Context,
	execution transitionmodel.Execution,
) (transitionmodel.StageResult, error) {
	return transitionmodel.StageResult{},
		ErrProcessExecutorUnsupported
}

func (e *ProcessExecutor) Reconfigure(
	ctx context.Context,
	execution transitionmodel.Execution,
) (transitionmodel.StageResult, error) {
	return transitionmodel.StageResult{},
		ErrProcessExecutorUnsupported
}

func (e *ProcessExecutor) Restore(
	ctx context.Context,
	execution transitionmodel.Execution,
) (transitionmodel.StageResult, error) {
	return transitionmodel.StageResult{},
		ErrProcessExecutorUnsupported
}

func (e *ProcessExecutor) Resume(
	ctx context.Context,
	execution transitionmodel.Execution,
) (transitionmodel.StageResult, error) {
	return transitionmodel.StageResult{},
		ErrProcessExecutorUnsupported
}
