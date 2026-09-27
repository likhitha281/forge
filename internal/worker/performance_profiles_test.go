package worker

import "testing"

func TestPerformanceProfilesReuseModel(
	t *testing.T,
) {
	profiles :=
		NewPerformanceProfiles(
			0.3,
		)

	first :=
		profiles.Model(
			"elastic",
		)

	second :=
		profiles.Model(
			"elastic",
		)

	if first != second {
		t.Fatal(
			"expected shared performance model",
		)
	}
}
