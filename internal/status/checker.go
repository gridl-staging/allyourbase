package status

import (
	"context"
	"sync"
	"time"
)

const defaultCheckInterval = 30 * time.Second

// Checker executes service probes and stores snapshots in history.
type Checker struct {
	probes   []Probe
	history  *StatusHistory
	interval time.Duration

	mu     sync.Mutex
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewChecker constructs a checker with probes, history, and run interval.
func NewChecker(probes []Probe, history *StatusHistory, interval time.Duration) *Checker {
	if interval <= 0 {
		interval = defaultCheckInterval
	}
	cloned := make([]Probe, 0, len(probes))
	for _, p := range probes {
		if p != nil {
			cloned = append(cloned, p)
		}
	}
	return &Checker{
		probes:   cloned,
		history:  history,
		interval: interval,
	}
}

// RunOnce executes all probes once, derives rollup status, stores history, and returns snapshot.
func (c *Checker) RunOnce(ctx context.Context) StatusSnapshot {
	now := time.Now().UTC()
	results := make([]ProbeResult, 0, len(c.probes))
	for _, probe := range c.probes {
		results = append(results, probe.Check(ctx))
	}

	snapshot := StatusSnapshot{
		Status:    DeriveStatus(results),
		Services:  results,
		CheckedAt: now,
	}
	if c.history != nil {
		c.history.Push(snapshot)
	}
	return snapshot
}

// Start begins periodic probe execution on a ticker until context cancel or Stop().
func (c *Checker) Start(ctx context.Context) {
	c.mu.Lock()
	if c.stopCh != nil {
		c.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	c.stopCh = stopCh
	c.doneCh = doneCh
	c.mu.Unlock()

	go func() {
		defer close(doneCh)
		c.RunOnce(ctx)

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				c.RunOnce(ctx)
			}
		}
	}()
}

// Stop requests checker shutdown and waits for its goroutine to exit.
func (c *Checker) Stop() {
	c.mu.Lock()
	stopCh := c.stopCh
	doneCh := c.doneCh
	if stopCh == nil {
		c.mu.Unlock()
		return
	}
	c.stopCh = nil
	c.doneCh = nil
	close(stopCh)
	c.mu.Unlock()

	<-doneCh
}
