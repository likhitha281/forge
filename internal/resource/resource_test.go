package resource

import (
	"errors"
	"testing"
)

func TestVectorValidate(t *testing.T) {
	tests := []struct {
		name    string
		vector  Vector
		wantErr error
	}{
		{
			name: "valid",
			vector: Vector{
				CPUCores: 4,
				MemoryMB: 8192,
				GPUs:     2,
			},
		},
		{
			name: "fractional cpu",
			vector: Vector{
				CPUCores: 0.5,
				MemoryMB: 512,
			},
		},
		{
			name: "negative cpu",
			vector: Vector{
				CPUCores: -1,
			},
			wantErr: ErrNegativeCPU,
		},
		{
			name: "negative memory",
			vector: Vector{
				MemoryMB: -1,
			},
			wantErr: ErrNegativeMemory,
		},
		{
			name: "negative gpu",
			vector: Vector{
				GPUs: -1,
			},
			wantErr: ErrNegativeGPU,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.vector.Validate()

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() returned unexpected error: %v", err)
				}
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestVectorFits(t *testing.T) {
	available := Vector{
		CPUCores: 8,
		MemoryMB: 32768,
		GPUs:     4,
	}

	tests := []struct {
		name    string
		request Vector
		want    bool
	}{
		{
			name: "fits",
			request: Vector{
				CPUCores: 4,
				MemoryMB: 8192,
				GPUs:     2,
			},
			want: true,
		},
		{
			name: "exact fit",
			request: Vector{
				CPUCores: 8,
				MemoryMB: 32768,
				GPUs:     4,
			},
			want: true,
		},
		{
			name: "too much cpu",
			request: Vector{
				CPUCores: 9,
			},
			want: false,
		},
		{
			name: "too much memory",
			request: Vector{
				MemoryMB: 65536,
			},
			want: false,
		},
		{
			name: "too many gpus",
			request: Vector{
				GPUs: 5,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := available.Fits(tt.request); got != tt.want {
				t.Fatalf(
					"Fits(%v) = %v, want %v",
					tt.request,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestVectorAdd(t *testing.T) {
	a := Vector{
		CPUCores: 2,
		MemoryMB: 4096,
		GPUs:     1,
	}

	b := Vector{
		CPUCores: 1.5,
		MemoryMB: 2048,
		GPUs:     2,
	}

	want := Vector{
		CPUCores: 3.5,
		MemoryMB: 6144,
		GPUs:     3,
	}

	got := a.Add(b)

	if !got.Equal(want) {
		t.Fatalf("Add() = %v, want %v", got, want)
	}
}

func TestVectorSubtract(t *testing.T) {
	capacity := Vector{
		CPUCores: 8,
		MemoryMB: 32768,
		GPUs:     4,
	}

	request := Vector{
		CPUCores: 3,
		MemoryMB: 8192,
		GPUs:     1,
	}

	want := Vector{
		CPUCores: 5,
		MemoryMB: 24576,
		GPUs:     3,
	}

	got, err := capacity.Subtract(request)
	if err != nil {
		t.Fatalf("Subtract() returned error: %v", err)
	}

	if !got.Equal(want) {
		t.Fatalf("Subtract() = %v, want %v", got, want)
	}
}

func TestVectorSubtractRejectsUnderflow(t *testing.T) {
	available := Vector{
		CPUCores: 2,
		MemoryMB: 4096,
		GPUs:     1,
	}

	request := Vector{
		CPUCores: 4,
	}

	_, err := available.Subtract(request)

	if !errors.Is(err, ErrInsufficient) {
		t.Fatalf("Subtract() error = %v, want ErrInsufficient", err)
	}
}

func TestZeroVector(t *testing.T) {
	if !(Vector{}).IsZero() {
		t.Fatal("zero Vector should report IsZero() = true")
	}
}
