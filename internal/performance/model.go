package performance

import (
	"errors"
	"math"
	"sync"
	"time"
)

var (
	ErrInvalidSnapshot = errors.New(
		"invalid performance snapshot",
	)

	ErrInsufficientData = errors.New(
		"insufficient performance data",
	)
)

type Snapshot struct {
	Completed uint64
	Total     uint64

	Allocation int
	Rate       float64

	Timestamp time.Time
}

type Estimate struct {
	Completed uint64
	Total     uint64

	CurrentAllocation int
	CurrentRate       float64

	RemainingWork    uint64
	RemainingRuntime time.Duration
}

type Model struct {
	mu sync.RWMutex

	alpha float64

	rates map[int]float64

	latest    Snapshot
	hasLatest bool
}

func New(
	alpha float64,
) *Model {
	if alpha <= 0 || alpha > 1 {
		alpha = 0.3
	}

	return &Model{
		alpha: alpha,
		rates: make(map[int]float64),
	}
}

func (m *Model) Observe(
	snapshot Snapshot,
) error {
	if snapshot.Total == 0 ||
		snapshot.Completed > snapshot.Total ||
		snapshot.Allocation <= 0 ||
		snapshot.Rate < 0 {

		return ErrInvalidSnapshot
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// A zero-rate sample is emitted immediately after restore.
	// Preserve the new progress position but do not feed the reset
	// marker into the throughput estimator.
	if snapshot.Rate > 0 {
		oldRate, exists :=
			m.rates[snapshot.Allocation]

		if !exists {
			m.rates[snapshot.Allocation] =
				snapshot.Rate
		} else {
			m.rates[snapshot.Allocation] =
				m.alpha*snapshot.Rate +
					(1-m.alpha)*oldRate
		}
	}

	m.latest = snapshot
	m.hasLatest = true

	return nil
}

func (m *Model) Rate(
	allocation int,
) (float64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rate, ok :=
		m.rates[allocation]

	return rate, ok
}

func (m *Model) Speedup(
	source int,
	target int,
) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sourceRate, sourceOK :=
		m.rates[source]

	targetRate, targetOK :=
		m.rates[target]

	if !sourceOK ||
		!targetOK ||
		sourceRate <= 0 ||
		targetRate <= 0 {

		return 0, ErrInsufficientData
	}

	return targetRate / sourceRate, nil
}

func (m *Model) CurrentEstimate() (
	Estimate,
	error,
) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.hasLatest {
		return Estimate{},
			ErrInsufficientData
	}

	currentRate, ok :=
		m.rates[m.latest.Allocation]

	if !ok || currentRate <= 0 {
		return Estimate{},
			ErrInsufficientData
	}

	remaining :=
		m.latest.Total -
			m.latest.Completed

	seconds :=
		float64(remaining) /
			currentRate

	if math.IsNaN(seconds) ||
		math.IsInf(seconds, 0) ||
		seconds < 0 {

		return Estimate{},
			ErrInsufficientData
	}

	return Estimate{
		Completed: m.latest.Completed,

		Total: m.latest.Total,

		CurrentAllocation: m.latest.Allocation,

		CurrentRate: currentRate,

		RemainingWork: remaining,

		RemainingRuntime: time.Duration(
			seconds *
				float64(time.Second),
		),
	}, nil
}
