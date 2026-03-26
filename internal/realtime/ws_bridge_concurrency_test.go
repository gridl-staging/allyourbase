package realtime_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/ws"
	"github.com/gorilla/websocket"
)

func TestBridgeConcurrency100Clients(t *testing.T) {
	hub, srv := setupBridgeServer(t)

	const numClients = 100
	const numEvents = 10

	// Connect 100 WS clients and subscribe to "posts".
	conns := make([]*websocket.Conn, numClients)
	for i := 0; i < numClients; i++ {
		conn := wsConnect(t, srv.URL)
		wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
		wsReadReply(t, conn)
		conns[i] = conn
	}

	// Wait for all hub clients to be registered.
	deadline := time.Now().Add(2 * time.Second)
	for hub.ClientCount() < numClients && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	testutil.True(t, hub.ClientCount() >= numClients,
		"expected at least %d hub clients, got %d", numClients, hub.ClientCount())

	// Publish 10 events.
	for i := 0; i < numEvents; i++ {
		hub.Publish(&realtime.Event{
			Action: "create",
			Table:  "posts",
			Record: map[string]any{"id": i},
		})
	}

	// Each client should receive all 10 events.
	var wg sync.WaitGroup
	var totalReceived atomic.Int64
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(conn *websocket.Conn) {
			defer wg.Done()
			for j := 0; j < numEvents; j++ {
				_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
				var msg ws.ServerMessage
				if err := conn.ReadJSON(&msg); err != nil {
					return
				}
				if msg.Type == "event" {
					totalReceived.Add(1)
				}
			}
		}(conns[i])
	}
	wg.Wait()

	testutil.Equal(t, int64(numClients*numEvents), totalReceived.Load())

	// Close all connections.
	for _, conn := range conns {
		conn.Close()
	}

	// Wait for cleanup.
	deadline = time.Now().Add(5 * time.Second)
	for hub.ClientCount() > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	testutil.Equal(t, 0, hub.ClientCount())
}

func TestBridgeConcurrentSubscribeUnsubscribe(t *testing.T) {
	hub, srv := setupBridgeServer(t)

	const totalClients = 50
	const unsubCount = 25

	// Connect 50 clients, all subscribe to "posts".
	conns := make([]*websocket.Conn, totalClients)
	for i := 0; i < totalClients; i++ {
		conn := wsConnect(t, srv.URL)
		wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
		wsReadReply(t, conn)
		conns[i] = conn
	}

	// Unsubscribe the first 25.
	for i := 0; i < unsubCount; i++ {
		wsSendJSON(t, conns[i], map[string]any{"type": "unsubscribe", "tables": []string{"posts"}})
		wsReadReply(t, conns[i])
	}

	// Small delay for hub table updates to propagate.
	time.Sleep(10 * time.Millisecond)

	// Publish one event.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})

	// The remaining 25 should receive it.
	var received atomic.Int64
	var wg sync.WaitGroup
	for i := unsubCount; i < totalClients; i++ {
		wg.Add(1)
		go func(conn *websocket.Conn) {
			defer wg.Done()
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			var msg ws.ServerMessage
			if err := conn.ReadJSON(&msg); err == nil && msg.Type == "event" {
				received.Add(1)
			}
		}(conns[i])
	}
	wg.Wait()
	testutil.Equal(t, int64(totalClients-unsubCount), received.Load())

	// The first 25 (unsubscribed) should NOT receive anything.
	var unexpected atomic.Int64
	for i := 0; i < unsubCount; i++ {
		wg.Add(1)
		go func(conn *websocket.Conn) {
			defer wg.Done()
			_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			var msg ws.ServerMessage
			if err := conn.ReadJSON(&msg); err == nil && msg.Type == "event" {
				unexpected.Add(1)
			}
		}(conns[i])
	}
	wg.Wait()
	testutil.Equal(t, int64(0), unexpected.Load())

	// Cleanup.
	for _, conn := range conns {
		conn.Close()
	}
}

func TestBridgeRapidConnectDisconnect(t *testing.T) {
	hub, srv := setupBridgeServer(t)

	// Get the ws.Handler for ConnCount checking.
	// Since we can't access it directly, we'll rely on hub.ClientCount.
	const numGoroutines = 100

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := wsConnect(t, srv.URL)
			wsSendJSON(t, conn, map[string]any{"type": "subscribe", "tables": []string{"posts"}})
			wsReadReply(t, conn)
			conn.Close()
		}()
	}
	wg.Wait()

	// Wait for all cleanup to complete.
	deadline := time.Now().Add(5 * time.Second)
	for hub.ClientCount() > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	testutil.Equal(t, 0, hub.ClientCount())
}
