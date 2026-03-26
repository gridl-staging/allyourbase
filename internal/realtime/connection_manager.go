// Package realtime Provides a thread-safe connection registry and lifecycle management for realtime connections across SSE and WebSocket transports. Enforces per-user connection limits, idle cleanup, and graceful shutdown handling.
package realtime

import (
	"errors"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
)

// ErrLimitExceeded is returned by Register when the per-user connection limit is reached.
var ErrLimitExceeded = errors.New("realtime: per-user connection limit exceeded")

// ErrDraining is returned by Register when the manager is draining for shutdown.
var ErrDraining = errors.New("realtime: server is draining")

// ConnectionMeta holds metadata and lifecycle callbacks for an active realtime connection.
// CloseFunc and HasSubscriptions are provided by the transport implementation;
// ConnectedAt and LastActivityAt are managed by the ConnectionManager.
type ConnectionMeta struct {
	ClientID  string
	UserID    string
	Transport string // "sse" or "ws"

	// CloseFunc is called by the manager to terminate the connection.
	// For SSE, this should cancel the request context. For WS, this should
	// send an appropriate close frame.
	CloseFunc func()

	// HasSubscriptions reports whether the connection has any active table or
	// channel subscriptions. A nil value is treated as "no subscriptions", making
	// the connection immediately idle-eligible. SSE connections should always
	// return true (they subscribe at connect time via query params).
	HasSubscriptions func() bool

	// Set by Register; callers should leave these zero.
	ConnectedAt    time.Time
	LastActivityAt time.Time
	idleMarkedAt   time.Time
}

// UserKey derives the per-user limit key from auth claims. Returns
// claims.Subject when present; falls back to a sentinel so unauthenticated
// connections share a single limit pool.
func UserKey(claims *auth.Claims) string {
	if claims != nil && claims.Subject != "" {
		return claims.Subject
	}
	return "__anonymous__"
}

// ConnectionSnapshot is an admin-safe view of a connection with no function fields.
type ConnectionSnapshot struct {
	ClientID       string    `json:"clientId"`
	UserID         string    `json:"userId"`
	Transport      string    `json:"transport"`
	ConnectedAt    time.Time `json:"connectedAt"`
	LastActivityAt time.Time `json:"lastActivityAt"`
}

// ConnectionManagerOptions configures a ConnectionManager.
type ConnectionManagerOptions struct {
	// MaxConnectionsPerUser is the maximum concurrent connections per user key.
	// Zero or negative uses the default of 100.
	MaxConnectionsPerUser int

	// IdleTimeout is the duration after which an unsubscribed connection with no
	// recent activity is closed. Zero or negative uses the default of 60s.
	IdleTimeout time.Duration

	// SweepInterval controls how often the idle sweep runs.
	// Zero or negative uses the default of 10s. Set low in tests.
	SweepInterval time.Duration
}

// ConnectionManager is a thread-safe registry and lifecycle governor for realtime
// connections across both SSE and WebSocket transports. It enforces per-user
// connection limits, idle cleanup, and graceful drain.
type ConnectionManager struct {
	maxPerUser    int
	idleTimeout   time.Duration
	sweepInterval time.Duration

	mu       sync.Mutex
	conns    map[string]*ConnectionMeta
	draining bool

	stopCh chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
}

// NewConnectionManager creates and starts a ConnectionManager with the given options.
func NewConnectionManager(opts ConnectionManagerOptions) *ConnectionManager {
	maxPerUser := opts.MaxConnectionsPerUser
	if maxPerUser <= 0 {
		maxPerUser = 100
	}
	idleTimeout := opts.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = 60 * time.Second
	}
	sweepInterval := opts.SweepInterval
	if sweepInterval <= 0 {
		sweepInterval = 10 * time.Second
	}

	cm := &ConnectionManager{
		maxPerUser:    maxPerUser,
		idleTimeout:   idleTimeout,
		sweepInterval: sweepInterval,
		conns:         make(map[string]*ConnectionMeta),
		stopCh:        make(chan struct{}),
	}
	cm.wg.Add(1)
	go cm.sweepLoop()
	return cm
}

// Register records a new connection. Returns ErrLimitExceeded when the per-user
// connection limit is reached, or ErrDraining when the server is shutting down.
// On success, ConnectedAt and LastActivityAt are set on the meta.
func (cm *ConnectionManager) Register(meta ConnectionMeta) error {
	now := time.Now()
	meta.ConnectedAt = now
	meta.LastActivityAt = now

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.draining {
		return ErrDraining
	}

	count := 0
	for _, m := range cm.conns {
		if m.UserID == meta.UserID {
			count++
		}
	}
	if count >= cm.maxPerUser {
		return ErrLimitExceeded
	}

	cp := meta
	cm.conns[meta.ClientID] = &cp
	return nil
}

// Deregister removes a connection. Safe to call when the ID is not present —
// this is a no-op and not an error (normal during disconnect races).
func (cm *ConnectionManager) Deregister(clientID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.conns, clientID)
}

// TouchActivity updates the last-activity timestamp for the given connection.
// No-op if the connection is not registered.
func (cm *ConnectionManager) TouchActivity(clientID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if m, ok := cm.conns[clientID]; ok {
		m.LastActivityAt = time.Now()
		m.idleMarkedAt = time.Time{}
	}
}

// Snapshot returns an admin-safe list of all active connections, without
// exposing function fields.
func (cm *ConnectionManager) Snapshot() []ConnectionSnapshot {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	out := make([]ConnectionSnapshot, 0, len(cm.conns))
	for _, m := range cm.conns {
		out = append(out, ConnectionSnapshot{
			ClientID:       m.ClientID,
			UserID:         m.UserID,
			Transport:      m.Transport,
			ConnectedAt:    m.ConnectedAt,
			LastActivityAt: m.LastActivityAt,
		})
	}
	return out
}

// ForceDisconnect calls the closeFunc for the given clientID and deregisters it.
// Returns true if the connection was found and closed, false if not found.
func (cm *ConnectionManager) ForceDisconnect(clientID string) bool {
	cm.mu.Lock()
	meta, ok := cm.conns[clientID]
	if !ok {
		cm.mu.Unlock()
		return false
	}
	closeFn := meta.CloseFunc
	delete(cm.conns, clientID)
	cm.mu.Unlock()

	closeFn()
	return true
}

// Stop shuts down the background sweep goroutine. Idempotent.
func (cm *ConnectionManager) Stop() {
	cm.once.Do(func() { close(cm.stopCh) })
	cm.wg.Wait()
}

// Drain signals that the server is shutting down so Register rejects new connections,
// then waits up to timeout for existing connections to deregister naturally, and
// force-closes any that remain. Stop is called before Drain returns.
func (cm *ConnectionManager) Drain(timeout time.Duration) {
	cm.mu.Lock()
	cm.draining = true
	cm.mu.Unlock()

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		cm.mu.Lock()
		remaining := len(cm.conns)
		cm.mu.Unlock()

		if remaining == 0 {
			break
		}
		if time.Now().After(deadline) {
			cm.forceCloseAll()
			break
		}
		<-ticker.C
	}

	cm.Stop()
}

// forceCloseAll calls CloseFunc on all remaining connections and clears the map.
// Caller must NOT hold cm.mu.
func (cm *ConnectionManager) forceCloseAll() {
	cm.mu.Lock()
	metas := make([]*ConnectionMeta, 0, len(cm.conns))
	for _, m := range cm.conns {
		metas = append(metas, m)
	}
	cm.conns = make(map[string]*ConnectionMeta)
	cm.mu.Unlock()

	for _, m := range metas {
		m.CloseFunc()
	}
}

// sweepLoop runs the idle-connection sweep on sweepInterval until Stop is called.
func (cm *ConnectionManager) sweepLoop() {
	defer cm.wg.Done()
	ticker := time.NewTicker(cm.sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.sweepIdle()
		}
	}
}

// sweepIdle closes and deregisters connections that are idle-eligible:
// no active subscriptions AND lastActivityAt older than IdleTimeout.
func (cm *ConnectionManager) sweepIdle() {
	cm.mu.Lock()
	now := time.Now()
	toClose := make([]*ConnectionMeta, 0, len(cm.conns))
	for _, m := range cm.conns {
		subscribed := m.HasSubscriptions != nil && m.HasSubscriptions()
		idle := now.Sub(m.LastActivityAt) >= cm.idleTimeout
		if subscribed || !idle {
			m.idleMarkedAt = time.Time{}
			continue
		}

		if m.idleMarkedAt.IsZero() {
			m.idleMarkedAt = now
			continue
		}

		if now.Sub(m.idleMarkedAt) < cm.sweepInterval {
			continue
		}

		delete(cm.conns, m.ClientID)
		m.idleMarkedAt = time.Time{}
		toClose = append(toClose, m)
	}
	cm.mu.Unlock()

	for _, m := range toClose {
		m.CloseFunc()
	}
}
