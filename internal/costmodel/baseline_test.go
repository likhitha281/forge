package costmodel

import "testing"

func TestMedianEstimator(
	t *testing.T,
) {
	observations := []Observation{
		{TotalUS: 100},
		{TotalUS: 200},
		{TotalUS: 300},
	}

	estimator :=
		NewMedianEstimator(
			observations,
			2.0,
		)

	got :=
		estimator.Estimate(
			Request{},
		)

	if got.ExpectedUS != 200 {
		t.Fatalf(
			"ExpectedUS = %d, want 200",
			got.ExpectedUS,
		)
	}

	if got.SafeUS != 400 {
		t.Fatalf(
			"SafeUS = %d, want 400",
			got.SafeUS,
		)
	}
}

func TestStateMedianEstimator(
	t *testing.T,
) {
	observations := []Observation{
		{
			StateMB: 16,
			TotalUS: 100,
		},
		{
			StateMB: 16,
			TotalUS: 200,
		},
		{
			StateMB: 64,
			TotalUS: 1000,
		},
	}

	estimator :=
		NewStateMedianEstimator(
			observations,
			2.0,
		)

	got :=
		estimator.Estimate(
			Request{
				StateMB: 16,
			},
		)

	if got.ExpectedUS != 150 {
		t.Fatalf(
			"ExpectedUS = %d, want 150",
			got.ExpectedUS,
		)
	}

	if got.SafeUS != 300 {
		t.Fatalf(
			"SafeUS = %d, want 300",
			got.SafeUS,
		)
	}
}
