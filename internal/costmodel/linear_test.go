package costmodel

import "testing"

func TestLinearEstimator(
	t *testing.T,
) {
	observations := []Observation{
		{
			StateMB: 10,
			TotalUS: 100,
		},
		{
			StateMB: 20,
			TotalUS: 200,
		},
		{
			StateMB: 30,
			TotalUS: 300,
		},
	}

	estimator :=
		NewLinearEstimator(
			observations,
			2.0,
		)

	got :=
		estimator.Estimate(
			Request{
				StateMB: 40,
			},
		)

	if got.ExpectedUS != 400 {
		t.Fatalf(
			"ExpectedUS = %d, want 400",
			got.ExpectedUS,
		)
	}

	if got.SafeUS != 800 {
		t.Fatalf(
			"SafeUS = %d, want 800",
			got.SafeUS,
		)
	}
}
