package ws

import (
	"fmt"
	"log/slog"
	"sync"
	"testing"
)

func TestPresenceHub_ConcurrentTrackUntrackSync(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default())
	const clientsN = 60
	clients := make([]*Conn, 0, clientsN)
	for i := 0; i < clientsN; i++ {
		clients = append(clients, newBroadcastTestConn(fmt.Sprintf("c-%d", i)))
	}

	var wg sync.WaitGroup
	for i := 0; i < clientsN; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := clients[i]
			for j := 0; j < 100; j++ {
				_, _ = hub.Track("room1", c, map[string]any{"i": i, "j": j})
				_ = hub.Sync("room1")
				if j%3 == 0 {
					hub.Untrack("room1", c)
				}
			}
		}()
	}
	wg.Wait()

	_ = hub.Sync("room1")
}
