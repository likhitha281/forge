package transition

import "time"

// Metrics contains the measured costs of executing a transition.
type Metrics struct {
	PrepareDuration     time.Duration
	CheckpointDuration  time.Duration
	ReconfigureDuration time.Duration
	RestoreDuration     time.Duration
	ResumeDuration      time.Duration

	CheckpointBytes int64
	RestoreBytes    int64
}

func MetricsFromExecutionResult(
	result ExecutionResult,
) Metrics {
	return Metrics{
		PrepareDuration: result.Prepare.Duration,

		CheckpointDuration: result.Checkpoint.Duration,

		ReconfigureDuration: result.Reconfigure.Duration,

		RestoreDuration: result.Restore.Duration,

		ResumeDuration: result.Resume.Duration,

		CheckpointBytes: result.Checkpoint.BytesMoved,

		RestoreBytes: result.Restore.BytesMoved,
	}
}

func (m Metrics) TotalDuration() time.Duration {
	return m.PrepareDuration +
		m.CheckpointDuration +
		m.ReconfigureDuration +
		m.RestoreDuration +
		m.ResumeDuration
}

func (m Metrics) TotalBytesMoved() int64 {
	return m.CheckpointBytes +
		m.RestoreBytes
}
