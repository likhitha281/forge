package costmodel

import (
	"math"
	"testing"
)

func TestEvaluatePredictions(
	t *testing.T,
) {
	predictions := []Prediction{
		{
			ActualUS:   100,
			ExpectedUS: 90,
			SafeUS:     120,
		},
		{
			ActualUS:   200,
			ExpectedUS: 220,
			SafeUS:     250,
		},
		{
			ActualUS:   300,
			ExpectedUS: 270,
			SafeUS:     280,
		},
	}

	got :=
		EvaluatePredictions(
			predictions,
		)

	if got.Count != 3 {
		t.Fatalf(
			"Count = %d, want 3",
			got.Count,
		)
	}

	// Absolute errors:
	// 10, 20, 30
	//
	// MAE = 20.
	if math.Abs(got.MAEUS-20) > 0.001 {
		t.Fatalf(
			"MAEUS = %f, want 20",
			got.MAEUS,
		)
	}

	expectedRMSE :=
		math.Sqrt(
			(100.0 + 400.0 + 900.0) / 3.0,
		)

	if math.Abs(
		got.RMSEUS-expectedRMSE,
	) > 0.001 {

		t.Fatalf(
			"RMSEUS = %f, want %f",
			got.RMSEUS,
			expectedRMSE,
		)
	}

	// Two of three safe estimates cover reality.
	expectedCoverage :=
		2.0 / 3.0

	if math.Abs(
		got.SafeCoverage-
			expectedCoverage,
	) > 0.001 {

		t.Fatalf(
			"SafeCoverage = %f, want %f",
			got.SafeCoverage,
			expectedCoverage,
		)
	}
}
