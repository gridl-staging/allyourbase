package auth

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultSessionActivityDebounce = 5 * time.Minute

// SessionActivityTracker debounces background session activity writes.
type SessionActivityTracker struct {
	mu        sync.Mutex
	lastWrite map[string]time.Time

	pool     *pgxpool.Pool
	logger   *slog.Logger
	debounce time.Duration

	nowFn    func() time.Time
	updateFn func(ctx context.Context, sessionID string) error
}

// NewSessionActivityTracker creates a session activity tracker.
func NewSessionActivityTracker(pool *pgxpool.Pool, debounce time.Duration, logger *slog.Logger) *SessionActivityTracker {
	if debounce <= 0 {
		debounce = defaultSessionActivityDebounce
	}

	tracker := &SessionActivityTracker{
		lastWrite: make(map[string]time.Time),
		pool:      pool,
		logger:    logger,
		debounce:  debounce,
		nowFn:     time.Now,
	}
	tracker.updateFn = tracker.updateSessionLastActive
	return tracker
}

// Touch marks a session as active and triggers a debounced async DB update.
func (t *SessionActivityTracker) Touch(ctx context.Context, sessionID string) {
	if t == nil || sessionID == "" {
		return
	}

	now := t.nowFn()

	t.mu.Lock()
	last, ok := t.lastWrite[sessionID]
	if ok && now.Sub(last) < t.debounce {
		t.mu.Unlock()
		return
	}
	t.lastWrite[sessionID] = now
	updateFn := t.updateFn
	t.mu.Unlock()

	if updateFn == nil {
		return
	}

	updateCtx := context.WithoutCancel(ctx)
	go func() {
		if err := updateFn(updateCtx, sessionID); err != nil && t.logger != nil {
			t.logger.Error("failed to update session last_active_at", "session_id", sessionID, "error", err)
		}
	}()
}

func (t *SessionActivityTracker) updateSessionLastActive(ctx context.Context, sessionID string) error {
	if t.pool == nil {
		return nil
	}
	_, err := t.pool.Exec(ctx, `UPDATE _ayb_sessions SET last_active_at = NOW() WHERE id = $1`, sessionID)
	return err
}
