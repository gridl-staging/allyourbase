// Package ws implements WebSocket message handling for real-time subscriptions, channel broadcasting, and presence state tracking.
package ws

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
)

// TokenValidator abstracts token validation for testability.
// In production, *auth.Service satisfies this interface.
type TokenValidator interface {
	ValidateToken(token string) (*auth.Claims, error)
	ValidateAPIKey(ctx context.Context, token string) (*auth.Claims, error)
}

// Handler manages WebSocket connections. It upgrades HTTP requests,
// authenticates clients, and runs read/write loops for each connection.
// Handler manages WebSocket connections, upgrading HTTP requests, authenticating clients, and running read and write loops for each connection. It integrates with broadcast and presence hubs to enable channel-based client-to-client messaging and presence state tracking.
type Handler struct {
	authValidator     TokenValidator // nil when auth is disabled
	logger            *slog.Logger
	upgrader          websocket.Upgrader
	nextID            atomic.Uint64
	droppedMessages   atomic.Uint64
	heartbeatFailures atomic.Uint64

	// Configurable timeouts (zero = use package defaults).
	AuthTimeout  time.Duration
	PingInterval time.Duration

	mu    sync.Mutex
	conns map[string]*Conn

	// Callbacks for Stage 6 integration. All are optional (nil-safe).
	OnConnect     func(c *Conn)
	OnDisconnect  func(c *Conn)
	OnSubscribe   func(c *Conn, tables []string, filter string) error
	OnUnsubscribe func(c *Conn, tables []string)

	// Broadcast enables channel-based client-to-client relay; nil disables it.
	Broadcast *BroadcastHub
	// Presence enables channel presence state tracking; nil disables it.
	Presence *PresenceHub
}

// NewHandler creates a WebSocket handler.
// authValidator may be nil to disable authentication.
// *auth.Service satisfies the TokenValidator interface.
func NewHandler(authValidator TokenValidator, logger *slog.Logger) *Handler {
	return &Handler{
		authValidator: authValidator,
		logger:        logger,
		conns:         make(map[string]*Conn),
		upgrader: websocket.Upgrader{
			CheckOrigin: httputil.CheckWebSocketOrigin,
		},
	}
}

func (h *Handler) authTimeoutDuration() time.Duration {
	if h.AuthTimeout > 0 {
		return h.AuthTimeout
	}
	return authTimeout
}

func (h *Handler) pingIntervalDuration() time.Duration {
	if h.PingInterval > 0 {
		return h.PingInterval
	}
	return pingInterval
}

// ServeHTTP upgrades the HTTP connection to WebSocket and starts the
// connection lifecycle.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wsConn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", "error", err)
		return
	}

	id := fmt.Sprintf("ws-%d", h.nextID.Add(1))
	c := newConn(id, wsConn, h.logger)
	h.setConnDropHook(c)

	h.trackConn(c)
	defer h.removeConn(c)

	// Try to authenticate from header or query param at upgrade time.
	authenticated := false
	if h.authValidator != nil {
		token, ok := httputil.ExtractBearerToken(r)
		if !ok {
			token = r.URL.Query().Get("token")
			ok = token != ""
		}
		if ok {
			claims, authErr := h.validateToken(r.Context(), token)
			if authErr != nil {
				h.logger.Warn("ws auth failed at upgrade", "error", authErr, "conn", id)
				c.Close(4401, "authentication failed")
				return
			}
			c.setAuth(claims)
			authenticated = true
		}
	} else {
		// No auth validator — auto-authenticate.
		c.setAuth(nil)
		authenticated = true
	}

	// Send connected message.
	c.Send(connectedMsg(id))

	if h.OnConnect != nil {
		h.OnConnect(c)
	}

	// Start auth timeout if not authenticated at upgrade.
	var authTimer *time.Timer
	if !authenticated {
		authTimer = time.AfterFunc(h.authTimeoutDuration(), func() {
			if !c.Authenticated() {
				h.logger.Warn("auth timeout", "conn", id)
				c.Close(4401, "auth timeout")
			}
		})
	}

	ctx := r.Context()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		h.readLoop(ctx, c, authTimer)
	}()
	go func() {
		defer wg.Done()
		h.writeLoop(c)
	}()
	wg.Wait()

	if h.Broadcast != nil {
		h.Broadcast.UnsubscribeAll(c)
	}
	if h.Presence != nil {
		h.Presence.DeferredUntrackAll(c, h.sendPresenceDiff)
	}

	if h.OnDisconnect != nil {
		h.OnDisconnect(c)
	}
}

// readLoop reads messages from the WebSocket and dispatches them.
func (h *Handler) readLoop(ctx context.Context, c *Conn, authTimer *time.Timer) {
	defer c.Close(websocket.CloseNormalClosure, "")

	c.ws.SetReadLimit(maxMessageSize)
	_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, data, err := c.ws.ReadMessage()
		if err != nil {
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				h.recordHeartbeatFailure()
			}
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				h.logger.Warn("ws read error", "conn", c.id, "error", err)
			}
			return
		}

		msg, parseErr := parseClientMessage(data)
		if parseErr != nil {
			c.Send(errorMsg(parseErr.Error()))
			continue
		}

		switch msg.Type {
		case MsgTypeAuth:
			h.handleAuth(ctx, c, msg, authTimer)
		case MsgTypeSubscribe:
			h.handleSubscribe(c, msg)
		case MsgTypeUnsubscribe:
			h.handleUnsubscribe(c, msg)
		case MsgTypeChannelSubscribe:
			h.handleChannelSubscribe(c, msg)
		case MsgTypeChannelUnsubscribe:
			h.handleChannelUnsubscribe(c, msg)
		case MsgTypeBroadcast:
			h.handleBroadcast(c, msg)
		case MsgTypePresenceTrack:
			h.handlePresenceTrack(c, msg)
		case MsgTypePresenceUntrack:
			h.handlePresenceUntrack(c, msg)
		case MsgTypePresenceSync:
			h.handlePresenceSync(c, msg)
		}
	}
}

// writeLoop writes outbound messages and sends pings.
func (h *Handler) writeLoop(c *Conn) {
	ticker := time.NewTicker(h.pingIntervalDuration())
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteJSON(msg); err != nil {
				h.logger.Warn("ws write error", "conn", c.id, "error", err)
				return
			}
			// Drain any queued messages.
			n := len(c.send)
			for i := 0; i < n; i++ {
				if err := c.ws.WriteJSON(<-c.send); err != nil {
					return
				}
			}

		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				h.recordHeartbeatFailure()
				return
			}

		case <-c.done:
			return
		}
	}
}

// handleAuth processes an auth message.
func (h *Handler) handleAuth(ctx context.Context, c *Conn, msg ClientMessage, authTimer *time.Timer) {
	if h.authValidator == nil {
		c.setAuth(nil)
		c.Send(replyOK(msg.Ref))
		return
	}

	if msg.Token == "" {
		c.Send(replyError(msg.Ref, "missing token"))
		return
	}

	claims, err := h.validateToken(ctx, msg.Token)
	if err != nil {
		c.Send(replyError(msg.Ref, "authentication failed"))
		return
	}

	c.setAuth(claims)
	if authTimer != nil {
		authTimer.Stop()
	}
	c.Send(replyOK(msg.Ref))
}

// handleSubscribe processes a subscribe message.
func (h *Handler) handleSubscribe(c *Conn, msg ClientMessage) {
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}

	previous := c.Subscriptions()
	c.Subscribe(msg.Tables)
	if h.OnSubscribe != nil {
		if err := h.OnSubscribe(c, msg.Tables, msg.Filter); err != nil {
			// Roll back only tables introduced by this request. Previously
			// subscribed tables must remain subscribed.
			c.Unsubscribe(newlySubscribedTables(previous, msg.Tables))
			c.Send(replyError(msg.Ref, err.Error()))
			return
		}
	}
	c.Send(replyOK(msg.Ref))
}

func newlySubscribedTables(previous map[string]bool, requested []string) []string {
	if len(requested) == 0 {
		return nil
	}
	added := make([]string, 0, len(requested))
	for _, table := range requested {
		if !previous[table] {
			added = append(added, table)
		}
	}
	return added
}

// handleUnsubscribe processes an unsubscribe message.
func (h *Handler) handleUnsubscribe(c *Conn, msg ClientMessage) {
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	c.Unsubscribe(msg.Tables)
	if h.OnUnsubscribe != nil {
		h.OnUnsubscribe(c, msg.Tables)
	}
	c.Send(replyOK(msg.Ref))
}

// handleChannelSubscribe processes a channel subscribe message, validating that broadcast is available and the client is authenticated, then subscribes the connection to the channel and sends the current presence state if presence tracking is enabled.
func (h *Handler) handleChannelSubscribe(c *Conn, msg ClientMessage) {
	if h.Broadcast == nil {
		c.Send(replyError(msg.Ref, "broadcast not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}

	c.SubscribeChannel(channel)
	h.Broadcast.Subscribe(channel, c)
	c.Send(replyOK(msg.Ref))
	if h.Presence != nil {
		h.sendPresenceSync(c, channel)
	}
}

// handleChannelUnsubscribe processes a channel unsubscribe message, validating that broadcast is available and the client is authenticated, then unsubscribes the connection from the channel.
func (h *Handler) handleChannelUnsubscribe(c *Conn, msg ClientMessage) {
	if h.Broadcast == nil {
		c.Send(replyError(msg.Ref, "broadcast not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}

	c.UnsubscribeChannel(channel)
	h.Broadcast.Unsubscribe(channel, c)
	c.Send(replyOK(msg.Ref))
}

// handleBroadcast processes a broadcast message, validating that broadcast is available, the client is authenticated, and the client is subscribed to the target channel, then relays the message to other clients in that channel.
func (h *Handler) handleBroadcast(c *Conn, msg ClientMessage) {
	if h.Broadcast == nil {
		c.Send(replyError(msg.Ref, "broadcast not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}
	if !c.HasChannel(channel) {
		c.Send(replyError(msg.Ref, "not subscribed to channel"))
		return
	}

	if err := h.Broadcast.Relay(channel, c, msg.Event, msg.Payload, msg.Self); err != nil {
		c.Send(replyError(msg.Ref, err.Error()))
		return
	}
	c.Send(replyOK(msg.Ref))
}

// handlePresenceTrack processes a presence track message, validating that presence is available, the client is authenticated, and the client is subscribed to the target channel, then updates the presence state and broadcasts the change to other clients in that channel.
func (h *Handler) handlePresenceTrack(c *Conn, msg ClientMessage) {
	if h.Presence == nil {
		c.Send(replyError(msg.Ref, "presence not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}
	if !c.HasChannel(channel) {
		c.Send(replyError(msg.Ref, "not subscribed to channel"))
		return
	}

	diff, err := h.Presence.Track(channel, c, msg.Presence)
	if err != nil {
		c.Send(replyError(msg.Ref, err.Error()))
		return
	}
	c.SetPresence(channel, msg.Presence)
	h.sendPresenceDiff(diff)
	c.Send(replyOK(msg.Ref))
}

// handlePresenceUntrack processes a presence untrack message, validating that presence is available, the client is authenticated, and the client is subscribed to the target channel, then removes the presence state and broadcasts the change to other clients in that channel.
func (h *Handler) handlePresenceUntrack(c *Conn, msg ClientMessage) {
	if h.Presence == nil {
		c.Send(replyError(msg.Ref, "presence not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}
	if !c.HasChannel(channel) {
		c.Send(replyError(msg.Ref, "not subscribed to channel"))
		return
	}

	diff := h.Presence.Untrack(channel, c)
	c.ClearPresence(channel)
	if diff.Action != "" {
		h.sendPresenceDiff(diff)
	}
	c.Send(replyOK(msg.Ref))
}

// handlePresenceSync processes a presence sync message, validating that presence is available, the client is authenticated, and the client is subscribed to the target channel, then sends the full presence state for that channel to the client.
func (h *Handler) handlePresenceSync(c *Conn, msg ClientMessage) {
	if h.Presence == nil {
		c.Send(replyError(msg.Ref, "presence not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}
	if !c.HasChannel(channel) {
		c.Send(replyError(msg.Ref, "not subscribed to channel"))
		return
	}

	h.sendPresenceSync(c, channel)
	c.Send(replyOK(msg.Ref))
}

func (h *Handler) sendPresenceSync(c *Conn, channel string) {
	if h.Presence == nil {
		return
	}
	h.Presence.RecordSync()
	c.Send(PresenceSyncMsg(channel, h.Presence.Sync(channel)))
}

// sendPresenceDiff sends a presence diff message to all connected clients that are subscribed to the channel, notifying them of presence state changes.
func (h *Handler) sendPresenceDiff(diff PresenceDiff) {
	if diff.Action == "" {
		return
	}
	if h.Presence != nil {
		h.Presence.RecordSync()
	}
	msg := PresenceDiffMsg(diff.Channel, diff.Action, diff.ConnID, diff.Presence)
	h.mu.Lock()
	conns := make([]*Conn, 0, len(h.conns))
	for _, conn := range h.conns {
		conns = append(conns, conn)
	}
	h.mu.Unlock()
	for _, conn := range conns {
		if conn.HasChannel(diff.Channel) {
			conn.Send(msg)
		}
	}
}

// validateToken validates a token string, auto-detecting JWT vs API key.
func (h *Handler) validateToken(ctx context.Context, token string) (*auth.Claims, error) {
	if auth.IsAPIKey(token) {
		return h.authValidator.ValidateAPIKey(ctx, token)
	}
	return h.authValidator.ValidateToken(token)
}

// trackConn adds a connection to the tracked set.
func (h *Handler) trackConn(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[c.id] = c
}

// removeConn removes a connection from the tracked set.
func (h *Handler) removeConn(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, c.id)
}

// Shutdown closes all active connections.
func (h *Handler) Shutdown() {
	h.mu.Lock()
	conns := make([]*Conn, 0, len(h.conns))
	for _, c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.Unlock()

	for _, c := range conns {
		c.Close(websocket.CloseGoingAway, "server shutdown")
	}
}

// ConnCount returns the number of active connections.
func (h *Handler) ConnCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.conns)
}
