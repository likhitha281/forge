package resource

import (
	"errors"
	"testing"
)

func TestGPUEnvelopeValidate(t *testing.T) {
	tests := []struct {
		name     string
		envelope GPUEnvelope
		wantErr  bool
	}{
		{
			name: "valid malleable envelope",
			envelope: GPUEnvelope{
				Min:       1,
				Preferred: 4,
				Max:       8,
			},
		},
		{
			name: "valid fixed allocation",
			envelope: GPUEnvelope{
				Min:       4,
				Preferred: 4,
				Max:       4,
			},
		},
		{
			name: "valid zero gpu",
			envelope: GPUEnvelope{
				Min:       0,
				Preferred: 0,
				Max:       0,
			},
		},
		{
			name: "min greater than preferred",
			envelope: GPUEnvelope{
				Min:       4,
				Preferred: 2,
				Max:       8,
			},
			wantErr: true,
		},
		{
			name: "preferred greater than max",
			envelope: GPUEnvelope{
				Min:       1,
				Preferred: 8,
				Max:       4,
			},
			wantErr: true,
		},
		{
			name: "negative minimum",
			envelope: GPUEnvelope{
				Min:       -1,
				Preferred: 2,
				Max:       4,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.envelope.Validate()

			if tt.wantErr && err == nil {
				t.Fatal("Validate() expected error")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf(
					"Validate() unexpected error: %v",
					err,
				)
			}
		})
	}
}

func TestGPUEnvelopeContains(t *testing.T) {
	envelope := GPUEnvelope{
		Min:       1,
		Preferred: 4,
		Max:       8,
	}

	tests := []struct {
		gpus int32
		want bool
	}{
		{0, false},
		{1, true},
		{2, true},
		{4, true},
		{8, true},
		{9, false},
	}

	for _, tt := range tests {
		if got := envelope.Contains(tt.gpus); got != tt.want {
			t.Fatalf(
				"Contains(%d) = %v, want %v",
				tt.gpus,
				got,
				tt.want,
			)
		}
	}
}

func TestGPUEnvelopeFixed(t *testing.T) {
	fixed := GPUEnvelope{
		Min:       4,
		Preferred: 4,
		Max:       4,
	}

	if !fixed.IsFixed() {
		t.Fatal("expected envelope to be fixed")
	}

	if fixed.IsMalleable() {
		t.Fatal("fixed envelope should not be malleable")
	}
}

func TestGPUEnvelopeMalleable(t *testing.T) {
	envelope := GPUEnvelope{
		Min:       1,
		Preferred: 4,
		Max:       8,
	}

	if !envelope.IsMalleable() {
		t.Fatal("expected envelope to be malleable")
	}
}

func TestGPUEnvelopeNegativeMinError(t *testing.T) {
	envelope := GPUEnvelope{
		Min:       -1,
		Preferred: 1,
		Max:       2,
	}

	err := envelope.Validate()

	if !errors.Is(err, ErrNegativeGPUMin) {
		t.Fatalf(
			"Validate() error = %v, want ErrNegativeGPUMin",
			err,
		)
	}
}
