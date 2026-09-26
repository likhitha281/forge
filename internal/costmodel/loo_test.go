package costmodel

import "testing"

func TestLeaveOneOut(
	t *testing.T,
) {
	observations := []Observation{
		{TotalUS: 100},
		{TotalUS: 200},
		{TotalUS: 300},
	}

	result :=
		LeaveOneOut(
			observations,
			func(
				training []Observation,
			) Estimator {
				return NewMedianEstimator(
					training,
					2.0,
				)
			},
		)

	if result.Count != 3 {
		t.Fatalf(
			"Count = %d, want 3",
			result.Count,
		)
	}
}
