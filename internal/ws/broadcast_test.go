package ws

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func newBroadcastTestConn(id string) *Conn {
	return &Conn{
		id:            id,
		logger:        slog.Default(),
		subscriptions: make(map[string]bool),
		channels:      make(map[string]bool),
		send:          make(chan ServerMessage, sendBufferSize),
		done:          make(chan struct{}),
	}
}

func mustReadBroadcastMessage(t *testing.T, c *Conn) ServerMessage {
	t.Helper()
	select {
	case msg := <-c.send:
		return msg
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for message")
		return ServerMessage{}
	}
}

func assertNoQueuedBroadcastMessage(t *testing.T, c *Conn) {
	t.Helper()
	select {
	case msg := <-c.send:
		t.Fatalf("unexpected message queued: %+v", msg)
	default:
	}
}

func TestBroadcastHub_SubscribeAndRelay(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default())
	sender := newBroadcastTestConn("c1")
	receiver := newBroadcastTestConn("c2")
	hub.Subscribe("room1", sender)
	hub.Subscribe("room1", receiver)

	err := hub.Relay("room1", sender, "move", map[string]any{"x": 1}, false)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}

	msg := mustReadBroadcastMessage(t, receiver)
	if msg.Type != MsgTypeBroadcast || msg.Channel != "room1" || msg.Event != "move" {
		t.Fatalf("unexpected relay message: %+v", msg)
	}
	assertNoQueuedBroadcastMessage(t, sender)
	if got := hub.MessagesSent(); got != 1 {
		t.Fatalf("expected MessagesSent=1, got %d", got)
	}
}

func TestBroadcastHub_RelayWithSelf(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default())
	sender := newBroadcastTestConn("c1")
	receiver := newBroadcastTestConn("c2")
	hub.Subscribe("room1", sender)
	hub.Subscribe("room1", receiver)

	err := hub.Relay("room1", sender, "move", map[string]any{"x": 2}, true)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}

	_ = mustReadBroadcastMessage(t, sender)
	_ = mustReadBroadcastMessage(t, receiver)
}

func TestBroadcastHub_Unsubscribe(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default())
	sender := newBroadcastTestConn("c1")
	receiver := newBroadcastTestConn("c2")
	hub.Subscribe("room1", sender)
	hub.Subscribe("room1", receiver)
	hub.Unsubscribe("room1", receiver)

	err := hub.Relay("room1", sender, "move", map[string]any{"x": 3}, true)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}

	_ = mustReadBroadcastMessage(t, sender)
	assertNoQueuedBroadcastMessage(t, receiver)
}

func TestBroadcastHub_UnsubscribeAll(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default())
	conn := newBroadcastTestConn("c1")
	sender := newBroadcastTestConn("c2")
	for _, ch := range []string{"room1", "room2", "room3"} {
		hub.Subscribe(ch, conn)
		hub.Subscribe(ch, sender)
	}

	hub.UnsubscribeAll(conn)

	for _, ch := range []string{"room1", "room2", "room3"} {
		err := hub.Relay(ch, sender, "move", map[string]any{"ch": ch}, true)
		if err != nil {
			t.Fatalf("relay %s: %v", ch, err)
		}
	}

	assertNoQueuedBroadcastMessage(t, conn)
}

func TestBroadcastHub_RateLimiting(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default(), BroadcastHubOptions{
		RateLimit:  3,
		RateWindow: time.Second,
	})
	sender := newBroadcastTestConn("c1")
	receiver := newBroadcastTestConn("c2")
	hub.Subscribe("room1", sender)
	hub.Subscribe("room1", receiver)

	for i := 0; i < 3; i++ {
		if err := hub.Relay("room1", sender, "move", map[string]any{"i": i}, false); err != nil {
			t.Fatalf("relay %d unexpectedly failed: %v", i, err)
		}
	}
	if err := hub.Relay("room1", sender, "move", map[string]any{"i": 99}, false); err == nil {
		t.Fatal("expected rate limit error")
	} else if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("expected rate limit error, got %v", err)
	}
}

func TestBroadcastHub_PayloadSizeLimit(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default(), BroadcastHubOptions{
		MaxPayloadBytes: 8,
		RateLimit:       1,
	})
	sender := newBroadcastTestConn("c1")
	receiver := newBroadcastTestConn("c2")
	hub.Subscribe("room1", sender)
	hub.Subscribe("room1", receiver)

	err := hub.Relay("room1", sender, "move", map[string]any{"value": "this is too large"}, false)
	if err == nil {
		t.Fatal("expected payload size error")
	}
	if !strings.Contains(err.Error(), "payload") {
		t.Fatalf("expected payload size error, got %v", err)
	}
}

func TestBroadcastHub_RelayEmptyChannelNoop(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default())
	sender := newBroadcastTestConn("c1")
	if err := hub.Relay("missing", sender, "move", map[string]any{"x": 1}, false); err != nil {
		t.Fatalf("expected no-op relay, got error: %v", err)
	}
	if got := hub.MessagesSent(); got != 0 {
		t.Fatalf("expected MessagesSent=0 for empty channel, got %d", got)
	}
}

func TestBroadcastHub_RelaySenderOnlySelfTrue(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default())
	sender := newBroadcastTestConn("c1")
	hub.Subscribe("room1", sender)

	err := hub.Relay("room1", sender, "move", map[string]any{"x": 1}, true)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}

	msg := mustReadBroadcastMessage(t, sender)
	if msg.Type != MsgTypeBroadcast || msg.Channel != "room1" || msg.Event != "move" {
		t.Fatalf("unexpected relay message: %+v", msg)
	}
	if got := hub.MessagesSent(); got != 1 {
		t.Fatalf("expected MessagesSent=1, got %d", got)
	}
}

func TestBroadcastHub_RelaySenderOnlySelfFalse(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default())
	sender := newBroadcastTestConn("c1")
	hub.Subscribe("room1", sender)

	err := hub.Relay("room1", sender, "move", map[string]any{"x": 1}, false)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	assertNoQueuedBroadcastMessage(t, sender)
	if got := hub.MessagesSent(); got != 0 {
		t.Fatalf("expected MessagesSent=0 when nothing relayed, got %d", got)
	}
}
