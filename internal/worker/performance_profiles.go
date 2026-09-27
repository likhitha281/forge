package worker

import (
	"sync"

	"github.com/likhitha281/forge/internal/performance"
)

type PerformanceProfiles struct {
	mu sync.Mutex

	models map[string]*performance.Model

	alpha float64
}

func NewPerformanceProfiles(
	alpha float64,
) *PerformanceProfiles {
	return &PerformanceProfiles{
		models: make(
			map[string]*performance.Model,
		),

		alpha: alpha,
	}
}

func (p *PerformanceProfiles) Model(
	key string,
) *performance.Model {
	p.mu.Lock()
	defer p.mu.Unlock()

	model, exists :=
		p.models[key]

	if exists {
		return model
	}

	model =
		performance.New(
			p.alpha,
		)

	p.models[key] =
		model

	return model
}
