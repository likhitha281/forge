package costmodel

func LeaveOneStateSizeOut(
	observations []Observation,
	factory EstimatorFactory,
) Evaluation {
	if len(observations) < 2 {
		return Evaluation{}
	}

	stateSizes :=
		make(
			map[int]struct{},
		)

	for _, observation := range observations {
		stateSizes[observation.StateMB] =
			struct{}{}
	}

	var predictions []Prediction

	for stateMB := range stateSizes {
		var (
			training []Observation
			test     []Observation
		)

		for _, observation := range observations {

			if observation.StateMB ==
				stateMB {

				test = append(
					test,
					observation,
				)

				continue
			}

			training = append(
				training,
				observation,
			)
		}

		if len(training) == 0 {
			continue
		}

		estimator :=
			factory(training)

		for _, heldOut := range test {

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
	}

	return EvaluatePredictions(
		predictions,
	)
}
