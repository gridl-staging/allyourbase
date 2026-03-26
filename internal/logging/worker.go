// Package logging DrainWorker batches log entries and exports them through a LogDrain with configurable retry and exponential backoff logic.
package logging

import (
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	sharedbackoff "github.com/allyourbase/ayb/internal/backoff"
)

// DrainWorkerConfig configures a batched export worker.
type DrainWorkerConfig struct {
	BatchSize      int
	FlushInterval  time.Duration
	QueueSize      int
	MaxRetries     int           // default 3
	BaseRetryDelay time.Duration // default 1s; grows 2x with jitter, capped at MaxRetryDelay
	MaxRetryDelay  time.Duration // default 30s
}

// defaults assigns default values to zero-valued fields: BatchSize=100, FlushInterval=5s, QueueSize=10000, MaxRetries=3, BaseRetryDelay=1s, MaxRetryDelay=30s.
func (c *DrainWorkerConfig) defaults() {
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = 5 * time.Second
	}
	if c.QueueSize <= 0 {
		c.QueueSize = 10000
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
	if c.BaseRetryDelay <= 0 {
		c.BaseRetryDelay = time.Second
	}
	if c.MaxRetryDelay <= 0 {
		c.MaxRetryDelay = 30 * time.Second
	}
}

// DrainWorker runs a goroutine that batches log entries and sends them
// through a LogDrain with retry and backoff.
type DrainWorker struct {
	drain     LogDrain
	cfg       DrainWorkerConfig
	ch        chan LogEntry
	stopCh    chan struct{}
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once
	logger    *slog.Logger
}

// NewDrainWorker creates a drain worker. Call Start to launch the goroutine.
func NewDrainWorker(drain LogDrain, cfg DrainWorkerConfig) *DrainWorker {
	cfg.defaults()
	return &DrainWorker{
		drain:  drain,
		cfg:    cfg,
		ch:     make(chan LogEntry, cfg.QueueSize),
		stopCh: make(chan struct{}),
		logger: slog.Default(),
	}
}

// Start launches the background worker goroutine. Safe to call multiple times;
// only the first call starts the goroutine.
func (w *DrainWorker) Start() {
	w.startOnce.Do(func() {
		w.wg.Add(1)
		go w.run()
	})
}

// Stop signals the worker to drain remaining entries and stop. Safe to call
// multiple times; only the first call closes the stop channel.
func (w *DrainWorker) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
	w.wg.Wait()
}

// Enqueue adds an entry to the worker's bounded channel.
// Non-blocking: drops the entry if the channel is full.
func (w *DrainWorker) Enqueue(entry LogEntry) {
	select {
	case w.ch <- entry:
	default:
		// Channel full — drop silently.
	}
}

// run executes the background worker's main loop, batching log entries and flushing them when the batch reaches its size limit or the flush interval elapses; on stop signal, it drains remaining entries before exiting.
func (w *DrainWorker) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]LogEntry, 0, w.cfg.BatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		w.sendWithRetry(batch)
		batch = batch[:0]
	}

	for {
		select {
		case entry := <-w.ch:
			batch = append(batch, entry)
			if len(batch) >= w.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-w.stopCh:
			// Drain remaining channel entries.
			for {
				select {
				case entry := <-w.ch:
					batch = append(batch, entry)
				default:
					flush()
					return
				}
			}
		}
	}
}

// sendWithRetry sends a batch of log entries through the drain with exponential backoff retry logic, attempting up to MaxRetries times; if all retries fail, entries are dropped and ReportDropped is called on the drain if it implements DropReporter.
func (w *DrainWorker) sendWithRetry(batch []LogEntry) {
	cp := make([]LogEntry, len(batch))
	copy(cp, batch)

	for attempt := range w.cfg.MaxRetries {
		err := w.drain.Send(cp)
		if err == nil {
			return
		}
		if attempt < w.cfg.MaxRetries-1 {
			delay := sharedbackoff.Exponential(attempt+1, sharedbackoff.Config{
				Base: w.cfg.BaseRetryDelay,
				Cap:  w.cfg.MaxRetryDelay,
				Jitter: func(delay time.Duration) time.Duration {
					jitter := float64(delay) * 0.25 * (2*rand.Float64() - 1)
					return time.Duration(jitter)
				},
			})
			time.Sleep(delay)
		}
	}
	// All retries exhausted — entries are dropped.
	if dr, ok := w.drain.(DropReporter); ok {
		dr.ReportDropped(int64(len(cp)))
	}
}
