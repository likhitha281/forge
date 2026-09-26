package transition

import "testing"

func TestHappyPathStateMachine(t *testing.T) {
	path := []State{
		StateRequested,
		StatePreparing,
		StateCheckpointing,
		StateReconfiguring,
		StateRestoring,
		StateResuming,
		StateCompleted,
	}

	for i := 0; i < len(path)-1; i++ {
		from := path[i]
		to := path[i+1]

		if !CanTransition(from, to) {
			t.Fatalf(
				"expected %s -> %s to be valid",
				from,
				to,
			)
		}
	}
}

func TestCannotSkipTransitionStages(t *testing.T) {
	if CanTransition(
		StateRequested,
		StateReconfiguring,
	) {
		t.Fatal(
			"REQUESTED -> RECONFIGURING should be invalid",
		)
	}
}

func TestFailureAllowedFromNonTerminalState(t *testing.T) {
	states := []State{
		StateRequested,
		StatePreparing,
		StateCheckpointing,
		StateReconfiguring,
		StateRestoring,
		StateResuming,
	}

	for _, state := range states {
		if !CanTransition(state, StateFailed) {
			t.Fatalf(
				"%s -> FAILED should be valid",
				state,
			)
		}
	}
}

func TestTerminalStateCannotTransition(t *testing.T) {
	if CanTransition(
		StateCompleted,
		StatePreparing,
	) {
		t.Fatal("COMPLETED must be terminal")
	}

	if CanTransition(
		StateFailed,
		StatePreparing,
	) {
		t.Fatal("FAILED must be terminal")
	}
}
