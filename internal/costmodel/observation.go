package costmodel

type Observation struct {
	StateMB int

	SourceGPU int
	TargetGPU int

	PrepareUS     int64
	CheckpointUS  int64
	ReconfigureUS int64
	RestoreUS     int64
	ResumeUS      int64
	TotalUS       int64

	CheckpointBytes int64
	RestoreBytes    int64
	BytesMoved      int64
}

func (o Observation) GPUDelta() int {
	return o.TargetGPU - o.SourceGPU
}

func (o Observation) Direction() string {
	if o.TargetGPU > o.SourceGPU {
		return "grow"
	}

	if o.TargetGPU < o.SourceGPU {
		return "shrink"
	}

	return "same"
}
