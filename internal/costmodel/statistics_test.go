package costmodel

import (
	"math"
	"testing"
)

func TestSummarize(
	t *testing.T,
) {
	observations := []Observation{
		{
			TotalUS:         100_000,
			CheckpointUS:    60_000,
			RestoreUS:       40_000,
			CheckpointBytes: 16 * 1024 * 1024,
			RestoreBytes:    16 * 1024 * 1024,
		},
		{
			TotalUS:         200_000,
			CheckpointUS:    120_000,
			RestoreUS:       80_000,
			CheckpointBytes: 16 * 1024 * 1024,
			RestoreBytes:    16 * 1024 * 1024,
		},
		{
			TotalUS:         300_000,
			CheckpointUS:    180_000,
			RestoreUS:       120_000,
			CheckpointBytes: 16 * 1024 * 1024,
			RestoreBytes:    16 * 1024 * 1024,
		},
	}

	got := Summarize(
		observations,
	)

	if got.Count != 3 {
		t.Fatalf(
			"Count = %d, want 3",
			got.Count,
		)
	}

	if got.MeanUS != 200_000 {
		t.Fatalf(
			"MeanUS = %f, want 200000",
			got.MeanUS,
		)
	}

	if got.MedianUS != 200_000 {
		t.Fatalf(
			"MedianUS = %f, want 200000",
			got.MedianUS,
		)
	}

	if got.MinUS != 100_000 {
		t.Fatalf(
			"MinUS = %d, want 100000",
			got.MinUS,
		)
	}

	if got.MaxUS != 300_000 {
		t.Fatalf(
			"MaxUS = %d, want 300000",
			got.MaxUS,
		)
	}

	if math.Abs(
		got.P95US-300_000,
	) > 0.001 {
		t.Fatalf(
			"P95US = %f, want 300000",
			got.P95US,
		)
	}
}
