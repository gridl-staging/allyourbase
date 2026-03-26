// Package tenant This file implements a circuit breaker tracker to manage tenant availability and prevent cascading failures, with support for persisting state across restarts.
package tenant

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TenantBreakerConfig struct {
	FailureThreshold    int
	OpenDuration        time.Duration
	HalfOpenMaxRequests int
}

func NormalizeTenantBreakerConfig(cfg TenantBreakerConfig) TenantBreakerConfig {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.OpenDuration <= 0 {
		cfg.OpenDuration = 30 * time.Second
	}
	if cfg.HalfOpenMaxRequests <= 0 {
		cfg.HalfOpenMaxRequests = 1
	}
	return cfg
}

type TenantBreakerOpenError struct {
	TenantID   string
	RetryAfter time.Duration
}

func (e *TenantBreakerOpenError) Error() string {
	if e == nil {
		return "tenant circuit breaker open"
	}
	return fmt.Sprintf("tenant %q temporarily unavailable (circuit breaker open)", e.TenantID)
}

type tenantBreakerState struct {
	state               BreakerState
	consecutiveFailures int
	openedAt            time.Time
	halfOpenProbes      int
	lastFailureAt       time.Time
}

type TenantBreakerTracker struct {
	mu     sync.Mutex
	cfg    TenantBreakerConfig
	nowFn  func() time.Time
	states map[string]*tenantBreakerState
}

func NewTenantBreakerTracker(cfg TenantBreakerConfig, nowFn func() time.Time) *TenantBreakerTracker {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &TenantBreakerTracker{
		cfg:    NormalizeTenantBreakerConfig(cfg),
		nowFn:  nowFn,
		states: make(map[string]*tenantBreakerState),
	}
}

func (t *TenantBreakerTracker) getStateLocked(tenantID string) *tenantBreakerState {
	st, ok := t.states[tenantID]
	if !ok {
		st = &tenantBreakerState{state: BreakerStateClosed}
		t.states[tenantID] = st
	}
	return st
}

func (t *TenantBreakerTracker) advanceStateLocked(st *tenantBreakerState, now time.Time) {
	if st.state == BreakerStateOpen {
		if now.Sub(st.openedAt) >= t.cfg.OpenDuration {
			st.state = BreakerStateHalfOpen
			st.halfOpenProbes = 0
		}
	}
}

func (t *TenantBreakerTracker) State(tenantID string) BreakerState {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.nowFn()
	st := t.getStateLocked(tenantID)
	t.advanceStateLocked(st, now)
	return st.state
}

// BreakerSnapshot contains the full in-memory state for a tenant breaker,
// used by admin endpoints to return counters alongside the state enum.
type BreakerSnapshot struct {
	State               BreakerState
	ConsecutiveFailures int
	HalfOpenProbes      int
}

// StateSnapshot returns the full breaker state including internal counters.
func (t *TenantBreakerTracker) StateSnapshot(tenantID string) BreakerSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.nowFn()
	st := t.getStateLocked(tenantID)
	t.advanceStateLocked(st, now)
	return BreakerSnapshot{
		State:               st.state,
		ConsecutiveFailures: st.consecutiveFailures,
		HalfOpenProbes:      st.halfOpenProbes,
	}
}

// Allow returns nil if a request for the given tenant is permitted through the circuit breaker, or TenantBreakerOpenError if the breaker is open or half-open and has exhausted probe requests. When allowed in half-open state, the probe counter is incremented.
func (t *TenantBreakerTracker) Allow(tenantID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.nowFn()
	st := t.getStateLocked(tenantID)
	t.advanceStateLocked(st, now)

	switch st.state {
	case BreakerStateClosed:
		return nil
	case BreakerStateOpen:
		retryAfter := t.cfg.OpenDuration - now.Sub(st.openedAt)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return &TenantBreakerOpenError{TenantID: tenantID, RetryAfter: retryAfter}
	case BreakerStateHalfOpen:
		if st.halfOpenProbes >= t.cfg.HalfOpenMaxRequests {
			return &TenantBreakerOpenError{TenantID: tenantID}
		}
		st.halfOpenProbes++
		return nil
	default:
		return nil
	}
}

// RecordSuccess transitions the breaker to Closed and resets counters.
// Returns (previousState, newState) so callers can detect transitions atomically.
func (t *TenantBreakerTracker) RecordSuccess(tenantID string) (BreakerState, BreakerState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.getStateLocked(tenantID)
	prev := st.state
	st.state = BreakerStateClosed
	st.consecutiveFailures = 0
	st.halfOpenProbes = 0
	return prev, st.state
}

// RecordFailure increments consecutive failures and may transition the breaker
// to Open. Returns (previousState, newState) so callers can detect transitions atomically.
func (t *TenantBreakerTracker) RecordFailure(tenantID string) (BreakerState, BreakerState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.nowFn()
	st := t.getStateLocked(tenantID)
	t.advanceStateLocked(st, now)
	prev := st.state

	switch st.state {
	case BreakerStateClosed:
		st.consecutiveFailures++
		st.lastFailureAt = now
		if st.consecutiveFailures >= t.cfg.FailureThreshold {
			st.state = BreakerStateOpen
			st.openedAt = now
			st.halfOpenProbes = 0
		}
	case BreakerStateHalfOpen:
		st.state = BreakerStateOpen
		st.openedAt = now
		st.halfOpenProbes = 0
		st.lastFailureAt = now
	default:
	}
	return prev, st.state
}

func (t *TenantBreakerTracker) ResetBreaker(tenantID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.getStateLocked(tenantID)
	st.state = BreakerStateClosed
	st.consecutiveFailures = 0
	st.halfOpenProbes = 0
	st.openedAt = time.Time{}
	st.lastFailureAt = time.Time{}
}

type breakerPersistEntry struct {
	tenantID            string
	state               string
	consecutiveFailures int
	halfOpenProbes      int
	lastFailureAt       *time.Time
	openedAt            *time.Time
}

// Snapshot writes the current breaker state for all tenants to the database, using upsert to safely handle concurrent updates. State transitions are advanced before persisting to ensure the database reflects current state.
func (t *TenantBreakerTracker) Snapshot(ctx context.Context, pool *pgxpool.Pool) error {
	t.mu.Lock()
	entries := make([]breakerPersistEntry, 0, len(t.states))
	now := t.nowFn()
	for tenantID, st := range t.states {
		t.advanceStateLocked(st, now)
		entry := breakerPersistEntry{
			tenantID:            tenantID,
			state:               string(st.state),
			consecutiveFailures: st.consecutiveFailures,
			halfOpenProbes:      st.halfOpenProbes,
		}
		if !st.lastFailureAt.IsZero() {
			entry.lastFailureAt = &st.lastFailureAt
		}
		if !st.openedAt.IsZero() {
			entry.openedAt = &st.openedAt
		}
		entries = append(entries, entry)
	}
	t.mu.Unlock()

	for _, e := range entries {
		_, err := pool.Exec(ctx,
			`INSERT INTO _ayb_tenant_circuit_breaker 
				(tenant_id, state, consecutive_failures, half_open_probes, last_failure_at, opened_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (tenant_id) DO UPDATE SET
				state = EXCLUDED.state,
				consecutive_failures = EXCLUDED.consecutive_failures,
				half_open_probes = EXCLUDED.half_open_probes,
				last_failure_at = EXCLUDED.last_failure_at,
				opened_at = EXCLUDED.opened_at,
				updated_at = NOW()`,
			e.tenantID, e.state, e.consecutiveFailures, e.halfOpenProbes, e.lastFailureAt, e.openedAt,
		)
		if err != nil {
			return fmt.Errorf("snapshotting breaker state for tenant %s: %w", e.tenantID, err)
		}
	}
	return nil
}

// Restore populates the tracker's in-memory state by loading persisted circuit breaker state from the database. It is typically called on initialization to recover breaker state across server restarts.
func (t *TenantBreakerTracker) Restore(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx,
		`SELECT tenant_id, state, consecutive_failures, half_open_probes, last_failure_at, opened_at
		 FROM _ayb_tenant_circuit_breaker`,
	)
	if err != nil {
		return fmt.Errorf("restoring breaker state: %w", err)
	}
	defer rows.Close()

	var entries []breakerPersistEntry
	for rows.Next() {
		var e breakerPersistEntry
		if err := rows.Scan(&e.tenantID, &e.state, &e.consecutiveFailures, &e.halfOpenProbes, &e.lastFailureAt, &e.openedAt); err != nil {
			return fmt.Errorf("scanning breaker state row: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating breaker state rows: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	for _, e := range entries {
		st := &tenantBreakerState{
			state:               BreakerState(e.state),
			consecutiveFailures: e.consecutiveFailures,
			halfOpenProbes:      e.halfOpenProbes,
		}
		if e.lastFailureAt != nil {
			st.lastFailureAt = *e.lastFailureAt
		}
		if e.openedAt != nil {
			st.openedAt = *e.openedAt
		}
		t.states[e.tenantID] = st
	}
	return nil
}
