package schema

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	notifyChannel  = "ayb_schema_changed"
	reconnectDelay = 5 * time.Second
	debounceDelay  = 500 * time.Millisecond
	pollInterval   = 60 * time.Second
	listenTimeout  = 30 * time.Second
)

// Watcher listens for DDL change notifications and triggers schema cache reloads.
// If event triggers cannot be installed (insufficient privileges), it falls back
// to periodic polling.
type Watcher struct {
	cache      *CacheHolder
	pool       *pgxpool.Pool
	connString string
	logger     *slog.Logger
	pollMode   bool

	// Debounce state: multiple notifications within debounceDelay trigger one reload.
	debounceMu    sync.Mutex
	debounceTimer *time.Timer
}

// NewWatcher creates a schema change watcher.
func NewWatcher(cache *CacheHolder, pool *pgxpool.Pool, connString string, logger *slog.Logger) *Watcher {
	return &Watcher{
		cache:      cache,
		pool:       pool,
		connString: connString,
		logger:     logger,
	}
}

// Start installs event triggers (if possible), performs the initial schema load,
// and starts the background listener. It blocks until ctx is cancelled.
// Run this in a goroutine.
func (w *Watcher) Start(ctx context.Context) error {
	// Try to install event triggers for DDL notifications.
	if err := w.ensureTriggers(ctx); err != nil {
		w.logger.Warn("DDL event triggers not available, using polling for schema changes",
			"error", err, "interval", pollInterval)
		w.pollMode = true
	} else {
		w.logger.Info("DDL event triggers installed, listening for schema changes")
	}

	// For LISTEN/NOTIFY mode, establish the listener BEFORE the initial cache load
	// so we don't miss any notifications that fire right after Ready() is signaled.
	var listenerConn *pgx.Conn
	if !w.pollMode {
		conn, err := pgx.Connect(ctx, w.connString)
		if err != nil {
			w.logger.Warn("could not establish listener connection, falling back to polling",
				"error", err)
			w.pollMode = true
		} else {
			if _, err := conn.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
				conn.Close(ctx)
				w.logger.Warn("LISTEN failed, falling back to polling", "error", err)
				w.pollMode = true
			} else {
				listenerConn = conn
				w.logger.Debug("listening on channel", "channel", notifyChannel)
			}
		}
	}

	// Initial schema load (signals Ready via CacheHolder).
	if err := w.cache.Load(ctx); err != nil {
		if listenerConn != nil {
			listenerConn.Close(ctx)
		}
		return fmt.Errorf("initial schema load: %w", err)
	}

	// Start background listener/poller.
	if w.pollMode {
		return w.runPoller(ctx)
	}
	return w.runListenerWithConn(ctx, listenerConn)
}

// runListenerWithConn runs the notification loop using an already-established
// LISTEN connection. On disconnect, reconnects automatically.
func (w *Watcher) runListenerWithConn(ctx context.Context, conn *pgx.Conn) error {
	// Run the notification loop with the pre-established connection.
	err := w.notifyLoop(ctx, conn)
	conn.Close(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// On disconnect, fall into the reconnect loop.
	w.logger.Warn("schema listener connection lost, reconnecting",
		"error", err, "delay", reconnectDelay)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(reconnectDelay):
	}
	return w.runListener(ctx)
}

// runListener connects to PostgreSQL with a dedicated connection and listens
// for schema change notifications. Reconnects automatically on connection loss.
func (w *Watcher) runListener(ctx context.Context) error {
	for {
		err := w.listenLoop(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		w.logger.Warn("schema listener connection lost, reconnecting",
			"error", err, "delay", reconnectDelay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(reconnectDelay):
		}
	}
}

func (w *Watcher) listenLoop(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, w.connString)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	w.logger.Debug("listening on channel", "channel", notifyChannel)

	// On reconnect, do a full reload in case we missed notifications.
	w.scheduleReload(ctx)

	return w.notifyLoop(ctx, conn)
}

// notifyLoop waits for notifications on an established connection.
func (w *Watcher) notifyLoop(ctx context.Context, conn *pgx.Conn) error {
	for {
		waitCtx, cancel := context.WithTimeout(ctx, listenTimeout)
		notification, err := conn.WaitForNotification(waitCtx)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Timeout is normal; just loop to keep the connection alive.
			if waitCtx.Err() == context.DeadlineExceeded {
				continue
			}
			return fmt.Errorf("wait: %w", err)
		}

		w.logger.Info("schema change notification received",
			"channel", notification.Channel,
			"payload", notification.Payload)
		w.scheduleReload(ctx)
	}
}

// runPoller periodically reloads the schema cache when event triggers aren't available.
func (w *Watcher) runPoller(ctx context.Context) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			w.logger.Debug("polling for schema changes")
			if err := w.cache.Reload(ctx); err != nil {
				w.logger.Error("schema cache poll reload failed", "error", err)
			}
		}
	}
}

// scheduleReload debounces reload requests. Multiple calls within debounceDelay
// result in a single reload.
func (w *Watcher) scheduleReload(ctx context.Context) {
	w.debounceMu.Lock()
	defer w.debounceMu.Unlock()

	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}
	w.debounceTimer = time.AfterFunc(debounceDelay, func() {
		reloadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := w.cache.Reload(reloadCtx); err != nil {
			w.logger.Error("schema cache reload failed", "error", err)
		}
	})
}

// ensureTriggers attempts to create the DDL event trigger functions and event
// triggers needed for LISTEN/NOTIFY schema change detection.
func (w *Watcher) ensureTriggers(ctx context.Context) error {
	// Create the notification functions (requires CREATE privilege on schema).
	_, err := w.pool.Exec(ctx, ddlNotifyFunctionSQL)
	if err != nil {
		return fmt.Errorf("creating DDL notify function: %w", err)
	}

	_, err = w.pool.Exec(ctx, dropNotifyFunctionSQL)
	if err != nil {
		return fmt.Errorf("creating drop notify function: %w", err)
	}

	// Create event triggers (requires superuser or appropriate privileges).
	// Wrapped in a DO block with exception handling for graceful degradation.
	_, err = w.pool.Exec(ctx, createEventTriggersSQL)
	if err != nil {
		return fmt.Errorf("creating event triggers: %w", err)
	}

	return nil
}

const ddlNotifyFunctionSQL = `
CREATE OR REPLACE FUNCTION _ayb_ddl_notify() RETURNS event_trigger
LANGUAGE plpgsql AS $$
DECLARE
  cmd record;
BEGIN
  FOR cmd IN SELECT * FROM pg_event_trigger_ddl_commands()
  LOOP
    IF cmd.command_tag IN (
      'CREATE TABLE', 'CREATE TABLE AS', 'SELECT INTO', 'ALTER TABLE',
      'CREATE VIEW', 'ALTER VIEW',
      'CREATE MATERIALIZED VIEW', 'ALTER MATERIALIZED VIEW',
      'CREATE TYPE', 'ALTER TYPE',
      'CREATE FUNCTION', 'ALTER FUNCTION'
    )
    AND cmd.schema_name IS DISTINCT FROM 'pg_temp'
    AND cmd.schema_name NOT LIKE 'pg_%'
    AND cmd.schema_name != 'information_schema'
    THEN
      NOTIFY ayb_schema_changed, 'reload';
      RETURN;
    END IF;
  END LOOP;
END;
$$`

const dropNotifyFunctionSQL = `
CREATE OR REPLACE FUNCTION _ayb_drop_notify() RETURNS event_trigger
LANGUAGE plpgsql AS $$
DECLARE
  obj record;
BEGIN
  FOR obj IN SELECT * FROM pg_event_trigger_dropped_objects()
  LOOP
    IF obj.object_type IN (
      'table', 'foreign table', 'view', 'materialized view', 'type', 'function'
    )
    AND obj.is_temporary IS false
    AND obj.schema_name NOT LIKE 'pg_%'
    AND obj.schema_name != 'information_schema'
    THEN
      NOTIFY ayb_schema_changed, 'reload';
      RETURN;
    END IF;
  END LOOP;
END;
$$`

const createEventTriggersSQL = `
DO $$
BEGIN
  EXECUTE 'DROP EVENT TRIGGER IF EXISTS _ayb_ddl_watch';
  EXECUTE 'CREATE EVENT TRIGGER _ayb_ddl_watch ON ddl_command_end EXECUTE FUNCTION _ayb_ddl_notify()';
  EXECUTE 'DROP EVENT TRIGGER IF EXISTS _ayb_drop_watch';
  EXECUTE 'CREATE EVENT TRIGGER _ayb_drop_watch ON sql_drop EXECUTE FUNCTION _ayb_drop_notify()';
EXCEPTION WHEN insufficient_privilege THEN
  RAISE EXCEPTION 'insufficient privileges for event triggers';
END;
$$`
