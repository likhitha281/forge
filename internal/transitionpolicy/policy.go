package transitionpolicy

import (
	"time"

	"github.com/likhitha281/forge/internal/costmodel"
)

type Decision string

const (
	DecisionTransition Decision = "TRANSITION"
	DecisionStay       Decision = "STAY"
)

type Request struct {
	StateMB int

	SourceGPU int
	TargetGPU int

	RemainingRuntime time.Duration

	// Estimated speedup after the transition.
	//
	// Example:
	//
	// 1.50 means the target allocation is expected
	// to execute 1.5x as fast as the current allocation.
	Speedup float64
}

type Result struct {
	Decision Decision

	ExpectedTransitionCost time.Duration
	SafeTransitionCost     time.Duration

	ExpectedTimeSaved time.Duration
	NetBenefit        time.Duration

	Model string
}

type Policy struct {
	estimator costmodel.Estimator

	minimumNetBenefit time.Duration
}

func New(
	estimator costmodel.Estimator,
	minimumNetBenefit time.Duration,
) *Policy {
	return &Policy{
		estimator: estimator,

		minimumNetBenefit: minimumNetBenefit,
	}
}

func (p *Policy) Evaluate(
	request Request,
) Result {
	estimate :=
		p.estimator.Estimate(
			costmodel.Request{
				StateMB: request.StateMB,

				SourceGPU: request.SourceGPU,

				TargetGPU: request.TargetGPU,
			},
		)

	expectedCost :=
		time.Duration(
			estimate.ExpectedUS,
		) * time.Microsecond

	safeCost :=
		time.Duration(
			estimate.SafeUS,
		) * time.Microsecond

	result := Result{
		Decision: DecisionStay,

		ExpectedTransitionCost: expectedCost,

		SafeTransitionCost: safeCost,

		Model: estimate.Model,
	}

	if request.RemainingRuntime <= 0 ||
		request.Speedup <= 1 {

		return result
	}

	targetRuntime :=
		time.Duration(
			float64(
				request.RemainingRuntime,
			) / request.Speedup,
		)

	timeSaved :=
		request.RemainingRuntime -
			targetRuntime

	netBenefit :=
		timeSaved -
			safeCost

	result.ExpectedTimeSaved =
		timeSaved

	result.NetBenefit =
		netBenefit

	if netBenefit >=
		p.minimumNetBenefit {

		result.Decision =
			DecisionTransition
	}

	return result
}
