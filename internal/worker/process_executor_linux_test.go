//go:build linux

package worker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/likhitha281/forge/internal/resource"
	transitionmodel "github.com/likhitha281/forge/internal/transition"
)

func TestProcessExecutorReconfigureUpdatesAllocationFile(
	t *testing.T,
) {
	dir :=
		t.TempDir()

	allocationPath :=
		filepath.Join(
			dir,
			"allocation",
		)

	if err := os.WriteFile(
		allocationPath,
		[]byte("2\n"),
		0o644,
	); err != nil {
		t.Fatalf(
			"write initial allocation: %v",
			err,
		)
	}

	runtime :=
		NewRuntime(
			"job-1",
			&exec.Cmd{},
			resource.Vector{
				CPUCores: 2,
				MemoryMB: 1024,
				GPUs:     2,
			},
			time.Now(),
		)

	runtime.SetControlPaths(
		dir,
		filepath.Join(
			dir,
			"checkpoint.bin",
		),
		filepath.Join(
			dir,
			"checkpoint.done",
		),
		filepath.Join(
			dir,
			"restore.done",
		),
		allocationPath,
		filepath.Join(
			dir,
			"progress.json",
		),
	)

	executor :=
		&ProcessExecutor{
			Runtime: runtime,
		}

	result, err :=
		executor.Reconfigure(
			context.Background(),
			transitionmodel.Execution{
				Target: resource.Vector{
					CPUCores: 2,
					MemoryMB: 1024,
					GPUs:     4,
				},
			},
		)

	if err != nil {
		t.Fatalf(
			"Reconfigure(): %v",
			err,
		)
	}

	if result.Duration < 0 {
		t.Fatalf(
			"Duration = %s",
			result.Duration,
		)
	}

	data, err :=
		os.ReadFile(
			allocationPath,
		)

	if err != nil {
		t.Fatalf(
			"read allocation: %v",
			err,
		)
	}

	if string(data) != "4\n" {
		t.Fatalf(
			"allocation file = %q, want %q",
			string(data),
			"4\n",
		)
	}

	allocation :=
		runtime.Allocation()

	if allocation.GPUs != 4 {
		t.Fatalf(
			"runtime GPUs = %d, want 4",
			allocation.GPUs,
		)
	}
}
