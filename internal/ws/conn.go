// Package ws The file defines Conn, a thread-safe WebSocket connection wrapper with authentication, subscriptions, and presence support.
package ws

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/allyourbase/ayb/internal/auth"
)

// Connection lifecycle constants.
const (
	writeWait      = 10 * time.Second
	pongWait       = 30 * time.Second
	pingInterval   = 25 * time.Second // must be < pongWait
	authTimeout    = 10 * time.Second
	maxMessageSize = 4096 // 4KB
	sendBufferSize = 256
)

// Conn wraps a gorilla/websocket.Conn with authentication state,
// subscriptions, and a buffered outbound channel.
// Conn wraps a gorilla/websocket connection with authentication state, table and channel subscriptions, presence information, and a buffered send channel. All shared state is protected by mutex. Shutdown is idempotent via the done channel and sync.Once.
type Conn struct {
	id     string
	ws     *websocket.Conn
	logger *slog.Logger

	mu            sync.Mutex
	authenticated bool
	claims        *auth.Claims
	subscriptions map[string]bool
	channels      map[string]bool
	presence      map[string]map[string]any // channel -> presence payload

	send chan ServerMessage
	done chan struct{}
	once sync.Once // ensures done is closed exactly once
	// onDrop runs when Send drops due to a full buffer.
	onDrop func()
}

// newConn creates a new Conn wrapping the given websocket connection.
func newConn(id string, ws *websocket.Conn, logger *slog.Logger) *Conn {
	return &Conn{
		id:            id,
		ws:            ws,
		logger:        logger,
		subscriptions: make(map[string]bool),
		channels:      make(map[string]bool),
		presence:      make(map[string]map[string]any),
		send:          make(chan ServerMessage, sendBufferSize),
		done:          make(chan struct{}),
	}
}

// ID returns the connection's unique identifier.
func (c *Conn) ID() string {
	return c.id
}

// Send queues a message for delivery. Non-blocking: drops the message
// if the send buffer is full.
func (c *Conn) Send(msg ServerMessage) {
	select {
	case c.send <- msg:
	default:
		if c.onDrop != nil {
			c.onDrop()
		}
		c.logger.Warn("dropping message, send buffer full", "conn", c.id)
	}
}

// Close performs an idempotent close: signals the done channel and sends
// a WebSocket close frame. Safe to call multiple times.
func (c *Conn) Close(code int, reason string) {
	c.once.Do(func() {
		close(c.done)
		// Best-effort close frame — ignore errors since the connection may
		// already be broken.
		_ = c.ws.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, reason),
			time.Now().Add(writeWait),
		)
		_ = c.ws.Close()
	})
}

// Authenticated returns whether the connection has been authenticated.
func (c *Conn) Authenticated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authenticated
}

// setAuth marks the connection as authenticated with the given claims.
func (c *Conn) setAuth(claims *auth.Claims) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authenticated = true
	c.claims = claims
}

// Claims returns the auth claims, or nil if unauthenticated.
func (c *Conn) Claims() *auth.Claims {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.claims
}

// Subscribe adds tables to the subscription set.
func (c *Conn) Subscribe(tables []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range tables {
		c.subscriptions[t] = true
	}
}

// Unsubscribe removes tables from the subscription set.
func (c *Conn) Unsubscribe(tables []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range tables {
		delete(c.subscriptions, t)
	}
}

// Subscriptions returns a copy of the current subscriptions.
func (c *Conn) Subscriptions() map[string]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make(map[string]bool, len(c.subscriptions))
	for k, v := range c.subscriptions {
		cp[k] = v
	}
	return cp
}

// SubscribeChannel adds a channel subscription.
func (c *Conn) SubscribeChannel(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.channels[channel] = true
}

// UnsubscribeChannel removes a channel subscription.
func (c *Conn) UnsubscribeChannel(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.channels, channel)
}

// Channels returns a copy of the current channel subscriptions.
func (c *Conn) Channels() map[string]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make(map[string]bool, len(c.channels))
	for k, v := range c.channels {
		cp[k] = v
	}
	return cp
}

// HasChannel reports whether the connection is subscribed to the given channel.
func (c *Conn) HasChannel(channel string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.channels[channel]
}

// SetPresence sets the connection's presence payload for a channel.
func (c *Conn) SetPresence(channel string, payload map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.presence[channel] = copyMap(payload)
}

// ClearPresence clears the connection's presence payload for a channel.
func (c *Conn) ClearPresence(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.presence, channel)
}

// Presence returns a copy of the channel presence payload, or nil.
func (c *Conn) Presence(channel string) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	payload, ok := c.presence[channel]
	if !ok {
		return nil
	}
	return copyMap(payload)
}

func copyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	cp := make(map[string]any, len(src))
	for k, v := range src {
		cp[k] = v
	}
	return cp
}
