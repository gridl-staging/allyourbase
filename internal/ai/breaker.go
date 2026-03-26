package ai

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// BreakerState models provider health for circuit-breaking.
//
// State machine:
// - closed: normal operation, failures are counted.
// - open: calls are short-circuited until open_duration elapses.
// - half_open: limited probe requests are allowed; success closes, failure reopens.
//
// Isolation boundary is provider-name scoped: one provider's failures never open
// another provider's breaker.
type BreakerState string

const (
	BreakerStateClosed   BreakerState = "closed"
	BreakerStateOpen     BreakerState = "open"
	BreakerStateHalfOpen BreakerState = "half_open"
)

// BreakerConfig controls circuit-breaker behavior.
type BreakerConfig struct {
	FailureThreshold    int
	OpenDuration        time.Duration
	HalfOpenMaxRequests int
}

// BreakerOpenError is returned when a provider call is short-circuited.
type BreakerOpenError struct {
	Provider   string
	RetryAfter time.Duration
}

func (e *BreakerOpenError) Error() string {
	if e == nil {
		return "ai provider circuit breaker open"
	}
	if e.Provider == "" {
		return "ai provider temporarily unavailable (circuit breaker open)"
	}
	return fmt.Sprintf("ai provider %q temporarily unavailable (circuit breaker open)", e.Provider)
}

type providerBreakerState struct {
	state               BreakerState
	consecutiveFailures int
	openedAt            time.Time
	halfOpenProbes      int
}

// ProviderHealthTracker tracks circuit-breaker state per provider.
type ProviderHealthTracker struct {
	mu     sync.Mutex
	cfg    BreakerConfig
	nowFn  func() time.Time
	states map[string]*providerBreakerState
}

func normalizeBreakerConfig(cfg BreakerConfig) BreakerConfig {
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

// NewProviderHealthTracker creates a provider-scoped breaker tracker.
func NewProviderHealthTracker(cfg BreakerConfig, nowFn func() time.Time) *ProviderHealthTracker {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &ProviderHealthTracker{
		cfg:    normalizeBreakerConfig(cfg),
		nowFn:  nowFn,
		states: make(map[string]*providerBreakerState),
	}
}

func (t *ProviderHealthTracker) getStateLocked(provider string) *providerBreakerState {
	st, ok := t.states[provider]
	if !ok {
		st = &providerBreakerState{state: BreakerStateClosed}
		t.states[provider] = st
	}
	return st
}

func (t *ProviderHealthTracker) advanceStateLocked(st *providerBreakerState, now time.Time) {
	if st.state == BreakerStateOpen {
		if now.Sub(st.openedAt) >= t.cfg.OpenDuration {
			st.state = BreakerStateHalfOpen
			st.halfOpenProbes = 0
		}
	}
}

// State returns the current breaker state for a provider.
func (t *ProviderHealthTracker) State(provider string) BreakerState {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.nowFn()
	st := t.getStateLocked(provider)
	t.advanceStateLocked(st, now)
	return st.state
}

// Allow checks whether a provider call is currently permitted.
func (t *ProviderHealthTracker) Allow(provider string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.nowFn()
	st := t.getStateLocked(provider)
	t.advanceStateLocked(st, now)

	switch st.state {
	case BreakerStateClosed:
		return nil
	case BreakerStateOpen:
		retryAfter := t.cfg.OpenDuration - now.Sub(st.openedAt)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return &BreakerOpenError{Provider: provider, RetryAfter: retryAfter}
	case BreakerStateHalfOpen:
		if st.halfOpenProbes >= t.cfg.HalfOpenMaxRequests {
			return &BreakerOpenError{Provider: provider}
		}
		st.halfOpenProbes++
		return nil
	default:
		return nil
	}
}

// RecordSuccess records a successful provider call.
func (t *ProviderHealthTracker) RecordSuccess(provider string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.getStateLocked(provider)
	st.state = BreakerStateClosed
	st.consecutiveFailures = 0
	st.halfOpenProbes = 0
}

// RecordFailure records a failed provider call and transitions breaker state.
func (t *ProviderHealthTracker) RecordFailure(provider string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.nowFn()
	st := t.getStateLocked(provider)
	t.advanceStateLocked(st, now)

	switch st.state {
	case BreakerStateClosed:
		st.consecutiveFailures++
		if st.consecutiveFailures >= t.cfg.FailureThreshold {
			st.state = BreakerStateOpen
			st.openedAt = now
			st.halfOpenProbes = 0
		}
	case BreakerStateHalfOpen:
		st.state = BreakerStateOpen
		st.openedAt = now
		st.halfOpenProbes = 0
	default:
		// Keep open state until timer elapses.
	}
}

// NewBreakerProvider wraps a provider with circuit-breaker checks.
// Optional provider interfaces (embedding, streaming) are preserved.
func NewBreakerProvider(inner Provider, providerName string, tracker *ProviderHealthTracker) Provider {
	if tracker == nil {
		return inner
	}
	bp := &breakerProvider{inner: inner, providerName: providerName, tracker: tracker}
	_, hasEmbedding := inner.(EmbeddingProvider)
	_, hasStreaming := inner.(StreamingProvider)
	switch {
	case hasEmbedding && hasStreaming:
		return &breakerEmbeddingStreamingProvider{breakerProvider: bp}
	case hasEmbedding:
		return &breakerEmbeddingProvider{breakerProvider: bp}
	case hasStreaming:
		return &breakerStreamingProvider{breakerProvider: bp}
	default:
		return bp
	}
}

type breakerProvider struct {
	inner        Provider
	providerName string
	tracker      *ProviderHealthTracker
}

func (bp *breakerProvider) GenerateText(ctx context.Context, req GenerateTextRequest) (GenerateTextResponse, error) {
	if err := bp.tracker.Allow(bp.providerName); err != nil {
		return GenerateTextResponse{}, err
	}
	resp, err := bp.inner.GenerateText(ctx, req)
	if err != nil {
		bp.tracker.RecordFailure(bp.providerName)
		return GenerateTextResponse{}, err
	}
	bp.tracker.RecordSuccess(bp.providerName)
	return resp, nil
}

type breakerEmbeddingProvider struct {
	*breakerProvider
}

func (bp *breakerEmbeddingProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	ep := bp.inner.(EmbeddingProvider) // safe: checked in constructor
	if err := bp.tracker.Allow(bp.providerName); err != nil {
		return EmbeddingResponse{}, err
	}
	resp, err := ep.GenerateEmbedding(ctx, req)
	if err != nil {
		bp.tracker.RecordFailure(bp.providerName)
		return EmbeddingResponse{}, err
	}
	bp.tracker.RecordSuccess(bp.providerName)
	return resp, nil
}

type breakerStreamingProvider struct {
	*breakerProvider
}

func (bp *breakerStreamingProvider) GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	sp := bp.inner.(StreamingProvider) // safe: checked in constructor
	if err := bp.tracker.Allow(bp.providerName); err != nil {
		return nil, err
	}
	reader, err := sp.GenerateTextStream(ctx, req)
	if err != nil {
		bp.tracker.RecordFailure(bp.providerName)
		return nil, err
	}
	bp.tracker.RecordSuccess(bp.providerName)
	return reader, nil
}

type breakerEmbeddingStreamingProvider struct {
	*breakerProvider
}

func (bp *breakerEmbeddingStreamingProvider) GenerateEmbedding(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error) {
	return (&breakerEmbeddingProvider{breakerProvider: bp.breakerProvider}).GenerateEmbedding(ctx, req)
}

func (bp *breakerEmbeddingStreamingProvider) GenerateTextStream(ctx context.Context, req GenerateTextRequest) (io.ReadCloser, error) {
	return (&breakerStreamingProvider{breakerProvider: bp.breakerProvider}).GenerateTextStream(ctx, req)
}
