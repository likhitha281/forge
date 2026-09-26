package costmodel

import "testing"

func TestHybridEstimatorUsesEmpiricalForKnownState(
	t *testing.T,
) {
	observations := []Observation{
		{
			StateMB:         16,
			TotalUS:         100,
			CheckpointUS:    60,
			RestoreUS:       40,
			CheckpointBytes: 16,
			RestoreBytes:    16,
		},
		{
			StateMB:         16,
			TotalUS:         200,
			CheckpointUS:    120,
			RestoreUS:       80,
			CheckpointBytes: 16,
			RestoreBytes:    16,
		},
		{
			StateMB:         16,
			TotalUS:         300,
			CheckpointUS:    180,
			RestoreUS:       120,
			CheckpointBytes: 16,
			RestoreBytes:    16,
		},
	}

	estimator :=
		NewHybridEstimator(
			observations,
			2.0,
			3,
		)

	got :=
		estimator.Estimate(
			Request{
				StateMB: 16,
			},
		)

	if got.ExpectedUS != 200 {
		t.Fatalf(
			"ExpectedUS = %d, want 200",
			got.ExpectedUS,
		)
	}

	if got.Model !=
		"hybrid-empirical" {

		t.Fatalf(
			"Model = %q, want hybrid-empirical",
			got.Model,
		)
	}
}

func TestHybridEstimatorUsesThroughputForUnknownState(
	t *testing.T,
) {
	observations := []Observation{
		{
			StateMB:         16,
			TotalUS:         150_000,
			CheckpointUS:    100_000,
			RestoreUS:       50_000,
			CheckpointBytes: 10 * 1024 * 1024,
			RestoreBytes:    10 * 1024 * 1024,
		},
		{
			StateMB:         64,
			TotalUS:         300_000,
			CheckpointUS:    200_000,
			RestoreUS:       100_000,
			CheckpointBytes: 20 * 1024 * 1024,
			RestoreBytes:    20 * 1024 * 1024,
		},
	}

	estimator :=
		NewHybridEstimator(
			observations,
			2.0,
			3,
		)

	got :=
		estimator.Estimate(
			Request{
				StateMB: 30,
			},
		)

	if got.Model !=
		"hybrid-throughput" {

		t.Fatalf(
			"Model = %q, want hybrid-throughput",
			got.Model,
		)
	}

	if got.ExpectedUS != 450_000 {
		t.Fatalf(
			"ExpectedUS = %d, want 450000",
			got.ExpectedUS,
		)
	}
}
