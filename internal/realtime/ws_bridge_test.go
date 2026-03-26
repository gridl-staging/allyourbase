package realtime_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/ws"
	"github.com/gorilla/websocket"
)

// wsConnect dials a test server and reads the initial "connected" message.
func wsConnect(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + url[len("http"):]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	testutil.NoError(t, err)

	// Read and discard the connected message.
	var msg ws.ServerMessage
	err = conn.ReadJSON(&msg)
	testutil.NoError(t, err)
	testutil.Equal(t, "connected", msg.Type)
	return conn
}

// wsSendJSON sends a JSON message to the WebSocket.
func wsSendJSON(t *testing.T, conn *websocket.Conn, msg any) {
	t.Helper()
	err := conn.WriteJSON(msg)
	testutil.NoError(t, err)
}

// wsReadReply reads a reply message and verifies it's "ok".
func wsReadReply(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	var msg ws.ServerMessage
	err := conn.ReadJSON(&msg)
	testutil.NoError(t, err)
	testutil.Equal(t, "reply", msg.Type)
	testutil.Equal(t, "ok", msg.Status)
}

// wsReadEvent reads an event message with a timeout.
func wsReadEvent(t *testing.T, conn *websocket.Conn, timeout time.Duration) ws.ServerMessage {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var msg ws.ServerMessage
	err := conn.ReadJSON(&msg)
	testutil.NoError(t, err)
	testutil.Equal(t, "event", msg.Type)
	return msg
}

// setupBridgeServer creates a Hub, WSBridge, ws.Handler, and httptest.Server.
func setupBridgeServer(t *testing.T) (*realtime.Hub, *httptest.Server) {
	t.Helper()
	hub := realtime.NewHub(testutil.DiscardLogger())
	wsHandler := ws.NewHandler(nil, testutil.DiscardLogger()) // no auth
	bridge := realtime.NewWSBridge(hub, nil, nil, testutil.DiscardLogger())
	bridge.SetupHandler(wsHandler)
	srv := httptest.NewServer(wsHandler)
	t.Cleanup(func() {
		wsHandler.Shutdown()
		hub.Close()
		srv.Close()
	})
	return hub, srv
}

func TestBridgeEndToEnd(t *testing.T) {
	t.Parallel()
	hub, srv := setupBridgeServer(t)

	conn := wsConnect(t, srv.URL)
	defer conn.Close()

	// Subscribe to "posts".
	wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
	wsReadReply(t, conn)

	// Publish an event.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1, "title": "Hello"}})

	// Read the event.
	msg := wsReadEvent(t, conn, time.Second)
	testutil.Equal(t, "create", msg.Action)
	testutil.Equal(t, "posts", msg.Table)
	testutil.Equal(t, "Hello", msg.Record["title"])
}

func TestBridgeMultiTableSubscribe(t *testing.T) {
	t.Parallel()
	hub, srv := setupBridgeServer(t)

	conn := wsConnect(t, srv.URL)
	defer conn.Close()

	// Subscribe to both tables.
	wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"posts", "comments"}})
	wsReadReply(t, conn)

	// Publish to posts.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})
	msg := wsReadEvent(t, conn, time.Second)
	testutil.Equal(t, "posts", msg.Table)

	// Publish to comments.
	hub.Publish(&realtime.Event{Action: "create", Table: "comments", Record: map[string]any{"id": 2}})
	msg = wsReadEvent(t, conn, time.Second)
	testutil.Equal(t, "comments", msg.Table)
}

func TestBridgeUnsubscribeStopsDelivery(t *testing.T) {
	t.Parallel()
	hub, srv := setupBridgeServer(t)

	conn := wsConnect(t, srv.URL)
	defer conn.Close()

	// Subscribe.
	wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
	wsReadReply(t, conn)

	// Unsubscribe.
	wsSendJSON(t, conn, map[string]any{"type": "unsubscribe", "tables": []string{"posts"}})
	wsReadReply(t, conn)

	// Publish — should NOT be delivered.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})

	_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	var msg ws.ServerMessage
	err := conn.ReadJSON(&msg)
	testutil.True(t, err != nil, "should timeout — no event expected after unsubscribe")
}

func TestBridgeDynamicResubscribe(t *testing.T) {
	t.Parallel()
	hub, srv := setupBridgeServer(t)

	conn := wsConnect(t, srv.URL)
	defer conn.Close()

	// Subscribe to posts first.
	wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
	wsReadReply(t, conn)

	// Subscribe to comments (additive).
	wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"comments"}})
	wsReadReply(t, conn)

	// Both should deliver.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})
	msg := wsReadEvent(t, conn, time.Second)
	testutil.Equal(t, "posts", msg.Table)

	hub.Publish(&realtime.Event{Action: "update", Table: "comments", Record: map[string]any{"id": 2}})
	msg = wsReadEvent(t, conn, time.Second)
	testutil.Equal(t, "comments", msg.Table)
}

func TestBridgeDisconnectCleansUpHub(t *testing.T) {
	t.Parallel()
	hub, srv := setupBridgeServer(t)

	conn := wsConnect(t, srv.URL)

	// Subscribe to register a hub client.
	wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
	wsReadReply(t, conn)

	// Hub should have a client.
	testutil.True(t, hub.ClientCount() > 0, "hub should have at least one client")

	// Disconnect.
	conn.Close()

	// Wait for cleanup.
	deadline := time.Now().Add(time.Second)
	for hub.ClientCount() > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	testutil.Equal(t, 0, hub.ClientCount())
}

func TestBridgeNoPoolAllEventsPass(t *testing.T) {
	t.Parallel()
	// Bridge with nil pool — all events should pass RLS filtering.
	hub, srv := setupBridgeServer(t) // already uses nil pool

	conn := wsConnect(t, srv.URL)
	defer conn.Close()

	wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
	wsReadReply(t, conn)

	for _, action := range []string{"create", "update", "delete"} {
		hub.Publish(&realtime.Event{Action: action, Table: "posts", Record: map[string]any{"id": 1}})
		msg := wsReadEvent(t, conn, time.Second)
		testutil.Equal(t, action, msg.Action)
	}
}

func TestBridgeMultipleClientsReceiveSameEvent(t *testing.T) {
	t.Parallel()
	hub, srv := setupBridgeServer(t)

	conn1 := wsConnect(t, srv.URL)
	defer conn1.Close()
	conn2 := wsConnect(t, srv.URL)
	defer conn2.Close()

	// Both subscribe to posts.
	wsSendJSON(t, conn1, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
	wsReadReply(t, conn1)
	wsSendJSON(t, conn2, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
	wsReadReply(t, conn2)

	// Publish one event.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})

	// Both should receive it.
	msg1 := wsReadEvent(t, conn1, time.Second)
	testutil.Equal(t, "create", msg1.Action)
	msg2 := wsReadEvent(t, conn2, time.Second)
	testutil.Equal(t, "create", msg2.Action)
}

func TestBridgeNotificationsTableEventForwarding(t *testing.T) {
	t.Parallel()
	hub, srv := setupBridgeServer(t)

	conn := wsConnect(t, srv.URL)
	defer conn.Close()

	wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"_ayb_notifications"}})
	wsReadReply(t, conn)

	hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "_ayb_notifications",
		Record: map[string]any{"id": "n1", "user_id": "u1", "title": "hello"},
	})

	msg := wsReadEvent(t, conn, time.Second)
	testutil.Equal(t, "_ayb_notifications", msg.Table)
	testutil.Equal(t, "create", msg.Action)
	testutil.Equal(t, "n1", msg.Record["id"])
}

func TestBridgeWSDoesNotInterfereWithSSE(t *testing.T) {
	t.Parallel()
	hub, srv := setupBridgeServer(t)

	// SSE client subscribes directly to Hub.
	sseClient := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(sseClient.ID)

	// WS client subscribes via bridge.
	conn := wsConnect(t, srv.URL)
	defer conn.Close()
	wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
	wsReadReply(t, conn)

	// Publish one event.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})

	// SSE client receives it.
	select {
	case event := <-sseClient.Events():
		testutil.Equal(t, "create", event.Action)
	case <-time.After(time.Second):
		t.Fatal("SSE client should receive event")
	}

	// WS client receives it.
	msg := wsReadEvent(t, conn, time.Second)
	testutil.Equal(t, "create", msg.Action)
}

func TestBridgeInvalidFilterRejected(t *testing.T) {
	t.Parallel()
	hub, srv := setupBridgeServer(t)

	conn := wsConnect(t, srv.URL)
	defer conn.Close()

	wsSendJSON(t, conn, map[string]any{
		"type":   "subscribe",
		"tables": []string{"posts"},
		"filter": "status=like.pending",
		"ref":    "sub-1",
	})

	var reply ws.ServerMessage
	err := conn.ReadJSON(&reply)
	testutil.NoError(t, err)
	testutil.Equal(t, "reply", reply.Type)
	testutil.Equal(t, "error", reply.Status)
	testutil.True(t, strings.Contains(reply.Message, "invalid filter"), "expected invalid filter message")

	// Subscription should not be active after callback validation failure.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})
	_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	err = conn.ReadJSON(&reply)
	testutil.True(t, err != nil, "expected no event after failed subscribe")
}
