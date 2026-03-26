package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/allyourbase/ayb/internal/auth"
)

// testHandler creates a ws.Handler with no auth (nil authSvc) for testing.
func testHandler(t *testing.T) (*Handler, *httptest.Server) {
	t.Helper()
	h := NewHandler(nil, slog.Default())
	srv := httptest.NewServer(h)
	return h, srv
}

// dialWS dials the test server and returns the client websocket.
func dialWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return ws
}

// readJSON reads a JSON message from the websocket into a ServerMessage.
func readJSON(t *testing.T, ws *websocket.Conn) ServerMessage {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg ServerMessage
	if err := ws.ReadJSON(&msg); err != nil {
		t.Fatalf("read: %v", err)
	}
	return msg
}

// writeJSON sends a JSON message to the websocket.
func writeJSON(t *testing.T, ws *websocket.Conn, msg any) {
	t.Helper()
	data, _ := json.Marshal(msg)
	if err := ws.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestHandler_ConnectAndReceiveConnectedMsg(t *testing.T) {
	t.Parallel()
	_, srv := testHandler(t)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	msg := readJSON(t, ws)
	if msg.Type != MsgTypeConnected {
		t.Fatalf("got type %q, want %q", msg.Type, MsgTypeConnected)
	}
	if msg.ClientID == "" {
		t.Fatal("expected non-empty client_id")
	}
}

func TestHandler_RejectsCrossOriginUpgrade(t *testing.T) {
	t.Parallel()
	_, srv := testHandler(t)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	header := http.Header{}
	header.Set("Origin", "https://evil.example")
	_, _, err := websocket.DefaultDialer.Dial(url, header)
	if err == nil {
		t.Fatal("expected cross-origin upgrade to be rejected")
	}
}

func TestHandler_AllowsSameOriginUpgrade(t *testing.T) {
	t.Parallel()
	_, srv := testHandler(t)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	httpURL := strings.TrimPrefix(srv.URL, "http://")
	header := http.Header{}
	header.Set("Origin", "http://"+httpURL)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial same-origin: %v", err)
	}
	defer ws.Close()

	msg := readJSON(t, ws)
	if msg.Type != MsgTypeConnected {
		t.Fatalf("got type %q, want %q", msg.Type, MsgTypeConnected)
	}
}

func TestHandler_SubscribeAndGetReply(t *testing.T) {
	t.Parallel()
	_, srv := testHandler(t)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	// Read connected message.
	readJSON(t, ws)

	// Subscribe.
	writeJSON(t, ws, ClientMessage{Type: MsgTypeSubscribe, Tables: []string{"users"}, Ref: "sub-1"})
	reply := readJSON(t, ws)
	if reply.Type != MsgTypeReply || reply.Status != "ok" || reply.Ref != "sub-1" {
		t.Fatalf("unexpected reply: %+v", reply)
	}
}

func TestHandler_UnsubscribeAndGetReply(t *testing.T) {
	t.Parallel()
	_, srv := testHandler(t)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	writeJSON(t, ws, ClientMessage{Type: MsgTypeSubscribe, Tables: []string{"users", "logs"}})
	readJSON(t, ws) // reply ok

	writeJSON(t, ws, ClientMessage{Type: MsgTypeUnsubscribe, Tables: []string{"users"}, Ref: "unsub-1"})
	reply := readJSON(t, ws)
	if reply.Type != MsgTypeReply || reply.Status != "ok" || reply.Ref != "unsub-1" {
		t.Fatalf("unexpected reply: %+v", reply)
	}
}

func TestHandler_UnknownMessageTypeReturnsError(t *testing.T) {
	t.Parallel()
	_, srv := testHandler(t)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	// Send raw unknown type.
	ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"bogus"}`))
	msg := readJSON(t, ws)
	if msg.Type != MsgTypeError {
		t.Fatalf("got type %q, want %q", msg.Type, MsgTypeError)
	}
}

func TestHandler_MalformedJSONReturnsError(t *testing.T) {
	t.Parallel()
	_, srv := testHandler(t)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	ws.WriteMessage(websocket.TextMessage, []byte(`not json`))
	msg := readJSON(t, ws)
	if msg.Type != MsgTypeError {
		t.Fatalf("got type %q, want %q", msg.Type, MsgTypeError)
	}
}

func TestHandler_OnConnectOnDisconnectCallbacks(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())

	var connected, disconnected atomic.Int32
	h.OnConnect = func(c *Conn) { connected.Add(1) }
	h.OnDisconnect = func(c *Conn) { disconnected.Add(1) }

	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	readJSON(t, ws) // connected
	ws.Close()

	// Wait a bit for disconnect callback.
	time.Sleep(100 * time.Millisecond)
	if connected.Load() != 1 {
		t.Fatalf("expected 1 connect callback, got %d", connected.Load())
	}
	if disconnected.Load() != 1 {
		t.Fatalf("expected 1 disconnect callback, got %d", disconnected.Load())
	}
}

func TestHandler_OnSubscribeCallback(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())

	var subscribedTables []string
	h.OnSubscribe = func(c *Conn, tables []string, filter string) error {
		subscribedTables = tables
		return nil
	}

	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected
	writeJSON(t, ws, ClientMessage{Type: MsgTypeSubscribe, Tables: []string{"orders", "items"}})
	readJSON(t, ws) // reply ok

	// Give callback time to fire.
	time.Sleep(50 * time.Millisecond)
	if len(subscribedTables) != 2 || subscribedTables[0] != "orders" {
		t.Fatalf("expected [orders items], got %v", subscribedTables)
	}
}

func TestHandler_OnSubscribeCallbackErrorRollsBackAndRepliesError(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())

	var conn *Conn
	h.OnConnect = func(c *Conn) { conn = c }
	h.OnSubscribe = func(c *Conn, tables []string, filter string) error {
		return errors.New("invalid filter")
	}

	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected
	writeJSON(t, ws, ClientMessage{Type: MsgTypeSubscribe, Tables: []string{"orders"}, Filter: "status=like.pending", Ref: "s1"})

	reply := readJSON(t, ws)
	if reply.Type != MsgTypeReply || reply.Status != "error" || reply.Ref != "s1" {
		t.Fatalf("unexpected reply: %+v", reply)
	}
	if !strings.Contains(reply.Message, "invalid filter") {
		t.Fatalf("expected invalid filter message, got %q", reply.Message)
	}
	if conn == nil {
		t.Fatal("expected connection to be captured")
	}
	if len(conn.Subscriptions()) != 0 {
		t.Fatalf("expected subscriptions to be rolled back, got %v", conn.Subscriptions())
	}
}

func TestHandler_SubscribeErrorPreservesExistingSubscriptions(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())

	var conn *Conn
	h.OnConnect = func(c *Conn) { conn = c }
	h.OnSubscribe = func(c *Conn, tables []string, filter string) error {
		if filter == "status=like.pending" {
			return errors.New("invalid filter")
		}
		return nil
	}

	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	// Initial valid subscribe.
	writeJSON(t, ws, ClientMessage{Type: MsgTypeSubscribe, Tables: []string{"orders"}, Ref: "s1"})
	reply := readJSON(t, ws)
	if reply.Status != "ok" {
		t.Fatalf("expected initial subscribe ok, got %+v", reply)
	}

	// Later invalid subscribe must not drop existing subscriptions.
	writeJSON(t, ws, ClientMessage{
		Type:   MsgTypeSubscribe,
		Tables: []string{"orders"},
		Filter: "status=like.pending",
		Ref:    "s2",
	})
	reply = readJSON(t, ws)
	if reply.Status != "error" {
		t.Fatalf("expected error for invalid subscribe, got %+v", reply)
	}
	if conn == nil {
		t.Fatal("expected connection to be captured")
	}
	subs := conn.Subscriptions()
	if !subs["orders"] || len(subs) != 1 {
		t.Fatalf("expected existing subscription to be preserved, got %v", subs)
	}
}

func TestHandler_SendEventToConn(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())

	var capturedConn *Conn
	h.OnConnect = func(c *Conn) { capturedConn = c }

	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	// Wait for OnConnect callback.
	time.Sleep(50 * time.Millisecond)

	// Push event from server side.
	capturedConn.Send(EventMsg("create", "users", map[string]any{"id": 1}))

	msg := readJSON(t, ws)
	if msg.Type != MsgTypeEvent || msg.Action != "create" || msg.Table != "users" {
		t.Fatalf("unexpected event: %+v", msg)
	}
}

func TestHandler_Shutdown(t *testing.T) {
	t.Parallel()
	h, srv := testHandler(t)
	defer srv.Close()

	ws1 := dialWS(t, srv)
	readJSON(t, ws1) // connected
	ws2 := dialWS(t, srv)
	readJSON(t, ws2) // connected

	if h.ConnCount() != 2 {
		t.Fatalf("expected 2 connections, got %d", h.ConnCount())
	}

	h.Shutdown()

	// Give goroutines time to clean up.
	time.Sleep(100 * time.Millisecond)

	if h.ConnCount() != 0 {
		t.Fatalf("expected 0 connections after shutdown, got %d", h.ConnCount())
	}

	ws1.Close()
	ws2.Close()
}

func TestHandler_AuthMessageNoAuthSvc(t *testing.T) {
	t.Parallel()
	_, srv := testHandler(t) // nil authSvc
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	writeJSON(t, ws, ClientMessage{Type: MsgTypeAuth, Token: "anything", Ref: "auth-1"})
	reply := readJSON(t, ws)
	if reply.Type != MsgTypeReply || reply.Status != "ok" || reply.Ref != "auth-1" {
		t.Fatalf("unexpected reply: %+v", reply)
	}
}

func TestHandler_ConnCountTracking(t *testing.T) {
	t.Parallel()
	h, srv := testHandler(t)
	defer srv.Close()

	if h.ConnCount() != 0 {
		t.Fatalf("expected 0 initial connections")
	}

	ws := dialWS(t, srv)
	readJSON(t, ws) // connected

	if h.ConnCount() != 1 {
		t.Fatalf("expected 1 connection, got %d", h.ConnCount())
	}

	ws.Close()
	time.Sleep(100 * time.Millisecond)

	if h.ConnCount() != 0 {
		t.Fatalf("expected 0 after close, got %d", h.ConnCount())
	}
}

// --- Mock TokenValidator for auth tests ---

type mockValidator struct {
	validToken string
}

func (m *mockValidator) ValidateToken(token string) (*auth.Claims, error) {
	if token == m.validToken {
		return &auth.Claims{}, nil
	}
	return nil, errors.New("invalid token")
}

func (m *mockValidator) ValidateAPIKey(_ context.Context, _ string) (*auth.Claims, error) {
	return nil, errors.New("api keys not supported in test")
}

// testHandlerWithAuth creates a handler with auth enabled and a short auth timeout.
func testHandlerWithAuth(t *testing.T, validToken string) (*Handler, *httptest.Server) {
	t.Helper()
	h := NewHandler(&mockValidator{validToken: validToken}, slog.Default())
	h.AuthTimeout = 200 * time.Millisecond // short for tests
	srv := httptest.NewServer(h)
	return h, srv
}

func TestHandler_AuthTimeout(t *testing.T) {
	t.Parallel()
	_, srv := testHandlerWithAuth(t, "valid-jwt")
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	// Should receive connected message.
	msg := readJSON(t, ws)
	if msg.Type != MsgTypeConnected {
		t.Fatalf("got type %q, want %q", msg.Type, MsgTypeConnected)
	}

	// Don't send auth — wait for timeout.
	// The server should close the connection with code 4401.
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := ws.ReadMessage()
	if err == nil {
		t.Fatal("expected read error after auth timeout")
	}

	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected CloseError, got %T: %v", err, err)
	}
	if closeErr.Code != 4401 {
		t.Fatalf("expected close code 4401, got %d", closeErr.Code)
	}
}

func TestHandler_AuthSuccessCancelsTimeout(t *testing.T) {
	t.Parallel()
	_, srv := testHandlerWithAuth(t, "valid-jwt")
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	// Authenticate before timeout.
	writeJSON(t, ws, ClientMessage{Type: MsgTypeAuth, Token: "valid-jwt", Ref: "a1"})
	reply := readJSON(t, ws)
	if reply.Status != "ok" {
		t.Fatalf("expected ok, got %+v", reply)
	}

	// Wait past the original auth timeout — connection should stay alive.
	time.Sleep(300 * time.Millisecond)

	// Should still be able to subscribe.
	writeJSON(t, ws, ClientMessage{Type: MsgTypeSubscribe, Tables: []string{"users"}, Ref: "s1"})
	subReply := readJSON(t, ws)
	if subReply.Status != "ok" {
		t.Fatalf("expected subscribe ok, got %+v", subReply)
	}
}

func TestHandler_AuthFailedReply(t *testing.T) {
	t.Parallel()
	_, srv := testHandlerWithAuth(t, "valid-jwt")
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	// Send wrong token.
	writeJSON(t, ws, ClientMessage{Type: MsgTypeAuth, Token: "bad-token", Ref: "a1"})
	reply := readJSON(t, ws)
	if reply.Status != "error" || reply.Ref != "a1" {
		t.Fatalf("expected error reply, got %+v", reply)
	}
}

func TestHandler_UnauthenticatedSubscribeRejected(t *testing.T) {
	t.Parallel()
	_, srv := testHandlerWithAuth(t, "valid-jwt")
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	// Try to subscribe without authenticating.
	writeJSON(t, ws, ClientMessage{Type: MsgTypeSubscribe, Tables: []string{"users"}, Ref: "s1"})
	reply := readJSON(t, ws)
	if reply.Status != "error" {
		t.Fatalf("expected error for unauthenticated subscribe, got %+v", reply)
	}
	if !strings.Contains(reply.Message, "authentication required") {
		t.Fatalf("expected 'authentication required', got %q", reply.Message)
	}
}

func TestHandler_UnauthenticatedUnsubscribeRejected(t *testing.T) {
	t.Parallel()
	_, srv := testHandlerWithAuth(t, "valid-jwt")
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	writeJSON(t, ws, ClientMessage{Type: MsgTypeUnsubscribe, Tables: []string{"users"}, Ref: "u1"})
	reply := readJSON(t, ws)
	if reply.Status != "error" {
		t.Fatalf("expected error for unauthenticated unsubscribe, got %+v", reply)
	}
}

func TestHandler_AuthThenSubscribe(t *testing.T) {
	t.Parallel()
	_, srv := testHandlerWithAuth(t, "valid-jwt")
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	// Authenticate first.
	writeJSON(t, ws, ClientMessage{Type: MsgTypeAuth, Token: "valid-jwt", Ref: "a1"})
	readJSON(t, ws) // reply ok

	// Now subscribe should work.
	writeJSON(t, ws, ClientMessage{Type: MsgTypeSubscribe, Tables: []string{"users"}, Ref: "s1"})
	reply := readJSON(t, ws)
	if reply.Status != "ok" {
		t.Fatalf("expected subscribe ok after auth, got %+v", reply)
	}
}

func TestHandler_UpgradeWithTokenHeader(t *testing.T) {
	t.Parallel()
	_, srv := testHandlerWithAuth(t, "valid-jwt")
	defer srv.Close()

	// Dial with Authorization header.
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	header := http.Header{"Authorization": []string{"Bearer valid-jwt"}}
	ws, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	msg := readJSON(t, ws)
	if msg.Type != MsgTypeConnected {
		t.Fatalf("got type %q, want connected", msg.Type)
	}

	// Should be able to subscribe immediately (authenticated at upgrade).
	writeJSON(t, ws, ClientMessage{Type: MsgTypeSubscribe, Tables: []string{"users"}, Ref: "s1"})
	reply := readJSON(t, ws)
	if reply.Status != "ok" {
		t.Fatalf("expected ok, got %+v", reply)
	}
}

func TestHandler_UpgradeWithTokenQueryParam(t *testing.T) {
	t.Parallel()
	_, srv := testHandlerWithAuth(t, "valid-jwt")
	defer srv.Close()

	// Dial with token as query param.
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "?token=valid-jwt"
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	readJSON(t, ws) // connected

	// Should be pre-authenticated.
	writeJSON(t, ws, ClientMessage{Type: MsgTypeSubscribe, Tables: []string{"users"}, Ref: "s1"})
	reply := readJSON(t, ws)
	if reply.Status != "ok" {
		t.Fatalf("expected ok, got %+v", reply)
	}
}

func TestHandler_UpgradeWithBadToken(t *testing.T) {
	t.Parallel()
	_, srv := testHandlerWithAuth(t, "valid-jwt")
	defer srv.Close()

	// Dial with bad token in header.
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	header := http.Header{"Authorization": []string{"Bearer bad-token"}}
	ws, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		// Connection should be rejected — this is expected.
		return
	}
	defer ws.Close()

	// If upgrade succeeded, connection should be closed immediately with 4401.
	ws.SetReadDeadline(time.Now().Add(time.Second))
	_, _, readErr := ws.ReadMessage()
	if readErr == nil {
		t.Fatal("expected connection to be closed")
	}
}

func TestHandler_Heartbeat(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.PingInterval = 100 * time.Millisecond // short for test
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()

	readJSON(t, ws) // connected

	// Track pings received.
	var pingCount atomic.Int32
	ws.SetPingHandler(func(msg string) error {
		pingCount.Add(1)
		// Send pong back (standard behavior).
		return ws.WriteControl(websocket.PongMessage, []byte(msg), time.Now().Add(time.Second))
	})

	// Read in background to keep the connection alive and allow ping handler to fire.
	go func() {
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// Wait enough for at least 2 pings.
	time.Sleep(350 * time.Millisecond)

	count := pingCount.Load()
	if count < 2 {
		t.Fatalf("expected at least 2 pings, got %d", count)
	}
}

func assertNoWSMessage(t *testing.T, ws *websocket.Conn) {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	var msg ServerMessage
	err := ws.ReadJSON(&msg)
	if err == nil {
		t.Fatalf("unexpected message: %+v", msg)
	}
	closeErr, ok := err.(*websocket.CloseError)
	if ok {
		t.Fatalf("unexpected close error: %v", closeErr)
	}
}

func TestHandler_ChannelSubscribeAndBroadcastEndToEnd(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws1 := dialWS(t, srv)
	defer ws1.Close()
	ws2 := dialWS(t, srv)
	defer ws2.Close()

	readJSON(t, ws1)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "c1-sub"})
	reply1 := readJSON(t, ws1)
	if reply1.Status != "ok" {
		t.Fatalf("subscribe reply1: %+v", reply1)
	}
	writeJSON(t, ws2, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "c2-sub"})
	reply2 := readJSON(t, ws2)
	if reply2.Status != "ok" {
		t.Fatalf("subscribe reply2: %+v", reply2)
	}

	payload := map[string]any{"x": float64(1), "y": "north"}
	writeJSON(t, ws1, ClientMessage{Type: MsgTypeBroadcast, Channel: "room1", Event: "move", Payload: payload, Ref: "b1"})
	broadcast := readJSON(t, ws2)
	if broadcast.Type != MsgTypeBroadcast || broadcast.Channel != "room1" || broadcast.Event != "move" {
		t.Fatalf("unexpected broadcast: %+v", broadcast)
	}
	if broadcast.Payload["y"] != "north" {
		t.Fatalf("unexpected payload: %+v", broadcast.Payload)
	}
	reply := readJSON(t, ws1)
	if reply.Type != MsgTypeReply || reply.Status != "ok" || reply.Ref != "b1" {
		t.Fatalf("unexpected sender reply: %+v", reply)
	}
}

func TestHandler_BroadcastSelfFlag(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws1 := dialWS(t, srv)
	defer ws1.Close()
	ws2 := dialWS(t, srv)
	defer ws2.Close()
	readJSON(t, ws1)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s1"})
	readJSON(t, ws1)
	writeJSON(t, ws2, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s2"})
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeBroadcast, Channel: "room1", Event: "move", Payload: map[string]any{"x": float64(2)}, Self: true, Ref: "b1"})
	first := readJSON(t, ws1)
	second := readJSON(t, ws1)
	if first.Type != MsgTypeBroadcast && second.Type != MsgTypeBroadcast {
		t.Fatalf("sender did not receive self broadcast: first=%+v second=%+v", first, second)
	}
	if first.Type != MsgTypeReply && second.Type != MsgTypeReply {
		t.Fatalf("sender did not receive reply: first=%+v second=%+v", first, second)
	}
	other := readJSON(t, ws2)
	if other.Type != MsgTypeBroadcast {
		t.Fatalf("receiver expected broadcast, got %+v", other)
	}
}

func TestHandler_BroadcastSelfFalseDefault(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws1 := dialWS(t, srv)
	defer ws1.Close()
	readJSON(t, ws1)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s1"})
	readJSON(t, ws1)
	writeJSON(t, ws1, ClientMessage{Type: MsgTypeBroadcast, Channel: "room1", Event: "move", Payload: map[string]any{"x": float64(3)}, Ref: "b1"})
	reply := readJSON(t, ws1)
	if reply.Type != MsgTypeReply || reply.Status != "ok" {
		t.Fatalf("unexpected reply: %+v", reply)
	}
	assertNoWSMessage(t, ws1)
}

func TestHandler_ChannelUnsubscribeStopsBroadcastDelivery(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws1 := dialWS(t, srv)
	defer ws1.Close()
	ws2 := dialWS(t, srv)
	defer ws2.Close()
	readJSON(t, ws1)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s1"})
	readJSON(t, ws1)
	writeJSON(t, ws2, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s2"})
	readJSON(t, ws2)

	writeJSON(t, ws2, ClientMessage{Type: MsgTypeChannelUnsubscribe, Channel: "room1", Ref: "u1"})
	uReply := readJSON(t, ws2)
	if uReply.Status != "ok" {
		t.Fatalf("unexpected unsubscribe reply: %+v", uReply)
	}

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeBroadcast, Channel: "room1", Event: "move", Payload: map[string]any{"x": float64(4)}, Ref: "b1"})
	readJSON(t, ws1)
	assertNoWSMessage(t, ws2)
}

func TestHandler_BroadcastRequiresChannelSubscription(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()
	readJSON(t, ws)

	writeJSON(t, ws, ClientMessage{Type: MsgTypeBroadcast, Channel: "room1", Event: "move", Payload: map[string]any{"x": float64(5)}, Ref: "b1"})
	reply := readJSON(t, ws)
	if reply.Status != "error" {
		t.Fatalf("expected error reply, got %+v", reply)
	}
}

func TestHandler_BroadcastRateLimitExceeded(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	h.Broadcast.RateLimit = 2
	h.Broadcast.RateWindow = 2 * time.Second
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws1 := dialWS(t, srv)
	defer ws1.Close()
	ws2 := dialWS(t, srv)
	defer ws2.Close()
	readJSON(t, ws1)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s1"})
	readJSON(t, ws1)
	writeJSON(t, ws2, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s2"})
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeBroadcast, Channel: "room1", Event: "move", Payload: map[string]any{"i": float64(1)}, Ref: "b1"})
	readJSON(t, ws2)
	r1 := readJSON(t, ws1)
	if r1.Status != "ok" {
		t.Fatalf("expected first broadcast ok, got %+v", r1)
	}

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeBroadcast, Channel: "room1", Event: "move", Payload: map[string]any{"i": float64(2)}, Ref: "b2"})
	readJSON(t, ws2)
	r2 := readJSON(t, ws1)
	if r2.Status != "ok" {
		t.Fatalf("expected second broadcast ok, got %+v", r2)
	}

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeBroadcast, Channel: "room1", Event: "move", Payload: map[string]any{"i": float64(3)}, Ref: "b3"})
	r3 := readJSON(t, ws1)
	if r3.Status != "error" {
		t.Fatalf("expected rate limit error on third broadcast, got %+v", r3)
	}
}

func TestHandler_DisconnectCleansUpChannelSubscriptions(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	connected := readJSON(t, ws)
	connID := connected.ClientID

	writeJSON(t, ws, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s1"})
	reply := readJSON(t, ws)
	if reply.Status != "ok" {
		t.Fatalf("unexpected subscribe reply: %+v", reply)
	}

	ws.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.Broadcast.mu.RLock()
		members := h.Broadcast.channels["room1"]
		_, exists := members[connID]
		h.Broadcast.mu.RUnlock()
		if !exists {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("connection %s still subscribed after disconnect", connID)
}

func TestHandler_PresenceTrackAndSync(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	h.Presence = NewPresenceHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws1 := dialWS(t, srv)
	defer ws1.Close()
	ws2 := dialWS(t, srv)
	defer ws2.Close()
	readJSON(t, ws1)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s1"})
	readJSON(t, ws1)
	readJSON(t, ws1)
	writeJSON(t, ws2, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s2"})
	readJSON(t, ws2)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypePresenceTrack, Channel: "room1", Presence: map[string]any{"user": "alice"}, Ref: "p1"})
	join := readJSON(t, ws2)
	if join.Type != MsgTypePresence || join.PresenceAction != PresenceActionJoin {
		t.Fatalf("expected join presence message, got %+v", join)
	}
	first := readJSON(t, ws1)
	second := readJSON(t, ws1)
	if first.Type != MsgTypeReply && second.Type != MsgTypeReply {
		t.Fatalf("expected track reply, got first=%+v second=%+v", first, second)
	}

	writeJSON(t, ws2, ClientMessage{Type: MsgTypePresenceSync, Channel: "room1", Ref: "sync1"})
	syncMsg := readJSON(t, ws2)
	if syncMsg.Type != MsgTypePresence || syncMsg.PresenceAction != PresenceActionSync {
		t.Fatalf("expected sync presence message, got %+v", syncMsg)
	}
	if syncMsg.Presences["ws-1"]["user"] != "alice" {
		t.Fatalf("unexpected sync payload: %+v", syncMsg.Presences)
	}
	readJSON(t, ws2) // sync reply
}

func TestHandler_PresenceSyncCounterIncludesStateAndDiffMessages(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	h.Presence = NewPresenceHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()
	readJSON(t, ws) // connected

	writeJSON(t, ws, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s1"})
	readJSON(t, ws) // subscribe reply
	readJSON(t, ws) // auto presence_state

	writeJSON(t, ws, ClientMessage{Type: MsgTypePresenceTrack, Channel: "room1", Presence: map[string]any{"user": "alice"}, Ref: "p1"})
	first := readJSON(t, ws)
	second := readJSON(t, ws)
	if first.Type != MsgTypePresence && second.Type != MsgTypePresence {
		t.Fatalf("expected presence diff from track, got first=%+v second=%+v", first, second)
	}
	if first.Type != MsgTypeReply && second.Type != MsgTypeReply {
		t.Fatalf("expected presence track reply, got first=%+v second=%+v", first, second)
	}

	writeJSON(t, ws, ClientMessage{Type: MsgTypePresenceSync, Channel: "room1", Ref: "sync1"})
	syncMsg := readJSON(t, ws)
	reply := readJSON(t, ws)
	if syncMsg.Type != MsgTypePresence || syncMsg.PresenceAction != PresenceActionSync {
		t.Fatalf("expected sync presence message, got %+v", syncMsg)
	}
	if reply.Type != MsgTypeReply || reply.Status != "ok" {
		t.Fatalf("expected sync reply, got %+v", reply)
	}

	if got := h.Presence.SyncedCount(); got != 3 {
		t.Fatalf("expected SyncedCount=3 (state+diff+sync), got %d", got)
	}
}

func TestHandler_PresenceUntrack(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	h.Presence = NewPresenceHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws1 := dialWS(t, srv)
	defer ws1.Close()
	ws2 := dialWS(t, srv)
	defer ws2.Close()
	readJSON(t, ws1)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1"})
	readJSON(t, ws1)
	readJSON(t, ws1)
	writeJSON(t, ws2, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1"})
	readJSON(t, ws2)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypePresenceTrack, Channel: "room1", Presence: map[string]any{"user": "alice"}})
	readJSON(t, ws2)
	readJSON(t, ws1)
	readJSON(t, ws1)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypePresenceUntrack, Channel: "room1", Ref: "u1"})
	leave := readJSON(t, ws2)
	if leave.Type != MsgTypePresence || leave.PresenceAction != PresenceActionLeave {
		t.Fatalf("expected leave presence message, got %+v", leave)
	}
	first := readJSON(t, ws1)
	second := readJSON(t, ws1)
	var reply ServerMessage
	if first.Type == MsgTypeReply {
		reply = first
	} else if second.Type == MsgTypeReply {
		reply = second
	}
	if reply.Status != "ok" || reply.Ref != "u1" {
		t.Fatalf("expected untrack reply ok, got first=%+v second=%+v", first, second)
	}
}

func TestHandler_PresenceDisconnectCleanup(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	h.Presence = NewPresenceHub(slog.Default(), PresenceHubOptions{LeaveTimeout: 30 * time.Millisecond})
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws1 := dialWS(t, srv)
	defer ws1.Close()
	ws2 := dialWS(t, srv)
	defer ws2.Close()
	readJSON(t, ws1)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1"})
	readJSON(t, ws1)
	readJSON(t, ws1)
	writeJSON(t, ws2, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1"})
	readJSON(t, ws2)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypePresenceTrack, Channel: "room1", Presence: map[string]any{"user": "alice"}})
	readJSON(t, ws2)
	readJSON(t, ws1)
	readJSON(t, ws1)

	ws1.Close()

	leaveCh := make(chan ServerMessage, 1)
	go func() {
		leaveCh <- readJSON(t, ws2)
	}()

	select {
	case leave := <-leaveCh:
		if leave.Type != MsgTypePresence || leave.PresenceAction != PresenceActionLeave {
			t.Fatalf("expected leave on disconnect, got %+v", leave)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected leave on disconnect after leave timeout")
	}
}

func TestHandler_ChannelSubscribeSendsPresenceStateForEmptyChannel(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	h.Presence = NewPresenceHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()
	readJSON(t, ws)

	writeJSON(t, ws, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s1"})
	reply := readJSON(t, ws)
	if reply.Status != "ok" {
		t.Fatalf("subscribe reply: %+v", reply)
	}
	state := readJSON(t, ws)
	if state.Type != MsgTypePresence || state.PresenceAction != PresenceActionSync {
		t.Fatalf("expected presence state on subscribe, got %+v", state)
	}
	if len(state.Presences) != 0 {
		t.Fatalf("expected empty presence state, got %+v", state.Presences)
	}
}

func TestHandler_ChannelSubscribeSendsPresenceStateForPopulatedChannel(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	h.Presence = NewPresenceHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws1 := dialWS(t, srv)
	defer ws1.Close()
	ws2 := dialWS(t, srv)
	defer ws2.Close()
	readJSON(t, ws1)
	readJSON(t, ws2)

	writeJSON(t, ws1, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s1"})
	readJSON(t, ws1)
	readJSON(t, ws1)
	writeJSON(t, ws1, ClientMessage{Type: MsgTypePresenceTrack, Channel: "room1", Presence: map[string]any{"user": "alice"}, Ref: "p1"})
	readJSON(t, ws1) // join diff
	readJSON(t, ws1) // reply

	writeJSON(t, ws2, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1", Ref: "s2"})
	reply := readJSON(t, ws2)
	if reply.Status != "ok" {
		t.Fatalf("subscribe reply: %+v", reply)
	}
	state := readJSON(t, ws2)
	if state.Type != MsgTypePresence || state.PresenceAction != PresenceActionSync {
		t.Fatalf("expected presence state on subscribe, got %+v", state)
	}
	if state.Presences["ws-1"]["user"] != "alice" {
		t.Fatalf("expected joining client to receive full presence state, got %+v", state.Presences)
	}
}

func TestHandler_PresenceRequiresChannelSubscription(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	h.Presence = NewPresenceHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()
	readJSON(t, ws)

	writeJSON(t, ws, ClientMessage{Type: MsgTypePresenceTrack, Channel: "room1", Presence: map[string]any{"user": "alice"}, Ref: "p1"})
	reply := readJSON(t, ws)
	if reply.Status != "error" {
		t.Fatalf("expected error when not subscribed, got %+v", reply)
	}

	writeJSON(t, ws, ClientMessage{Type: MsgTypePresenceSync, Channel: "room1", Ref: "sync1"})
	reply = readJSON(t, ws)
	if reply.Status != "error" {
		t.Fatalf("expected sync error when not subscribed, got %+v", reply)
	}
}

func TestHandler_PresenceAuthRequired(t *testing.T) {
	t.Parallel()
	h := NewHandler(&mockValidator{validToken: "valid-jwt"}, slog.Default())
	h.Presence = NewPresenceHub(slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	h.AuthTimeout = 500 * time.Millisecond
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()
	readJSON(t, ws)

	writeJSON(t, ws, ClientMessage{Type: MsgTypePresenceTrack, Channel: "room1", Presence: map[string]any{"user": "alice"}, Ref: "p1"})
	reply := readJSON(t, ws)
	if reply.Status != "error" {
		t.Fatalf("expected auth error, got %+v", reply)
	}
}

func TestHandler_PresenceUnavailable(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	srv := httptest.NewServer(h)
	defer srv.Close()

	ws := dialWS(t, srv)
	defer ws.Close()
	readJSON(t, ws)

	writeJSON(t, ws, ClientMessage{Type: MsgTypeChannelSubscribe, Channel: "room1"})
	readJSON(t, ws)

	writeJSON(t, ws, ClientMessage{Type: MsgTypePresenceTrack, Channel: "room1", Presence: map[string]any{"user": "alice"}, Ref: "p1"})
	reply := readJSON(t, ws)
	if reply.Status != "error" || !strings.Contains(reply.Message, "presence not available") {
		t.Fatalf("expected presence unavailable error, got %+v", reply)
	}
}
