// Package graphql implements WebSocket handlers for GraphQL subscriptions following the graphql-transport-ws protocol.
package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
)

const (
	gqlwsConnectionInit = "connection_init"
	gqlwsConnectionAck  = "connection_ack"
	gqlwsSubscribe      = "subscribe"
	gqlwsNext           = "next"
	gqlwsError          = "error"
	gqlwsComplete       = "complete"
	gqlwsPing           = "ping"
	gqlwsPong           = "pong"
	gqlwsSubprotocol    = "graphql-transport-ws"
)

const (
	closeInternalError       = 4400
	closeUnauthorized        = 4401
	closeInitTimeout         = 4408
	closeSubscriberExists    = 4409
	closeTooManyInitRequests = 4429
)

const (
	gqlwsWriteWait           = 10 * time.Second
	gqlwsDefaultInitTimeout  = 10 * time.Second
	gqlwsDefaultPingInterval = 25 * time.Second
	gqlwsMaxMessageSize      = 1 << 20
	gqlwsSendBufferSize      = 256
)

type gqlwsMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type gqlwsConnectionInitPayload struct {
	Authorization string `json:"Authorization,omitempty"`
	Token         string `json:"token,omitempty"`
}

type gqlwsSubscribePayload struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
	OperationName string                 `json:"operationName,omitempty"`
}

type gqlwsAuthValidator interface {
	ValidateToken(token string) (*auth.Claims, error)
	ValidateAPIKey(ctx context.Context, token string) (*auth.Claims, error)
}

type GQLWSHandler struct {
	pool          *pgxpool.Pool
	cache         func() *schema.SchemaCache
	authValidator gqlwsAuthValidator
	logger        *slog.Logger
	upgrader      websocket.Upgrader
	nextID        atomic.Uint64

	InitTimeout  time.Duration
	PingInterval time.Duration

	OnSubscribe  func(ctx context.Context, conn *GQLWSConn, id string, payload gqlwsSubscribePayload)
	OnComplete   func(conn *GQLWSConn, id string)
	OnDisconnect func(conn *GQLWSConn)
}

type GQLWSConn struct {
	id          string
	ws          *websocket.Conn
	claims      *auth.Claims
	initialized bool
	mu          sync.Mutex
	subs        map[string]bool
	send        chan []byte
	done        chan struct{}
	once        sync.Once
	logger      *slog.Logger
}

func NewGQLWSHandler(pool *pgxpool.Pool, cache func() *schema.SchemaCache, logger *slog.Logger) *GQLWSHandler {
	return &GQLWSHandler{
		pool:   pool,
		cache:  cache,
		logger: logger,
		upgrader: websocket.Upgrader{
			CheckOrigin:  httputil.CheckWebSocketOrigin,
			Subprotocols: []string{gqlwsSubprotocol},
		},
	}
}

func (h *GQLWSHandler) SetAuthValidator(v gqlwsAuthValidator) {
	h.authValidator = v
}

func (h *GQLWSHandler) initTimeoutDuration() time.Duration {
	if h.InitTimeout > 0 {
		return h.InitTimeout
	}
	return gqlwsDefaultInitTimeout
}

func (h *GQLWSHandler) pingIntervalDuration() time.Duration {
	if h.PingInterval > 0 {
		return h.PingInterval
	}
	return gqlwsDefaultPingInterval
}

// ServeHTTP upgrades an HTTP request to a WebSocket connection for GraphQL subscriptions, validates optional bearer token authentication, and manages the connection lifecycle through read and write loops.
func (h *GQLWSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wsConn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("graphql-ws upgrade failed", "error", err)
		}
		return
	}

	conn := newGQLWSConn(fmt.Sprintf("gqlws-%d", h.nextID.Add(1)), wsConn, h.logger)
	if wsConn.Subprotocol() != gqlwsSubprotocol {
		conn.Close(closeInternalError, "subprotocol graphql-transport-ws required")
		return
	}

	if h.authValidator != nil {
		if token, ok := httputil.ExtractBearerToken(r); ok {
			claims, authErr := h.validateToken(r.Context(), token)
			if authErr != nil {
				conn.Close(closeUnauthorized, "authentication failed")
				return
			}
			conn.setClaims(claims)
		}
	}

	go h.writeLoop(conn)
	h.readLoop(r.Context(), conn)

	if h.OnDisconnect != nil {
		h.OnDisconnect(conn)
	}
	conn.Close(websocket.CloseNormalClosure, "")
}

func newGQLWSConn(id string, wsConn *websocket.Conn, logger *slog.Logger) *GQLWSConn {
	return &GQLWSConn{
		id:     id,
		ws:     wsConn,
		subs:   make(map[string]bool),
		send:   make(chan []byte, gqlwsSendBufferSize),
		done:   make(chan struct{}),
		logger: logger,
	}
}

func (c *GQLWSConn) ID() string {
	return c.id
}

func (c *GQLWSConn) Claims() *auth.Claims {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.claims
}

func (c *GQLWSConn) setClaims(claims *auth.Claims) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.claims = claims
}

func (c *GQLWSConn) Initialized() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.initialized
}

func (c *GQLWSConn) setInitialized(initialized bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initialized = initialized
}

func (c *GQLWSConn) hasSubscription(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.subs[id]
}

func (c *GQLWSConn) addSubscription(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subs[id] = true
}

func (c *GQLWSConn) removeSubscription(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subs, id)
}

// SendMessage enqueues a message for delivery to the client; drops the message with a warning if the send buffer is full, or silently if the connection is closed.
func (c *GQLWSConn) SendMessage(msg gqlwsMessage) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}

	select {
	case <-c.done:
		return
	case c.send <- b:
	default:
		if c.logger != nil {
			c.logger.Warn("dropping graphql-ws message, send buffer full", "conn", c.id, "type", msg.Type)
		}
	}
}

func (c *GQLWSConn) SendNext(id string, data interface{}) {
	payload, err := json.Marshal(map[string]interface{}{"data": data})
	if err != nil {
		return
	}
	c.SendMessage(gqlwsMessage{ID: id, Type: gqlwsNext, Payload: payload})
}

func (c *GQLWSConn) SendError(id string, errors interface{}) {
	payload, err := json.Marshal(map[string]interface{}{"errors": errors})
	if err != nil {
		return
	}
	c.SendMessage(gqlwsMessage{ID: id, Type: gqlwsError, Payload: payload})
	c.SendComplete(id)
}

func (c *GQLWSConn) SendComplete(id string) {
	c.SendMessage(gqlwsMessage{ID: id, Type: gqlwsComplete})
}

func (c *GQLWSConn) Close(code int, reason string) {
	c.once.Do(func() {
		close(c.done)
		_ = c.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(gqlwsWriteWait))
		_ = c.ws.Close()
	})
}

// writeLoop continuously sends pending messages and periodic pings to the client in a separate goroutine; closes the connection on write error.
func (h *GQLWSHandler) writeLoop(conn *GQLWSConn) {
	pingTicker := time.NewTicker(h.pingIntervalDuration())
	defer pingTicker.Stop()

	for {
		select {
		case <-conn.done:
			return
		case msg := <-conn.send:
			_ = conn.ws.SetWriteDeadline(time.Now().Add(gqlwsWriteWait))
			if err := conn.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				conn.Close(closeInternalError, "write error")
				return
			}
		case <-pingTicker.C:
			conn.SendMessage(gqlwsMessage{Type: gqlwsPing})
		}
	}
}

// readLoop continuously reads and processes incoming WebSocket messages; requires connection initialization via connection_init within a timeout period before accepting other message types.
func (h *GQLWSHandler) readLoop(ctx context.Context, conn *GQLWSConn) {
	conn.ws.SetReadLimit(gqlwsMaxMessageSize)
	_ = conn.ws.SetReadDeadline(time.Now().Add(h.initTimeoutDuration()))

	for {
		select {
		case <-ctx.Done():
			return
		case <-conn.done:
			return
		default:
		}

		_, data, err := conn.ws.ReadMessage()
		if err != nil {
			if !conn.Initialized() {
				if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
					conn.Close(closeInitTimeout, "connection_init timeout")
					return
				}
			}
			return
		}

		var msg gqlwsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			conn.Close(closeInternalError, "invalid graphql-ws message")
			return
		}

		if !conn.Initialized() && msg.Type != gqlwsConnectionInit {
			conn.Close(closeUnauthorized, "connection not initialized")
			return
		}

		switch msg.Type {
		case gqlwsConnectionInit:
			if conn.Initialized() {
				conn.Close(closeTooManyInitRequests, "too many connection_init requests")
				return
			}
			if err := h.handleConnectionInit(ctx, conn, msg); err != nil {
				return
			}
			_ = conn.ws.SetReadDeadline(time.Time{})
		case gqlwsSubscribe:
			h.handleSubscribe(ctx, conn, msg)
		case gqlwsComplete:
			h.handleComplete(conn, msg)
		case gqlwsPing:
			conn.SendMessage(gqlwsMessage{Type: gqlwsPong, Payload: msg.Payload})
		case gqlwsPong:
			// client pong, no-op
		default:
			conn.Close(closeInternalError, "unknown graphql-ws message type")
			return
		}
	}
}

// handleConnectionInit processes the connection_init message, extracting and validating authentication credentials from the payload or Authorization header; marks the connection as initialized and sends a connection_ack response.
func (h *GQLWSHandler) handleConnectionInit(ctx context.Context, conn *GQLWSConn, msg gqlwsMessage) error {
	payload := gqlwsConnectionInitPayload{}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			conn.Close(closeInternalError, "invalid connection_init payload")
			return err
		}
	}

	if h.authValidator != nil {
		token := strings.TrimSpace(payload.Token)
		authHeader := strings.TrimSpace(payload.Authorization)
		if authHeader != "" {
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				token = strings.TrimSpace(authHeader[7:])
			} else {
				token = authHeader
			}
		}

		if token != "" {
			claims, err := h.validateToken(ctx, token)
			if err != nil {
				conn.Close(closeUnauthorized, "authentication failed")
				return err
			}
			conn.setClaims(claims)
		} else if conn.Claims() == nil {
			conn.Close(closeUnauthorized, "authentication required")
			return errors.New("missing authentication")
		}
	}

	conn.setInitialized(true)
	conn.SendMessage(gqlwsMessage{Type: gqlwsConnectionAck})
	return nil
}

// handleSubscribe processes incoming subscription requests, validating the subscription ID and operation type; invokes the OnSubscribe callback to establish the subscription, or sends an error if the callback is not configured.
func (h *GQLWSHandler) handleSubscribe(ctx context.Context, conn *GQLWSConn, msg gqlwsMessage) {
	if msg.ID == "" {
		conn.SendError("", []map[string]string{{"message": "subscription id is required"}})
		return
	}
	if conn.hasSubscription(msg.ID) {
		conn.Close(closeSubscriberExists, "subscriber already exists")
		return
	}

	payload := gqlwsSubscribePayload{}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		conn.SendError(msg.ID, []map[string]string{{"message": "invalid subscribe payload"}})
		return
	}

	if !isSubscriptionOperation(payload.Query, payload.OperationName) {
		conn.SendError(msg.ID, []map[string]string{{"message": "operation must be a subscription"}})
		return
	}

	conn.addSubscription(msg.ID)
	if h.OnSubscribe != nil {
		h.OnSubscribe(ctx, conn, msg.ID, payload)
		return
	}

	conn.removeSubscription(msg.ID)
	conn.SendError(msg.ID, []map[string]string{{"message": "subscriptions are not wired"}})
}

func (h *GQLWSHandler) handleComplete(conn *GQLWSConn, msg gqlwsMessage) {
	conn.removeSubscription(msg.ID)
	if h.OnComplete != nil {
		h.OnComplete(conn, msg.ID)
	}
}

func (h *GQLWSHandler) validateToken(ctx context.Context, token string) (*auth.Claims, error) {
	if h.authValidator == nil {
		return nil, nil
	}
	if auth.IsAPIKey(token) {
		return h.authValidator.ValidateAPIKey(ctx, token)
	}
	return h.authValidator.ValidateToken(token)
}

func isSubscriptionOperation(query, operationName string) bool {
	doc, err := parser.Parse(parser.ParseParams{Source: query})
	if err != nil {
		return false
	}
	selected := selectedOperationDefinition(doc, operationName)
	return selected != nil && selected.Operation == ast.OperationTypeSubscription
}
