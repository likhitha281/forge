package costmodel

import "testing"

func TestThroughputEstimator(
	t *testing.T,
) {
	observations := []Observation{
		{
			CheckpointUS:    100_000,
			RestoreUS:       50_000,
			CheckpointBytes: 10 * 1024 * 1024,
			RestoreBytes:    10 * 1024 * 1024,
		},
		{
			CheckpointUS:    200_000,
			RestoreUS:       100_000,
			CheckpointBytes: 20 * 1024 * 1024,
			RestoreBytes:    20 * 1024 * 1024,
		},
	}

	estimator :=
		NewThroughputEstimator(
			observations,
			2.0,
		)

	got :=
		estimator.Estimate(
			Request{
				StateMB:   30,
				SourceGPU: 2,
				TargetGPU: 4,
			},
		)

	// Both observations imply:
	//
	// checkpoint = 100 MB/s
	// restore    = 200 MB/s
	//
	// 30/100 + 30/200 = 0.45 seconds.
	if got.ExpectedUS != 450_000 {
		t.Fatalf(
			"ExpectedUS = %d, want 450000",
			got.ExpectedUS,
		)
	}

	if got.SafeUS != 900_000 {
		t.Fatalf(
			"SafeUS = %d, want 900000",
			got.SafeUS,
		)
	}
}
