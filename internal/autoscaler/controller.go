package autoscaler

import (
	"errors"
	"time"

	"github.com/likhitha281/forge/internal/performance"
	"github.com/likhitha281/forge/internal/transitionpolicy"
)

var (
	ErrInsufficientPerformanceData = errors.New(
		"insufficient performance data",
	)

	ErrCooldownActive = errors.New(
		"autoscaler cooldown active",
	)
)

type Decision struct {
	CurrentGPU int
	TargetGPU  int

	CurrentRate float64
	TargetRate  float64
	Speedup     float64

	RemainingRuntime time.Duration

	Policy transitionpolicy.Result
}

type Controller struct {
	performance *performance.Model
	policy      *transitionpolicy.Policy

	cooldown       time.Duration
	lastTransition time.Time
}

func New(
	performanceModel *performance.Model,
	policy *transitionpolicy.Policy,
	cooldown time.Duration,
) *Controller {
	return &Controller{
		performance: performanceModel,
		policy:      policy,
		cooldown:    cooldown,
	}
}

func (c *Controller) Evaluate(
	now time.Time,
	stateMB int,
	currentGPU int,
	targetGPU int,
) (Decision, error) {
	if c.performance == nil {
		return Decision{},
			ErrInsufficientPerformanceData
	}

	if c.policy == nil {
		return Decision{},
			errors.New("transition policy is nil")
	}

	if currentGPU <= 0 ||
		targetGPU <= 0 ||
		currentGPU == targetGPU {

		return Decision{},
			errors.New("invalid GPU transition")
	}

	if !c.lastTransition.IsZero() &&
		c.cooldown > 0 &&
		now.Sub(c.lastTransition) <
			c.cooldown {

		return Decision{},
			ErrCooldownActive
	}

	estimate, err :=
		c.performance.CurrentEstimate()

	if err != nil {
		return Decision{},
			ErrInsufficientPerformanceData
	}

	if estimate.CurrentAllocation !=
		currentGPU {

		return Decision{},
			ErrInsufficientPerformanceData
	}

	currentRate, currentOK :=
		c.performance.Rate(
			currentGPU,
		)

	targetRate, targetOK :=
		c.performance.Rate(
			targetGPU,
		)

	if !currentOK ||
		!targetOK ||
		currentRate <= 0 ||
		targetRate <= 0 {

		return Decision{},
			ErrInsufficientPerformanceData
	}

	speedup :=
		targetRate /
			currentRate

	result :=
		c.policy.Evaluate(
			transitionpolicy.Request{
				StateMB: stateMB,

				SourceGPU: currentGPU,

				TargetGPU: targetGPU,

				RemainingRuntime: estimate.RemainingRuntime,

				Speedup: speedup,
			},
		)

	return Decision{
		CurrentGPU: currentGPU,

		TargetGPU: targetGPU,

		CurrentRate: currentRate,

		TargetRate: targetRate,

		Speedup: speedup,

		RemainingRuntime: estimate.RemainingRuntime,

		Policy: result,
	}, nil
}

func (c *Controller) RecordTransition(
	now time.Time,
) {
	c.lastTransition =
		now
}
