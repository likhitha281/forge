package transitionpolicy

import (
	"testing"
	"time"

	"github.com/likhitha281/forge/internal/costmodel"
)

type fixedEstimator struct {
	expectedUS int64
	safeUS     int64
}

func (e fixedEstimator) Estimate(
	request costmodel.Request,
) costmodel.Estimate {
	return costmodel.Estimate{
		ExpectedUS: e.expectedUS,
		SafeUS:     e.safeUS,
		Model:      "fixed-test",
	}
}

func TestPolicyTransitionsWhenBenefitExceedsCost(
	t *testing.T,
) {
	policy :=
		New(
			fixedEstimator{
				expectedUS: 2_000_000,

				safeUS: 3_000_000,
			},
			5*time.Second,
		)

	result :=
		policy.Evaluate(
			Request{
				StateMB: 128,

				SourceGPU: 2,
				TargetGPU: 4,

				RemainingRuntime: 60 * time.Second,

				Speedup: 1.5,
			},
		)

	if result.Decision !=
		DecisionTransition {

		t.Fatalf(
			"Decision = %s, want TRANSITION",
			result.Decision,
		)
	}
}

func TestPolicyStaysWhenTransitionCostExceedsBenefit(
	t *testing.T,
) {
	policy :=
		New(
			fixedEstimator{
				expectedUS: 2_000_000,

				safeUS: 3_000_000,
			},
			0,
		)

	result :=
		policy.Evaluate(
			Request{
				StateMB: 128,

				SourceGPU: 2,
				TargetGPU: 4,

				RemainingRuntime: 5 * time.Second,

				Speedup: 1.5,
			},
		)

	if result.Decision !=
		DecisionStay {

		t.Fatalf(
			"Decision = %s, want STAY",
			result.Decision,
		)
	}
}

func TestPolicyStaysWithoutSpeedup(
	t *testing.T,
) {
	policy :=
		New(
			fixedEstimator{
				expectedUS: 1_000,
				safeUS:     2_000,
			},
			0,
		)

	result :=
		policy.Evaluate(
			Request{
				StateMB: 16,

				SourceGPU: 2,
				TargetGPU: 4,

				RemainingRuntime: 60 * time.Second,

				Speedup: 1.0,
			},
		)

	if result.Decision !=
		DecisionStay {

		t.Fatalf(
			"Decision = %s, want STAY",
			result.Decision,
		)
	}
}
