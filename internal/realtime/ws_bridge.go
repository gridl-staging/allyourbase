// Package realtime WSBridge connects WebSocket clients to the realtime Hub, managing subscriptions and forwarding filtered events.
package realtime

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/ws"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WSBridge wires a ws.Handler into the realtime Hub, forwarding
// Hub events to WebSocket clients with per-event RLS filtering.
type WSBridge struct {
	hub         *Hub
	pool        *pgxpool.Pool // nil when RLS filtering unavailable
	schemaCache *schema.CacheHolder
	logger      *slog.Logger

	mu      sync.Mutex
	clients map[string]string // ws.Conn.ID() → hub Client.ID
}

// NewWSBridge creates a bridge that connects a ws.Handler to the realtime Hub.
// pool may be nil; when non-nil, events are filtered per-client via RLS.
func NewWSBridge(hub *Hub, pool *pgxpool.Pool, schemaCache *schema.CacheHolder, logger *slog.Logger) *WSBridge {
	return &WSBridge{
		hub:         hub,
		pool:        pool,
		schemaCache: schemaCache,
		logger:      logger,
		clients:     make(map[string]string),
	}
}

// SetupHandler wires the OnConnect/OnDisconnect/OnSubscribe/OnUnsubscribe
// callbacks on the ws.Handler to manage Hub integration.
func (b *WSBridge) SetupHandler(h *ws.Handler) {
	h.OnConnect = b.onConnect
	h.OnDisconnect = b.onDisconnect
	h.OnSubscribe = b.onSubscribe
	h.OnUnsubscribe = b.onUnsubscribe
}

func (b *WSBridge) onConnect(c *ws.Conn) {
	// Register a Hub client with no tables. Tables are added via subscribe.
	client := b.hub.Subscribe(map[string]bool{})

	b.mu.Lock()
	b.clients[c.ID()] = client.ID
	b.mu.Unlock()

	b.logger.Debug("ws bridge: connected", "wsConn", c.ID(), "hubClient", client.ID)

	// Start forwarding goroutine: Hub events → ws.Conn.
	go b.forwardEvents(c, client)
}

func (b *WSBridge) onDisconnect(c *ws.Conn) {
	b.mu.Lock()
	hubClientID, ok := b.clients[c.ID()]
	if ok {
		delete(b.clients, c.ID())
	}
	b.mu.Unlock()

	if ok {
		b.hub.Unsubscribe(hubClientID)
		b.logger.Debug("ws bridge: disconnected", "wsConn", c.ID(), "hubClient", hubClientID)
	}
}

// onSubscribe updates the Hub client's subscriptions and filters to match the WebSocket client's current state. It parses the filter expression and returns an error if the filter is invalid.
func (b *WSBridge) onSubscribe(c *ws.Conn, _ []string, filter string) error {
	b.mu.Lock()
	hubClientID, ok := b.clients[c.ID()]
	b.mu.Unlock()

	if !ok {
		return nil
	}

	// Parse filter expression
	filters, err := ParseFilters(filter)
	if err != nil {
		return fmt.Errorf("invalid filter: %w", err)
	}

	// conn.Subscriptions() already reflects the new tables (ws.Handler
	// calls c.Subscribe before firing OnSubscribe).
	b.hub.SetTables(hubClientID, c.Subscriptions())
	b.hub.SetFilters(hubClientID, filters)
	return nil
}

func (b *WSBridge) onUnsubscribe(c *ws.Conn, _ []string) {
	b.mu.Lock()
	hubClientID, ok := b.clients[c.ID()]
	b.mu.Unlock()

	if ok {
		// conn.Subscriptions() already has the tables removed.
		b.hub.SetTables(hubClientID, c.Subscriptions())
	}
}

// forwardEvents reads events from the Hub client's channel and sends them
// to the WebSocket connection with RLS filtering applied.
func (b *WSBridge) forwardEvents(c *ws.Conn, client *Client) {
	ctx := context.Background()
	for event := range client.Events() {
		if !CanSeeRecord(ctx, b.pool, b.schemaCache, b.logger, c.Claims(), event) {
			continue
		}
		if !shouldDeliverEvent(event, client.Filters()) {
			continue
		}
		c.Send(ws.EventMsg(event.Action, event.Table, event.Record))
	}
}
