package realtime_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/ws"
)

type fakeWSStatsProvider struct {
	snapshot ws.StatsSnapshot
}

func (f fakeWSStatsProvider) StatsSnapshot() ws.StatsSnapshot {
	return f.snapshot
}

func TestHubStatsSnapshotIncludesTableBreakdownAndDrops(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	for i := 0; i < 256; i++ {
		hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": i}})
	}
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 999}})

	snapshot := hub.StatsSnapshot()
	testutil.Equal(t, 1, snapshot.Connections)
	testutil.Equal(t, 1, snapshot.TableSubscriptions["posts"])
	testutil.Equal(t, uint64(1), snapshot.Counters.DroppedMessages)
}

func TestInspectorSnapshotAggregatesRealtimeAndWS(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	c1 := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(c1.ID)
	c2 := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(c2.ID)
	c3 := hub.Subscribe(map[string]bool{"comments": true})
	defer hub.Unsubscribe(c3.ID)

	inspector := realtime.NewInspector(hub, fakeWSStatsProvider{
		snapshot: ws.StatsSnapshot{
			Connections:        2,
			TableSubscriptions: map[string]int{"posts": 2, "comments": 1},
			ChannelSubscriptions: ws.ChannelSubscriptionSnapshot{
				Broadcast: map[string]int{"room1": 2},
				Presence:  map[string]int{"room1": 1},
			},
			Counters: ws.HandlerCountersSnapshot{
				DroppedMessages:   3,
				HeartbeatFailures: 4,
			},
		},
	})

	snapshot := inspector.Snapshot()
	testutil.Equal(t, 3, snapshot.Connections.Total)
	testutil.Equal(t, 1, snapshot.Connections.SSE)
	testutil.Equal(t, 2, snapshot.Connections.WS)
	testutil.Equal(t, 2, snapshot.Subscriptions.Tables["posts"])
	testutil.Equal(t, 1, snapshot.Subscriptions.Tables["comments"])
	testutil.Equal(t, 2, snapshot.Subscriptions.Channels.Broadcast["room1"])
	testutil.Equal(t, 1, snapshot.Subscriptions.Channels.Presence["room1"])
	testutil.Equal(t, uint64(3), snapshot.Counters.DroppedMessages)
	testutil.Equal(t, uint64(4), snapshot.Counters.HeartbeatFailures)
	testutil.Equal(t, realtime.InspectorVersion, snapshot.Version)
	testutil.True(t, time.Since(snapshot.Timestamp) < 2*time.Second, "snapshot timestamp should be recent")
}

func TestHubStatsSnapshotConcurrentSubscribeUnsubscribe(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client := hub.Subscribe(map[string]bool{fmt.Sprintf("t%d", i%5): true})
			_ = hub.StatsSnapshot()
			hub.Unsubscribe(client.ID)
		}(i)
	}
	wg.Wait()
	snapshot := hub.StatsSnapshot()
	testutil.Equal(t, 0, snapshot.Connections)
}
