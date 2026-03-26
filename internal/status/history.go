package status

import "sync"

// StatusHistory stores bounded status snapshots in insertion order.
type StatusHistory struct {
	mu       sync.RWMutex
	buf      []StatusSnapshot
	start    int
	count    int
	capacity int
}

// NewStatusHistory allocates a bounded status history ring buffer.
func NewStatusHistory(capacity int) *StatusHistory {
	if capacity <= 0 {
		capacity = 1
	}
	return &StatusHistory{
		buf:      make([]StatusSnapshot, capacity),
		capacity: capacity,
	}
}

// Push appends a snapshot, evicting the oldest snapshot when at capacity.
func (h *StatusHistory) Push(snapshot StatusSnapshot) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	snapshot = cloneSnapshot(snapshot)
	if h.count < h.capacity {
		idx := (h.start + h.count) % h.capacity
		h.buf[idx] = snapshot
		h.count++
		return
	}

	// Overwrite oldest and advance start.
	h.buf[h.start] = snapshot
	h.start = (h.start + 1) % h.capacity
}

// Latest returns the most recently pushed snapshot or nil when empty.
func (h *StatusHistory) Latest() *StatusSnapshot {
	if h == nil {
		return nil
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.count == 0 {
		return nil
	}
	lastIdx := (h.start + h.count - 1) % h.capacity
	s := cloneSnapshot(h.buf[lastIdx])
	return &s
}

// Recent returns up to n snapshots in chronological order (oldest to newest).
func (h *StatusHistory) Recent(n int) []StatusSnapshot {
	if h == nil || n <= 0 {
		return []StatusSnapshot{}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.count == 0 {
		return []StatusSnapshot{}
	}

	if n > h.count {
		n = h.count
	}
	startOffset := h.count - n
	out := make([]StatusSnapshot, 0, n)
	for i := 0; i < n; i++ {
		idx := (h.start + startOffset + i) % h.capacity
		out = append(out, cloneSnapshot(h.buf[idx]))
	}
	return out
}

func cloneSnapshot(s StatusSnapshot) StatusSnapshot {
	if s.Services != nil {
		services := make([]ProbeResult, len(s.Services))
		copy(services, s.Services)
		s.Services = services
	}
	return s
}
