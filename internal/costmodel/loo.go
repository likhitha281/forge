package costmodel

type EstimatorFactory func(
	observations []Observation,
) Estimator

func LeaveOneOut(
	observations []Observation,
	factory EstimatorFactory,
) Evaluation {
	if len(observations) < 2 {
		return Evaluation{}
	}

	predictions :=
		make(
			[]Prediction,
			0,
			len(observations),
		)

	for index, heldOut := range observations {

		training :=
			make(
				[]Observation,
				0,
				len(observations)-1,
			)

		training = append(
			training,
			observations[:index]...,
		)

		training = append(
			training,
			observations[index+1:]...,
		)

		estimator :=
			factory(training)

		estimate :=
			estimator.Estimate(
				Request{
					StateMB: heldOut.StateMB,

					SourceGPU: heldOut.SourceGPU,

					TargetGPU: heldOut.TargetGPU,
				},
			)

		predictions = append(
			predictions,
			Prediction{
				ActualUS: heldOut.TotalUS,

				ExpectedUS: estimate.ExpectedUS,

				SafeUS: estimate.SafeUS,
			},
		)
	}

	return EvaluatePredictions(
		predictions,
	)
}
