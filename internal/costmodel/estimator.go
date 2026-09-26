package costmodel

type Request struct {
	StateMB int

	SourceGPU int
	TargetGPU int
}

type Estimate struct {
	ExpectedUS int64
	SafeUS     int64

	Model string
}

type Estimator interface {
	Estimate(
		request Request,
	) Estimate
}
