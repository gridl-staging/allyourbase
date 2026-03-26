package ws

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

// dialTestServer creates a test server that upgrades to WS and returns
// the server-side *Conn plus a client-side *websocket.Conn for testing.
func dialTestServer(t *testing.T) (*Conn, *websocket.Conn, func()) {
	t.Helper()
	var serverConn *Conn
	ready := make(chan struct{})

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		serverConn = newConn("test-id", ws, slog.Default())
		close(ready)
		// Block until test closes the connection.
		<-serverConn.done
	}))

	clientWS, _, err := websocket.DefaultDialer.Dial("ws"+srv.URL[4:], nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}
	<-ready

	cleanup := func() {
		clientWS.Close()
		serverConn.Close(websocket.CloseNormalClosure, "")
		srv.Close()
	}
	return serverConn, clientWS, cleanup
}

func TestConn_SendNonBlocking(t *testing.T) {
	t.Parallel()
	sc, _, cleanup := dialTestServer(t)
	defer cleanup()

	// Fill the buffer.
	for i := 0; i < sendBufferSize; i++ {
		sc.Send(ServerMessage{Type: MsgTypeSystem, Message: "fill"})
	}

	// Next send should not block (dropped).
	sc.Send(ServerMessage{Type: MsgTypeSystem, Message: "overflow"})

	if len(sc.send) != sendBufferSize {
		t.Fatalf("expected buffer to stay at %d, got %d", sendBufferSize, len(sc.send))
	}
}

func TestConn_CloseIdempotent(t *testing.T) {
	t.Parallel()
	sc, _, cleanup := dialTestServer(t)
	defer cleanup()

	// Calling Close twice must not panic.
	sc.Close(websocket.CloseNormalClosure, "bye")
	sc.Close(websocket.CloseNormalClosure, "bye again")

	// done channel should be closed.
	select {
	case <-sc.done:
		// ok
	default:
		t.Fatal("expected done channel to be closed")
	}
}

func TestConn_SubscribeUnsubscribe(t *testing.T) {
	t.Parallel()
	sc, _, cleanup := dialTestServer(t)
	defer cleanup()

	sc.Subscribe([]string{"users", "logs"})
	subs := sc.Subscriptions()
	if len(subs) != 2 || !subs["users"] || !subs["logs"] {
		t.Fatalf("expected {users, logs}, got %v", subs)
	}

	sc.Unsubscribe([]string{"users"})
	subs = sc.Subscriptions()
	if len(subs) != 1 || !subs["logs"] {
		t.Fatalf("expected {logs}, got %v", subs)
	}

	sc.SubscribeChannel("room1")
	sc.SubscribeChannel("room2")
	if !sc.HasChannel("room1") {
		t.Fatal("expected to have room1 channel")
	}

	channels := sc.Channels()
	if len(channels) != 2 || !channels["room1"] || !channels["room2"] {
		t.Fatalf("unexpected channels: %v", channels)
	}

	sc.UnsubscribeChannel("room1")
	if sc.HasChannel("room1") {
		t.Fatal("expected room1 to be removed")
	}
}

func TestConn_AuthState(t *testing.T) {
	t.Parallel()
	sc, _, cleanup := dialTestServer(t)
	defer cleanup()

	if sc.Authenticated() {
		t.Fatal("expected unauthenticated initially")
	}
	if sc.Claims() != nil {
		t.Fatal("expected nil claims initially")
	}

	sc.setAuth(nil) // auth with nil claims (no-auth mode)
	if !sc.Authenticated() {
		t.Fatal("expected authenticated after setAuth")
	}
}

func TestConn_PresenceState(t *testing.T) {
	t.Parallel()
	sc, _, cleanup := dialTestServer(t)
	defer cleanup()

	sc.SetPresence("room1", map[string]any{"user": "alice"})
	presence := sc.Presence("room1")
	if presence["user"] != "alice" {
		t.Fatalf("unexpected presence payload: %+v", presence)
	}
	presence["user"] = "mutated"
	presence2 := sc.Presence("room1")
	if presence2["user"] != "alice" {
		t.Fatalf("presence should be copied, got %+v", presence2)
	}

	sc.ClearPresence("room1")
	if sc.Presence("room1") != nil {
		t.Fatal("expected nil presence after clear")
	}
}
