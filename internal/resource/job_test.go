package resource

import "testing"

func testMalleableJobResources() JobResources {
	return JobResources{
		CPUCores: 4,
		MemoryMB: 8192,
		GPU: GPUEnvelope{
			Min:       1,
			Preferred: 4,
			Max:       8,
		},
	}
}

func TestJobResourcesValidate(t *testing.T) {
	r := testMalleableJobResources()

	if err := r.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestJobResourcesMinimum(t *testing.T) {
	r := testMalleableJobResources()

	want := Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     1,
	}

	if got := r.Minimum(); !got.Equal(want) {
		t.Fatalf("Minimum() = %v, want %v", got, want)
	}
}

func TestJobResourcesPreferred(t *testing.T) {
	r := testMalleableJobResources()

	want := Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     4,
	}

	if got := r.Preferred(); !got.Equal(want) {
		t.Fatalf("Preferred() = %v, want %v", got, want)
	}
}

func TestJobResourcesMaximum(t *testing.T) {
	r := testMalleableJobResources()

	want := Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     8,
	}

	if got := r.Maximum(); !got.Equal(want) {
		t.Fatalf("Maximum() = %v, want %v", got, want)
	}
}

func TestJobResourcesAccepts(t *testing.T) {
	r := testMalleableJobResources()

	tests := []struct {
		name       string
		allocation Vector
		want       bool
	}{
		{
			name: "minimum",
			allocation: Vector{
				CPUCores: 4,
				MemoryMB: 8192,
				GPUs:     1,
			},
			want: true,
		},
		{
			name: "preferred",
			allocation: Vector{
				CPUCores: 4,
				MemoryMB: 8192,
				GPUs:     4,
			},
			want: true,
		},
		{
			name: "intermediate",
			allocation: Vector{
				CPUCores: 4,
				MemoryMB: 8192,
				GPUs:     3,
			},
			want: true,
		},
		{
			name: "below minimum",
			allocation: Vector{
				CPUCores: 4,
				MemoryMB: 8192,
				GPUs:     0,
			},
			want: false,
		},
		{
			name: "above maximum",
			allocation: Vector{
				CPUCores: 4,
				MemoryMB: 8192,
				GPUs:     9,
			},
			want: false,
		},
		{
			name: "wrong CPU",
			allocation: Vector{
				CPUCores: 2,
				MemoryMB: 8192,
				GPUs:     4,
			},
			want: false,
		},
		{
			name: "wrong memory",
			allocation: Vector{
				CPUCores: 4,
				MemoryMB: 4096,
				GPUs:     4,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Accepts(tt.allocation); got != tt.want {
				t.Fatalf(
					"Accepts(%v) = %v, want %v",
					tt.allocation,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestInitialAllocationPreferred(t *testing.T) {
	r := testMalleableJobResources()

	available := Vector{
		CPUCores: 8,
		MemoryMB: 16384,
		GPUs:     8,
	}

	got, ok := r.InitialAllocation(available)

	if !ok {
		t.Fatal("expected allocation to succeed")
	}

	want := r.Preferred()

	if !got.Equal(want) {
		t.Fatalf(
			"InitialAllocation() = %v, want %v",
			got,
			want,
		)
	}
}

func TestInitialAllocationDegradesGPUCount(t *testing.T) {
	r := testMalleableJobResources()

	available := Vector{
		CPUCores: 8,
		MemoryMB: 16384,
		GPUs:     2,
	}

	got, ok := r.InitialAllocation(available)

	if !ok {
		t.Fatal("expected degraded allocation to succeed")
	}

	want := Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     2,
	}

	if !got.Equal(want) {
		t.Fatalf(
			"InitialAllocation() = %v, want %v",
			got,
			want,
		)
	}
}

func TestInitialAllocationAtMinimum(t *testing.T) {
	r := testMalleableJobResources()

	available := Vector{
		CPUCores: 4,
		MemoryMB: 8192,
		GPUs:     1,
	}

	got, ok := r.InitialAllocation(available)

	if !ok {
		t.Fatal("expected minimum allocation to succeed")
	}

	if !got.Equal(r.Minimum()) {
		t.Fatalf(
			"InitialAllocation() = %v, want minimum %v",
			got,
			r.Minimum(),
		)
	}
}

func TestInitialAllocationRejectsBelowMinimumGPU(t *testing.T) {
	r := testMalleableJobResources()

	available := Vector{
		CPUCores: 8,
		MemoryMB: 16384,
		GPUs:     0,
	}

	_, ok := r.InitialAllocation(available)

	if ok {
		t.Fatal("allocation should fail below minimum GPU count")
	}
}

func TestInitialAllocationRejectsInsufficientCPU(t *testing.T) {
	r := testMalleableJobResources()

	available := Vector{
		CPUCores: 2,
		MemoryMB: 16384,
		GPUs:     8,
	}

	_, ok := r.InitialAllocation(available)

	if ok {
		t.Fatal("allocation should fail with insufficient CPU")
	}
}

func TestInitialAllocationRejectsInsufficientMemory(t *testing.T) {
	r := testMalleableJobResources()

	available := Vector{
		CPUCores: 8,
		MemoryMB: 4096,
		GPUs:     8,
	}

	_, ok := r.InitialAllocation(available)

	if ok {
		t.Fatal("allocation should fail with insufficient memory")
	}
}
