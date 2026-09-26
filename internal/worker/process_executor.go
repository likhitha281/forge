//go:build linux

package worker

import (
	"context"
	"errors"
	"os"
	"strconv"
	"syscall"
	"time"

	transitionmodel "github.com/likhitha281/forge/internal/transition"
)

var (
	ErrProcessExecutorTimeout = errors.New(
		"process executor operation timed out",
	)

	ErrCheckpointMissing = errors.New(
		"checkpoint file missing",
	)
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
	start := time.Now()

	if e.Runtime == nil {
		return transitionmodel.StageResult{},
			errors.New("runtime is nil")
	}

	if e.Runtime.PID() == 0 {
		return transitionmodel.StageResult{},
			errors.New("runtime process is not running")
	}

	return transitionmodel.StageResult{
		Duration: time.Since(start),
	}, nil
}

func (e *ProcessExecutor) Checkpoint(
	ctx context.Context,
	execution transitionmodel.Execution,
) (transitionmodel.StageResult, error) {
	if e.Runtime == nil {
		return transitionmodel.StageResult{},
			errors.New("runtime is nil")
	}

	if err := removeIfExists(
		e.CheckpointDone,
	); err != nil {
		return transitionmodel.StageResult{}, err
	}

	start := time.Now()

	if err := e.Runtime.Signal(
		syscall.SIGUSR1,
	); err != nil {
		return transitionmodel.StageResult{}, err
	}

	if err := e.waitForFile(
		ctx,
		e.CheckpointDone,
	); err != nil {
		return transitionmodel.StageResult{}, err
	}

	info, err := os.Stat(
		e.CheckpointPath,
	)
	if errors.Is(err, os.ErrNotExist) {
		return transitionmodel.StageResult{},
			ErrCheckpointMissing
	}

	if err != nil {
		return transitionmodel.StageResult{}, err
	}

	return transitionmodel.StageResult{
		Duration:   time.Since(start),
		BytesMoved: info.Size(),
	}, nil
}

func (e *ProcessExecutor) Reconfigure(
	ctx context.Context,
	execution transitionmodel.Execution,
) (transitionmodel.StageResult, error) {
	if e.Runtime == nil {
		return transitionmodel.StageResult{},
			errors.New("runtime is nil")
	}

	select {
	case <-ctx.Done():
		return transitionmodel.StageResult{},
			ctx.Err()

	default:
	}

	start := time.Now()

	allocationPath :=
		e.Runtime.AllocationPath()

	if allocationPath != "" {
		tmp :=
			allocationPath + ".tmp"

		value :=
			strconv.Itoa(
				int(execution.Target.GPUs),
			) + "\n"

		if err := os.WriteFile(
			tmp,
			[]byte(value),
			0o644,
		); err != nil {
			return transitionmodel.StageResult{},
				err
		}

		if err := os.Rename(
			tmp,
			allocationPath,
		); err != nil {
			_ = os.Remove(tmp)

			return transitionmodel.StageResult{},
				err
		}
	}

	e.Runtime.SetAllocation(
		execution.Target,
	)

	return transitionmodel.StageResult{
		Duration: time.Since(start),
	}, nil
}

func (e *ProcessExecutor) Restore(
	ctx context.Context,
	execution transitionmodel.Execution,
) (transitionmodel.StageResult, error) {
	if e.Runtime == nil {
		return transitionmodel.StageResult{},
			errors.New("runtime is nil")
	}

	if err := removeIfExists(
		e.RestoreDone,
	); err != nil {
		return transitionmodel.StageResult{}, err
	}

	start := time.Now()

	if err := e.Runtime.Signal(
		syscall.SIGUSR2,
	); err != nil {
		return transitionmodel.StageResult{}, err
	}

	if err := e.waitForFile(
		ctx,
		e.RestoreDone,
	); err != nil {
		return transitionmodel.StageResult{}, err
	}

	info, err := os.Stat(
		e.CheckpointPath,
	)
	if errors.Is(err, os.ErrNotExist) {
		return transitionmodel.StageResult{},
			ErrCheckpointMissing
	}

	if err != nil {
		return transitionmodel.StageResult{}, err
	}

	return transitionmodel.StageResult{
		Duration:   time.Since(start),
		BytesMoved: info.Size(),
	}, nil
}

func (e *ProcessExecutor) Resume(
	ctx context.Context,
	execution transitionmodel.Execution,
) (transitionmodel.StageResult, error) {
	start := time.Now()

	if e.Runtime == nil ||
		e.Runtime.PID() == 0 {
		return transitionmodel.StageResult{},
			errors.New("runtime process is not running")
	}

	return transitionmodel.StageResult{
		Duration: time.Since(start),
	}, nil
}

func (e *ProcessExecutor) waitForFile(
	ctx context.Context,
	path string,
) error {
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	interval := e.PollInterval
	if interval <= 0 {
		interval = 10 * time.Millisecond
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-timer.C:
			return ErrProcessExecutorTimeout

		case <-ticker.C:
			_, err := os.Stat(path)

			if err == nil {
				return nil
			}

			if !errors.Is(
				err,
				os.ErrNotExist,
			) {
				return err
			}
		}
	}
}

func removeIfExists(
	path string,
) error {
	err := os.Remove(path)

	if err == nil ||
		errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}
