package transition

import (
	"testing"
	"time"
)

func TestMetricsFromExecutionResult(
	t *testing.T,
) {
	result := ExecutionResult{
		Prepare: StageResult{
			Duration: 100 * time.Millisecond,
		},

		Checkpoint: StageResult{
			Duration:   300 * time.Millisecond,
			BytesMoved: 64 * 1024 * 1024,
		},

		Reconfigure: StageResult{
			Duration: 200 * time.Millisecond,
		},

		Restore: StageResult{
			Duration:   300 * time.Millisecond,
			BytesMoved: 64 * 1024 * 1024,
		},

		Resume: StageResult{
			Duration: 100 * time.Millisecond,
		},
	}

	metrics :=
		MetricsFromExecutionResult(result)

	if got, want :=
		metrics.TotalDuration(),
		time.Second; got != want {
		t.Fatalf(
			"TotalDuration() = %v, want %v",
			got,
			want,
		)
	}

	if got, want :=
		metrics.TotalBytesMoved(),
		int64(128*1024*1024); got != want {
		t.Fatalf(
			"TotalBytesMoved() = %d, want %d",
			got,
			want,
		)
	}
}
