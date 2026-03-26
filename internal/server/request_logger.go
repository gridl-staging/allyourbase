package server

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RequestLogEntry represents a single HTTP request log record.
type RequestLogEntry struct {
	Method       string
	Path         string
	StatusCode   int
	DurationMS   int64
	UserID       string
	APIKeyID     string
	RequestSize  int64
	ResponseSize int64
	IPAddress    string
	RequestID    string
	TenantID     string
}

// RequestLogConfig holds configuration for the async request logger.
type RequestLogConfig struct {
	Enabled           bool
	BatchSize         int
	FlushIntervalSecs int
	QueueSize         int
	RetentionDays     int
}

// flushFn is a function that persists a batch of log entries.
type flushFn func(ctx context.Context, entries []RequestLogEntry) error

// RequestLogger asynchronously batches and persists request log entries.
// Log() is non-blocking — entries are dropped (with a warning) when the
// internal channel is full, ensuring the request path is never impacted.
type RequestLogger struct {
	cfg            RequestLogConfig
	logger         *slog.Logger
	ch             chan RequestLogEntry
	flush          flushFn
	flushInterval  time.Duration // overrideable in tests
	isShuttingDown atomic.Bool
	shutdownOnce   sync.Once
	dropCount      atomic.Int64
	wg             sync.WaitGroup
	cancel         context.CancelFunc
	finalBatch     []RequestLogEntry
}

// newRequestLoggerWithFlush constructs a RequestLogger with an injectable flush function.
// Used for unit testing without a real DB connection.
func newRequestLoggerWithFlush(cfg RequestLogConfig, logger *slog.Logger, fn flushFn) *RequestLogger {
	interval := time.Duration(cfg.FlushIntervalSecs) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &RequestLogger{
		cfg:           cfg,
		logger:        logger,
		ch:            make(chan RequestLogEntry, cfg.QueueSize),
		flush:         fn,
		flushInterval: interval,
	}
}

// NewRequestLogger constructs a production RequestLogger that batch-inserts
// entries into the _ayb_request_logs table via UNNEST arrays.
func NewRequestLogger(cfg RequestLogConfig, logger *slog.Logger, pool *pgxpool.Pool) *RequestLogger {
	fn := func(ctx context.Context, entries []RequestLogEntry) error {
		if len(entries) == 0 {
			return nil
		}

		methods := make([]string, len(entries))
		paths := make([]string, len(entries))
		statusCodes := make([]int32, len(entries))
		durationMSs := make([]int64, len(entries))
		userIDs := make([]*string, len(entries))
		apiKeyIDs := make([]*string, len(entries))
		requestSizes := make([]int64, len(entries))
		responseSizes := make([]int64, len(entries))
		ipAddresses := make([]*string, len(entries))
		requestIDs := make([]*string, len(entries))

		for i, e := range entries {
			methods[i] = e.Method
			paths[i] = e.Path
			statusCodes[i] = int32(e.StatusCode)
			durationMSs[i] = e.DurationMS
			if e.UserID != "" {
				s := e.UserID
				userIDs[i] = &s
			}
			if e.APIKeyID != "" {
				s := e.APIKeyID
				apiKeyIDs[i] = &s
			}
			requestSizes[i] = e.RequestSize
			responseSizes[i] = e.ResponseSize
			if e.IPAddress != "" {
				s := e.IPAddress
				ipAddresses[i] = &s
			}
			if e.RequestID != "" {
				s := e.RequestID
				requestIDs[i] = &s
			}
		}

		_, err := pool.Exec(ctx, `
			INSERT INTO _ayb_request_logs
				(method, path, status_code, duration_ms, user_id, api_key_id,
				 request_size, response_size, ip_address, request_id)
			SELECT
				unnest($1::text[]),
				unnest($2::text[]),
				unnest($3::int[]),
				unnest($4::bigint[]),
				unnest($5::uuid[]),
				unnest($6::uuid[]),
				unnest($7::bigint[]),
				unnest($8::bigint[]),
				unnest($9::inet[]),
				unnest($10::text[])
		`,
			methods, paths, statusCodes, durationMSs,
			userIDs, apiKeyIDs, requestSizes, responseSizes,
			ipAddresses, requestIDs,
		)
		return err
	}
	return newRequestLoggerWithFlush(cfg, logger, fn)
}

// Start launches the background worker goroutine that drains the channel.
// The worker stops when the context passed to Shutdown is cancelled.
func (rl *RequestLogger) Start(ctx context.Context) {
	if !rl.cfg.Enabled {
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	rl.cancel = cancel
	rl.wg.Add(1)
	go rl.worker(workerCtx)
}

// Log enqueues a log entry. It never blocks — if the channel is full,
// the entry is dropped and the drop counter is incremented.
func (rl *RequestLogger) Log(entry RequestLogEntry) {
	if !rl.cfg.Enabled {
		return
	}
	if rl.isShuttingDown.Load() {
		n := rl.dropCount.Add(1)
		if n == 1 || n%100 == 0 {
			rl.logger.Warn("request logger dropping entry (shutting down)",
				"total_dropped", n)
		}
		return
	}
	select {
	case rl.ch <- entry:
	default:
		n := rl.dropCount.Add(1)
		if n == 1 || n%100 == 0 {
			rl.logger.Warn("request log channel full — dropping entry",
				"total_dropped", n)
		}
	}
}

// Shutdown signals the worker to stop and waits for it to flush all
// remaining entries before returning.
func (rl *RequestLogger) Shutdown(ctx context.Context) error {
	if !rl.cfg.Enabled {
		return nil
	}
	rl.shutdownOnce.Do(func() {
		rl.isShuttingDown.Store(true)
		if rl.cancel != nil {
			rl.cancel()
		}
	})
	rl.wg.Wait()

	dropped := rl.dropCount.Load()
	if dropped > 0 {
		rl.logger.Warn("request logger shutdown observed dropped entries", "total_dropped", dropped)
	}

	// Drain any remaining entries that arrived before shutdown.
	rl.drainRemaining(ctx)
	return nil
}

// DropCount returns the total number of entries dropped due to channel saturation.
func (rl *RequestLogger) DropCount() int64 {
	return rl.dropCount.Load()
}

// worker drains rl.ch, accumulating entries into a batch that flushes
// either when it reaches BatchSize or when the flush ticker fires.
func (rl *RequestLogger) worker(ctx context.Context) {
	defer rl.wg.Done()

	ticker := time.NewTicker(rl.flushInterval)
	defer ticker.Stop()

	batch := make([]RequestLogEntry, 0, rl.cfg.BatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		flushCtx := ctx
		if flushCtx == nil || flushCtx.Err() != nil {
			flushCtx = context.Background()
		}
		if err := rl.flush(flushCtx, batch); err != nil {
			rl.logger.Error("request log flush failed", "error", err, "dropped", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-rl.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= rl.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			// Drain whatever is buffered in the channel before exiting.
			for {
				select {
				case entry, ok := <-rl.ch:
					if !ok {
						flush()
						return
					}
					batch = append(batch, entry)
				default:
					if len(batch) > 0 {
						rl.finalBatch = append([]RequestLogEntry{}, batch...)
					}
					return
				}
			}
		}
	}
}

// drainRemaining flushes any entries still in the channel after the worker exits.
// Called by Shutdown with a fresh context so the final flush isn't cancelled.
func (rl *RequestLogger) drainRemaining(ctx context.Context) {
	if len(rl.finalBatch) > 0 {
		if err := rl.flush(ctx, rl.finalBatch); err != nil {
			rl.logger.Error("request log final flush failed", "error", err, "dropped", len(rl.finalBatch))
		}
		rl.finalBatch = nil
	}
	batch := make([]RequestLogEntry, 0, rl.cfg.BatchSize)
	for {
		select {
		case entry, ok := <-rl.ch:
			if !ok {
				return
			}
			batch = append(batch, entry)
		default:
			if len(batch) > 0 {
				if err := rl.flush(ctx, batch); err != nil {
					rl.logger.Error("request log final flush failed", "error", err, "dropped", len(batch))
				}
			}
			return
		}
	}
}
