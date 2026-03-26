// Package schema CacheHolder provides thread-safe access to an immutable schema cache with lock-free reads and atomic updates. It coordinates initialization, reloading, and testing of schema metadata.
package schema

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CacheHolder provides thread-safe access to the current SchemaCache.
// Reads are lock-free (atomic pointer load). Writes build a new immutable
// SchemaCache and swap it in atomically.
type CacheHolder struct {
	cache     atomic.Pointer[SchemaCache]
	mu        sync.Mutex  // serializes reloads
	loading   atomic.Bool // prevents concurrent reloads
	pool      *pgxpool.Pool
	logger    *slog.Logger
	ready     chan struct{} // closed after the first successful load
	readyOnce sync.Once     // ensures ready is closed exactly once
	readyInit sync.Once     // ensures ready channel is created exactly once
}

// NewCacheHolder creates a CacheHolder. Call Load() to perform the initial introspection.
func NewCacheHolder(pool *pgxpool.Pool, logger *slog.Logger) *CacheHolder {
	return &CacheHolder{
		pool:   pool,
		logger: logger,
		ready:  make(chan struct{}),
	}
}

// ensureReady lazily initializes the ready channel so that zero-value
// CacheHolder structs (created without NewCacheHolder) are safe to use.
func (h *CacheHolder) ensureReady() {
	h.readyInit.Do(func() {
		if h.ready == nil {
			h.ready = make(chan struct{})
		}
	})
}

// Ready returns a channel that is closed once the first schema load completes.
func (h *CacheHolder) Ready() <-chan struct{} {
	h.ensureReady()
	return h.ready
}

// Get returns the current schema cache. Lock-free, safe for concurrent use.
// Returns nil if the cache has not been loaded yet.
func (h *CacheHolder) Get() *SchemaCache {
	return h.cache.Load()
}

// Load performs the initial schema introspection. Must be called before Get().
func (h *CacheHolder) Load(ctx context.Context) error {
	return h.Reload(ctx)
}

// SetForTesting directly sets the schema cache. Intended for unit tests that
// cannot provide a real database connection.
func (h *CacheHolder) SetForTesting(sc *SchemaCache) {
	h.ensureReady()
	h.cache.Store(sc)
	if sc != nil {
		h.readyOnce.Do(func() { close(h.ready) })
	}
}

// Reload re-introspects the database and atomically swaps the cache.
// Returns immediately if a reload is already in progress.
func (h *CacheHolder) Reload(ctx context.Context) error {
	if !h.loading.CompareAndSwap(false, true) {
		h.logger.Debug("schema reload already in progress, skipping")
		return nil
	}
	defer h.loading.Store(false)

	h.mu.Lock()
	defer h.mu.Unlock()

	return h.reloadLocked(ctx)
}

// ReloadWait re-introspects the database and atomically swaps the cache.
// Unlike Reload, it waits for any in-progress reload to finish before
// performing its own. Use this when the caller needs to guarantee the
// cache reflects changes just committed (e.g., after DDL in the admin SQL editor).
func (h *CacheHolder) ReloadWait(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.reloadLocked(ctx)
}

// reloadLocked builds a new schema cache from the database and atomically swaps it in. Must be called with h.mu held. On first successful load, closes the ready channel to signal cache availability.
func (h *CacheHolder) reloadLocked(ctx context.Context) error {
	sc, err := BuildCache(ctx, h.pool)
	if err != nil {
		return fmt.Errorf("building schema cache: %w", err)
	}

	tableCount := len(sc.Tables)
	h.cache.Store(sc)
	// Signal readiness on first successful load.
	h.ensureReady()
	h.readyOnce.Do(func() { close(h.ready) })

	h.logger.Info("schema cache loaded",
		"tables", tableCount,
		"schemas", sc.Schemas,
		"builtAt", sc.BuiltAt,
	)

	return nil
}
