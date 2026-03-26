package ws

import (
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestBroadcastHub_Concurrency50Clients(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default(), BroadcastHubOptions{RateLimit: 1000, RateWindow: time.Second})
	const total = 60

	clients := make([]*Conn, 0, total)
	for i := 0; i < total; i++ {
		c := newBroadcastTestConn(fmt.Sprintf("c-%d", i))
		hub.Subscribe("room", c)
		clients = append(clients, c)
	}

	sender := clients[0]
	if err := hub.Relay("room", sender, "move", map[string]any{"x": 1}, false); err != nil {
		t.Fatalf("relay: %v", err)
	}

	for i := 1; i < total; i++ {
		msg := mustReadBroadcastMessage(t, clients[i])
		if msg.Type != MsgTypeBroadcast || msg.Channel != "room" {
			t.Fatalf("client %d got unexpected message: %+v", i, msg)
		}
	}
	assertNoQueuedBroadcastMessage(t, sender)
}

func TestBroadcastHub_ConcurrentInterleavingNoPanics(t *testing.T) {
	t.Parallel()
	hub := NewBroadcastHub(slog.Default(), BroadcastHubOptions{RateLimit: 100000, RateWindow: time.Second})

	const (
		clientsN = 30
		workers  = 24
		opsEach  = 300
	)

	clients := make([]*Conn, 0, clientsN)
	for i := 0; i < clientsN; i++ {
		clients = append(clients, newBroadcastTestConn(fmt.Sprintf("c-%d", i)))
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsEach; i++ {
				c := clients[(w+i)%clientsN]
				channel := fmt.Sprintf("room-%d", i%5)
				switch i % 3 {
				case 0:
					hub.Subscribe(channel, c)
				case 1:
					hub.Unsubscribe(channel, c)
				default:
					_ = hub.Relay(channel, c, "tick", map[string]any{"i": i}, false)
				}
			}
		}()
	}
	wg.Wait()
}
