package logging

import (
	"sync"
)

// DrainInfo holds metadata about a registered drain for listing.
type DrainInfo struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Stats DrainStats `json:"stats"`
}

// DrainManager is a central fan-out that copies log entries to all active drain workers.
type DrainManager struct {
	mu      sync.RWMutex
	workers map[string]*DrainWorker
	drains  map[string]LogDrain
	started bool
}

// NewDrainManager creates a new manager with no drains.
func NewDrainManager() *DrainManager {
	return &DrainManager{
		workers: make(map[string]*DrainWorker),
		drains:  make(map[string]LogDrain),
	}
}

// AddDrain registers a drain with the given ID and worker config.
// If the manager is already started, the new drain's worker starts immediately.
func (m *DrainManager) AddDrain(id string, drain LogDrain, cfg DrainWorkerConfig) {
	w := NewDrainWorker(drain, cfg)
	m.mu.Lock()
	prev, hadPrev := m.workers[id]
	m.workers[id] = w
	m.drains[id] = drain
	shouldStart := m.started
	m.mu.Unlock()

	if hadPrev {
		prev.Stop()
	}
	if shouldStart {
		w.Start()
	}
}

// Start launches all registered drain workers.
func (m *DrainManager) Start() {
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.workers {
		w.Start()
	}
}

// Stop gracefully stops all drain workers, flushing remaining entries.
func (m *DrainManager) Stop() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.workers {
		w.Stop()
	}
}

// Enqueue fans out a log entry to all active drain workers.
// Non-blocking: if any worker's channel is full, that entry is dropped for that drain.
func (m *DrainManager) Enqueue(entry LogEntry) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.workers {
		w.Enqueue(entry)
	}
}

// RemoveDrain stops and removes the drain with the given ID.
func (m *DrainManager) RemoveDrain(id string) {
	m.mu.Lock()
	w, ok := m.workers[id]
	if ok {
		delete(m.workers, id)
		delete(m.drains, id)
	}
	m.mu.Unlock()
	if ok {
		w.Stop()
	}
}

// ListDrains returns info about all registered drains.
func (m *DrainManager) ListDrains() []DrainInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]DrainInfo, 0, len(m.drains))
	for id, d := range m.drains {
		result = append(result, DrainInfo{
			ID:    id,
			Name:  d.Name(),
			Stats: d.Stats(),
		})
	}
	return result
}
