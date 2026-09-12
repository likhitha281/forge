package retry

import (
	"testing"
	"time"
)

func TestBackoffWithinExpectedRange(t *testing.T) {
	tests := []struct {
		attempt int
		base    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 64 * time.Second},
	}

	for _, tt := range tests {
		t.Run(
			time.Duration(tt.attempt).String(),
			func(t *testing.T) {
				for i := 0; i < 100; i++ {
					got := Backoff(tt.attempt)

					minimum := tt.base
					maximum := tt.base + tt.base/2

					if got < minimum || got > maximum {
						t.Fatalf(
							"Backoff(%d) = %v; expected between %v and %v",
							tt.attempt,
							got,
							minimum,
							maximum,
						)
					}
				}
			},
		)
	}
}

func TestBackoffNegativeAttemptUsesZero(t *testing.T) {
	got := Backoff(-10)

	if got < time.Second || got > 1500*time.Millisecond {
		t.Fatalf(
			"Backoff(-10) = %v; expected zero-attempt range",
			got,
		)
	}
}

func TestBackoffCapsAtAttemptSix(t *testing.T) {
	for i := 0; i < 100; i++ {
		got := Backoff(100)

		if got < 64*time.Second || got > 96*time.Second {
			t.Fatalf(
				"Backoff(100) = %v; expected capped attempt-six range",
				got,
			)
		}
	}
}
