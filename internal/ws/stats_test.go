package ws

import (
	"log/slog"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestHandlerStatsSnapshotIncludesCountersAndSubscriptions(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.Broadcast = NewBroadcastHub(slog.Default())
	h.Presence = NewPresenceHub(slog.Default())

	c1 := newBroadcastTestConn("c1")
	c2 := newBroadcastTestConn("c2")
	c1.Subscribe([]string{"posts"})
	c2.Subscribe([]string{"posts", "comments"})
	c1.SubscribeChannel("room1")
	c2.SubscribeChannel("room1")
	c2.SubscribeChannel("room2")
	h.trackConn(c1)
	h.trackConn(c2)
	defer h.removeConn(c1)
	defer h.removeConn(c2)

	h.Broadcast.Subscribe("room1", c1)
	h.Broadcast.Subscribe("room1", c2)
	h.Broadcast.Subscribe("room2", c2)
	_, _ = h.Presence.Track("room1", c1, map[string]any{"user": "alice"})

	snapshot := h.StatsSnapshot()
	testutil.Equal(t, 2, snapshot.Connections)
	testutil.Equal(t, 2, snapshot.TableSubscriptions["posts"])
	testutil.Equal(t, 1, snapshot.TableSubscriptions["comments"])
	testutil.Equal(t, 2, snapshot.ChannelSubscriptions.Broadcast["room1"])
	testutil.Equal(t, 1, snapshot.ChannelSubscriptions.Broadcast["room2"])
	testutil.Equal(t, 1, snapshot.ChannelSubscriptions.Presence["room1"])
}

func TestHandlerDroppedCounterIncrementsOnSendDrop(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	c := &Conn{
		id:            "dropper",
		logger:        slog.Default(),
		subscriptions: map[string]bool{},
		channels:      map[string]bool{},
		send:          make(chan ServerMessage, 1),
		done:          make(chan struct{}),
		onDrop:        func() { h.droppedMessages.Add(1) },
	}

	c.Send(ServerMessage{Type: MsgTypeSystem})
	c.Send(ServerMessage{Type: MsgTypeSystem})

	snapshot := h.StatsSnapshot()
	testutil.Equal(t, uint64(1), snapshot.Counters.DroppedMessages)
}

func TestHandlerHeartbeatFailureCounter(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, slog.Default())
	h.recordHeartbeatFailure()

	snapshot := h.StatsSnapshot()
	testutil.Equal(t, uint64(1), snapshot.Counters.HeartbeatFailures)
}
