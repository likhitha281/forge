package worker

import "sync"

// RuntimeManager tracks processes currently owned by this worker.
//
// Transition execution uses this registry to locate the local workload
// associated with a persisted transition.
type RuntimeManager struct {
	mu sync.RWMutex

	runtimes map[string]*Runtime
}

func NewRuntimeManager() *RuntimeManager {
	return &RuntimeManager{
		runtimes: make(
			map[string]*Runtime,
		),
	}
}

func (m *RuntimeManager) Add(
	runtime *Runtime,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists :=
		m.runtimes[runtime.JobID]; exists {
		return ErrRuntimeAlreadyExists
	}

	m.runtimes[runtime.JobID] = runtime

	return nil
}

func (m *RuntimeManager) Get(
	jobID string,
) (*Runtime, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	runtime, ok := m.runtimes[jobID]

	return runtime, ok
}

func (m *RuntimeManager) Remove(
	jobID string,
) (*Runtime, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	runtime, ok := m.runtimes[jobID]

	if !ok {
		return nil, false
	}

	delete(
		m.runtimes,
		jobID,
	)

	return runtime, true
}

func (m *RuntimeManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.runtimes)
}
