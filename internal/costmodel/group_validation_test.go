package costmodel

import "testing"

func TestLeaveOneStateSizeOut(
	t *testing.T,
) {
	observations := []Observation{
		{
			StateMB: 16,
			TotalUS: 100,
		},
		{
			StateMB: 16,
			TotalUS: 120,
		},
		{
			StateMB: 64,
			TotalUS: 400,
		},
		{
			StateMB: 64,
			TotalUS: 420,
		},
	}

	result :=
		LeaveOneStateSizeOut(
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

	if result.Count != 4 {
		t.Fatalf(
			"Count = %d, want 4",
			result.Count,
		)
	}
}
