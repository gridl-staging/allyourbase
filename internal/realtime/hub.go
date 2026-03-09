package realtime

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/allyourbase/ayb/internal/auth"
)

// eventBufferSize is the per-client channel buffer. Events are dropped when full.
const eventBufferSize = 256

// Event represents a data change on a table.
type Event struct {
	Action string         `json:"action"` // "create", "update", "delete"
	Table  string         `json:"table"`
	Record map[string]any `json:"record"`
}

// Hub manages realtime SSE client connections and broadcasts events.
// It is safe for concurrent use.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client
	nextID  atomic.Uint64
	logger  *slog.Logger
}

// Client represents a connected SSE subscriber.
type Client struct {
	ID      string
	tables  map[string]bool
	events  chan *Event
	oauthCh chan *auth.OAuthEvent // non-nil only for OAuth SSE clients
}

// Events returns a read-only channel of table events for this client.
func (c *Client) Events() <-chan *Event {
	return c.events
}

// OAuthEvents returns a read-only channel of OAuth events, or nil for non-OAuth clients.
func (c *Client) OAuthEvents() <-chan *auth.OAuthEvent {
	return c.oauthCh
}

// NewHub creates a new realtime event hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients: make(map[string]*Client),
		logger:  logger,
	}
}

// Subscribe creates a new client subscribed to the given tables and registers it.
func (h *Hub) Subscribe(tables map[string]bool) *Client {
	id := fmt.Sprintf("c%d", h.nextID.Add(1))
	client := &Client{
		ID:     id,
		tables: tables,
		events: make(chan *Event, eventBufferSize),
	}

	h.mu.Lock()
	h.clients[id] = client
	h.mu.Unlock()

	h.logger.Debug("client subscribed", "id", id, "tables", tables)
	return client
}

// SubscribeOAuth creates a client for an OAuth SSE flow.
// The client's ID serves as the CSRF state token for the popup flow.
func (h *Hub) SubscribeOAuth() *Client {
	id := fmt.Sprintf("c%d", h.nextID.Add(1))
	client := &Client{
		ID:      id,
		events:  make(chan *Event, eventBufferSize),
		oauthCh: make(chan *auth.OAuthEvent, 1),
	}

	h.mu.Lock()
	h.clients[id] = client
	h.mu.Unlock()

	h.logger.Debug("oauth client subscribed", "id", id)
	return client
}

// HasClient returns true if a client with the given ID is connected.
func (h *Hub) HasClient(id string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.clients[id]
	return ok
}

// PublishOAuth sends an OAuth event to the specific client identified by clientID.
// This is targeted delivery (not broadcast). No-op if the client doesn't exist
// or is not an OAuth client.
func (h *Hub) PublishOAuth(clientID string, event *auth.OAuthEvent) {
	h.mu.RLock()
	client, ok := h.clients[clientID]
	h.mu.RUnlock()

	if !ok {
		h.logger.Warn("oauth publish: client not found", "clientID", clientID)
		return
	}
	if client.oauthCh == nil {
		h.logger.Warn("oauth publish: client is not an oauth client", "clientID", clientID)
		return
	}

	select {
	case client.oauthCh <- event:
		h.logger.Debug("oauth event published", "clientID", clientID)
	default:
		h.logger.Warn("oauth publish: channel full", "clientID", clientID)
	}
}

// Unsubscribe removes a client and closes its event channel(s).
func (h *Hub) Unsubscribe(clientID string) {
	h.mu.Lock()
	client, ok := h.clients[clientID]
	if ok {
		delete(h.clients, clientID)
		close(client.events)
		if client.oauthCh != nil {
			close(client.oauthCh)
		}
	}
	h.mu.Unlock()

	if ok {
		h.logger.Debug("client unsubscribed", "id", clientID)
	}
}

// Publish sends an event to all clients subscribed to the event's table.
// Uses non-blocking sends — events are dropped for clients with full buffers.
func (h *Hub) Publish(event *Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, client := range h.clients {
		if !client.tables[event.Table] {
			continue
		}
		select {
		case client.events <- event:
		default:
			h.logger.Warn("client buffer full, dropping event", "clientID", client.ID)
		}
	}
}

// Close disconnects all clients and clears the hub.
// Safe to call multiple times.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for id, client := range h.clients {
		close(client.events)
		if client.oauthCh != nil {
			close(client.oauthCh)
		}
		delete(h.clients, id)
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
