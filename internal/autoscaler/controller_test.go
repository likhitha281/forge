package autoscaler

import (
	"errors"
	"testing"
	"time"

	"github.com/likhitha281/forge/internal/costmodel"
	"github.com/likhitha281/forge/internal/performance"
	"github.com/likhitha281/forge/internal/transitionpolicy"
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

		SafeUS: e.safeUS,

		Model: "fixed-test",
	}
}

func TestControllerRecommendsProfitableTransition(
	t *testing.T,
) {
	performanceModel :=
		performance.New(1.0)

	now :=
		time.Now()

	if err := performanceModel.Observe(
		performance.Snapshot{
			Completed:  1000,
			Total:      10000,
			Allocation: 4,
			Rate:       200,
			Timestamp:  now,
		},
	); err != nil {
		t.Fatal(err)
	}

	if err := performanceModel.Observe(
		performance.Snapshot{
			Completed:  2000,
			Total:      10000,
			Allocation: 2,
			Rate:       100,
			Timestamp:  now,
		},
	); err != nil {
		t.Fatal(err)
	}

	policy :=
		transitionpolicy.New(
			fixedEstimator{
				expectedUS: 2_000_000,

				safeUS: 3_000_000,
			},
			5*time.Second,
		)

	controller :=
		New(
			performanceModel,
			policy,
			30*time.Second,
		)

	decision, err :=
		controller.Evaluate(
			now,
			128,
			2,
			4,
		)

	if err != nil {
		t.Fatal(err)
	}

	if decision.Policy.Decision !=
		transitionpolicy.DecisionTransition {

		t.Fatalf(
			"decision = %s, want TRANSITION",
			decision.Policy.Decision,
		)
	}

	if decision.Speedup != 2 {
		t.Fatalf(
			"speedup = %.2f, want 2",
			decision.Speedup,
		)
	}
}

func TestControllerStaysForSlowerTarget(
	t *testing.T,
) {
	performanceModel :=
		performance.New(1.0)

	now :=
		time.Now()

	if err := performanceModel.Observe(
		performance.Snapshot{
			Completed:  1000,
			Total:      10000,
			Allocation: 4,
			Rate:       80,
			Timestamp:  now,
		},
	); err != nil {
		t.Fatal(err)
	}

	if err := performanceModel.Observe(
		performance.Snapshot{
			Completed:  2000,
			Total:      10000,
			Allocation: 2,
			Rate:       100,
			Timestamp:  now,
		},
	); err != nil {
		t.Fatal(err)
	}

	policy :=
		transitionpolicy.New(
			fixedEstimator{
				expectedUS: 1_000_000,

				safeUS: 2_000_000,
			},
			0,
		)

	controller :=
		New(
			performanceModel,
			policy,
			30*time.Second,
		)

	decision, err :=
		controller.Evaluate(
			now,
			64,
			2,
			4,
		)

	if err != nil {
		t.Fatal(err)
	}

	if decision.Policy.Decision !=
		transitionpolicy.DecisionStay {

		t.Fatalf(
			"decision = %s, want STAY",
			decision.Policy.Decision,
		)
	}
}

func TestControllerRequiresTargetPerformance(
	t *testing.T,
) {
	performanceModel :=
		performance.New(1.0)

	now :=
		time.Now()

	if err := performanceModel.Observe(
		performance.Snapshot{
			Completed:  1000,
			Total:      10000,
			Allocation: 2,
			Rate:       100,
			Timestamp:  now,
		},
	); err != nil {
		t.Fatal(err)
	}

	policy :=
		transitionpolicy.New(
			fixedEstimator{
				expectedUS: 1_000_000,

				safeUS: 2_000_000,
			},
			0,
		)

	controller :=
		New(
			performanceModel,
			policy,
			30*time.Second,
		)

	_, err :=
		controller.Evaluate(
			now,
			64,
			2,
			4,
		)

	if !errors.Is(
		err,
		ErrInsufficientPerformanceData,
	) {
		t.Fatalf(
			"error = %v, want ErrInsufficientPerformanceData",
			err,
		)
	}
}

func TestControllerEnforcesCooldown(
	t *testing.T,
) {
	performanceModel :=
		performance.New(1.0)

	now :=
		time.Now()

	if err := performanceModel.Observe(
		performance.Snapshot{
			Completed:  1000,
			Total:      10000,
			Allocation: 4,
			Rate:       200,
			Timestamp:  now,
		},
	); err != nil {
		t.Fatal(err)
	}

	if err := performanceModel.Observe(
		performance.Snapshot{
			Completed:  2000,
			Total:      10000,
			Allocation: 2,
			Rate:       100,
			Timestamp:  now,
		},
	); err != nil {
		t.Fatal(err)
	}

	policy :=
		transitionpolicy.New(
			fixedEstimator{
				expectedUS: 1_000_000,

				safeUS: 2_000_000,
			},
			0,
		)

	controller :=
		New(
			performanceModel,
			policy,
			30*time.Second,
		)

	controller.RecordTransition(
		now,
	)

	_, err :=
		controller.Evaluate(
			now.Add(
				10*time.Second,
			),
			64,
			2,
			4,
		)

	if !errors.Is(
		err,
		ErrCooldownActive,
	) {
		t.Fatalf(
			"error = %v, want ErrCooldownActive",
			err,
		)
	}

	_, err =
		controller.Evaluate(
			now.Add(
				31*time.Second,
			),
			64,
			2,
			4,
		)

	if err != nil {
		t.Fatalf(
			"Evaluate after cooldown: %v",
			err,
		)
	}
}
