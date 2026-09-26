package costmodel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCSV(
	t *testing.T,
) {
	path := filepath.Join(
		t.TempDir(),
		"observations.csv",
	)

	data := `state_mb,source_gpu,target_gpu,prepare_us,checkpoint_us,reconfigure_us,restore_us,resume_us,total_us,checkpoint_bytes,restore_bytes,bytes_moved
16,2,4,1000,65000,2000,25000,1000,94000,16777224,16777224,33554448
64,4,2,2000,310000,3000,198000,2000,515000,67108872,67108872,134217744
`

	if err := os.WriteFile(
		path,
		[]byte(data),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	observations, err :=
		LoadCSV(path)

	if err != nil {
		t.Fatalf(
			"LoadCSV(): %v",
			err,
		)
	}

	if len(observations) != 2 {
		t.Fatalf(
			"len = %d, want 2",
			len(observations),
		)
	}

	first := observations[0]

	if first.StateMB != 16 ||
		first.SourceGPU != 2 ||
		first.TargetGPU != 4 {

		t.Fatalf(
			"unexpected first observation: %+v",
			first,
		)
	}

	if first.TotalUS != 94_000 {
		t.Fatalf(
			"TotalUS = %d, want 94000",
			first.TotalUS,
		)
	}

	if first.BytesMoved != 33_554_448 {
		t.Fatalf(
			"BytesMoved = %d, want 33554448",
			first.BytesMoved,
		)
	}
}
