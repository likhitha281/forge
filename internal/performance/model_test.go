package performance

import (
	"math"
	"testing"
	"time"
)

func TestModelEWMA(
	t *testing.T,
) {
	model :=
		New(0.5)

	err :=
		model.Observe(
			Snapshot{
				Completed:  100,
				Total:      1000,
				Allocation: 2,
				Rate:       100,
				Timestamp:  time.Now(),
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	err =
		model.Observe(
			Snapshot{
				Completed:  200,
				Total:      1000,
				Allocation: 2,
				Rate:       200,
				Timestamp:  time.Now(),
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	rate, ok :=
		model.Rate(2)

	if !ok {
		t.Fatal(
			"missing rate for allocation 2",
		)
	}

	if math.Abs(rate-150) > 0.001 {
		t.Fatalf(
			"rate = %.2f, want 150",
			rate,
		)
	}
}

func TestModelSpeedup(
	t *testing.T,
) {
	model :=
		New(0.3)

	if err := model.Observe(
		Snapshot{
			Completed:  100,
			Total:      1000,
			Allocation: 2,
			Rate:       100,
		},
	); err != nil {
		t.Fatal(err)
	}

	if err := model.Observe(
		Snapshot{
			Completed:  200,
			Total:      1000,
			Allocation: 4,
			Rate:       175,
		},
	); err != nil {
		t.Fatal(err)
	}

	speedup, err :=
		model.Speedup(
			2,
			4,
		)

	if err != nil {
		t.Fatal(err)
	}

	if math.Abs(speedup-1.75) >
		0.001 {

		t.Fatalf(
			"speedup = %.2f, want 1.75",
			speedup,
		)
	}
}

func TestModelRemainingRuntime(
	t *testing.T,
) {
	model :=
		New(0.3)

	if err := model.Observe(
		Snapshot{
			Completed:  400,
			Total:      1000,
			Allocation: 2,
			Rate:       100,
		},
	); err != nil {
		t.Fatal(err)
	}

	estimate, err :=
		model.CurrentEstimate()

	if err != nil {
		t.Fatal(err)
	}

	if estimate.RemainingWork != 600 {
		t.Fatalf(
			"RemainingWork = %d, want 600",
			estimate.RemainingWork,
		)
	}

	if estimate.RemainingRuntime !=
		6*time.Second {

		t.Fatalf(
			"RemainingRuntime = %s, want 6s",
			estimate.RemainingRuntime,
		)
	}
}

func TestRestoreResetDoesNotDestroyRate(
	t *testing.T,
) {
	model :=
		New(0.3)

	if err := model.Observe(
		Snapshot{
			Completed:  500,
			Total:      1000,
			Allocation: 4,
			Rate:       200,
		},
	); err != nil {
		t.Fatal(err)
	}

	if err := model.Observe(
		Snapshot{
			Completed:  400,
			Total:      1000,
			Allocation: 4,
			Rate:       0,
		},
	); err != nil {
		t.Fatal(err)
	}

	rate, ok :=
		model.Rate(4)

	if !ok {
		t.Fatal(
			"rate disappeared after reset",
		)
	}

	if rate != 200 {
		t.Fatalf(
			"rate = %.2f, want 200",
			rate,
		)
	}
}
