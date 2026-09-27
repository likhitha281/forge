package performance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSnapshot(
	t *testing.T,
) {
	dir :=
		t.TempDir()

	path :=
		filepath.Join(
			dir,
			"progress.json",
		)

	data := []byte(
		`{"completed":250,"total":1000,"allocation":2,"rate":125.5,"timestamp":"2026-09-26T22:00:00Z"}`,
	)

	if err := os.WriteFile(
		path,
		data,
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	snapshot, err :=
		ReadSnapshot(path)

	if err != nil {
		t.Fatal(err)
	}

	if snapshot.Completed != 250 {
		t.Fatalf(
			"Completed = %d, want 250",
			snapshot.Completed,
		)
	}

	if snapshot.Total != 1000 {
		t.Fatalf(
			"Total = %d, want 1000",
			snapshot.Total,
		)
	}

	if snapshot.Allocation != 2 {
		t.Fatalf(
			"Allocation = %d, want 2",
			snapshot.Allocation,
		)
	}

	if snapshot.Rate != 125.5 {
		t.Fatalf(
			"Rate = %.2f, want 125.5",
			snapshot.Rate,
		)
	}

	if snapshot.Timestamp.IsZero() {
		t.Fatal(
			"Timestamp should not be zero",
		)
	}
}

func TestReadSnapshotRejectsInvalidData(
	t *testing.T,
) {
	dir :=
		t.TempDir()

	path :=
		filepath.Join(
			dir,
			"progress.json",
		)

	data := []byte(
		`{"completed":1100,"total":1000,"allocation":2,"rate":125.5}`,
	)

	if err := os.WriteFile(
		path,
		data,
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	_, err :=
		ReadSnapshot(path)

	if err == nil {
		t.Fatal(
			"expected invalid snapshot error",
		)
	}
}
