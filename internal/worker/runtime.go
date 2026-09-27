package worker

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/likhitha281/forge/internal/resource"
)

var (
	ErrRuntimeAlreadyExists = errors.New(
		"runtime already exists",
	)

	ErrRuntimeNotFound = errors.New(
		"runtime not found",
	)
)

// Runtime represents one locally executing Forge job.
//
// It deliberately separates the worker's local process state from the
// coordinator's persisted job state.
type Runtime struct {
	JobID string

	mu sync.RWMutex

	cmd *exec.Cmd

	allocation resource.Vector

	startedAt time.Time

	controlDir string

	checkpointPath string
	checkpointDone string
	restoreDone    string

	checkpointable bool

	allocationPath string

	progressPath string

	command string
	maxGPUs int32
}

func (r *Runtime) SetWorkloadMetadata(
	command string,
	maxGPUs int32,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.command = command
	r.maxGPUs = maxGPUs
}

func (r *Runtime) Command() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.command
}

func (r *Runtime) MaxGPUs() int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.maxGPUs
}

func NewRuntime(
	jobID string,
	cmd *exec.Cmd,
	allocation resource.Vector,
	now time.Time,
) *Runtime {
	return &Runtime{
		JobID:      jobID,
		cmd:        cmd,
		allocation: allocation,
		startedAt:  now,
	}
}

func (r *Runtime) Allocation() resource.Vector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.allocation
}

func (r *Runtime) SetAllocation(
	allocation resource.Vector,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.allocation = allocation
}

func (r *Runtime) StartedAt() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.startedAt
}

func (r *Runtime) PID() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.cmd == nil ||
		r.cmd.Process == nil {
		return 0
	}

	return r.cmd.Process.Pid
}

func (r *Runtime) Signal(
	signal os.Signal,
) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.cmd == nil ||
		r.cmd.Process == nil {
		return errors.New(
			"runtime process is not running",
		)
	}

	return r.cmd.Process.Signal(signal)
}

func (r *Runtime) SetControlPaths(
	controlDir string,
	checkpointPath string,
	checkpointDone string,
	restoreDone string,
	allocationPath string,
	progressPath string,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.progressPath =
		progressPath

	r.controlDir =
		controlDir

	r.checkpointPath =
		checkpointPath

	r.checkpointDone =
		checkpointDone

	r.restoreDone =
		restoreDone

	r.allocationPath =
		allocationPath
}

func (r *Runtime) ProgressPath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.progressPath
}

func (r *Runtime) ControlDir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.controlDir
}

func (r *Runtime) CheckpointPath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.checkpointPath
}

func (r *Runtime) CheckpointDonePath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.checkpointDone
}

func (r *Runtime) RestoreDonePath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.restoreDone
}

func (r *Runtime) AllocationPath() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.allocationPath
}

func (r *Runtime) SetCheckpointable(
	value bool,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.checkpointable = value
}

func (r *Runtime) Checkpointable() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.checkpointable
}
