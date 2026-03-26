package realtime

import (
	"time"

	"github.com/allyourbase/ayb/internal/ws"
)

const InspectorVersion = "v1"

// HubCountersSnapshot contains hot-path counters for SSE/hub delivery.
type HubCountersSnapshot struct {
	DroppedMessages uint64 `json:"dropped_messages"`
}

// HubStatsSnapshot is the SSE hub state used by inspector aggregation.
type HubStatsSnapshot struct {
	Connections        int            `json:"connections"`
	TableSubscriptions map[string]int `json:"table_subscriptions"`
	Counters           HubCountersSnapshot
}

// StatsSnapshot returns an inspector-safe view of hub runtime state.
func (h *Hub) StatsSnapshot() HubStatsSnapshot {
	h.mu.RLock()
	clientTables := make([]map[string]bool, 0, len(h.clients))
	for _, c := range h.clients {
		tables := make(map[string]bool, len(c.tables))
		for t, on := range c.tables {
			tables[t] = on
		}
		clientTables = append(clientTables, tables)
	}
	h.mu.RUnlock()

	tableCounts := make(map[string]int)
	for _, tables := range clientTables {
		for table := range tables {
			tableCounts[table]++
		}
	}

	return HubStatsSnapshot{
		Connections:        len(clientTables),
		TableSubscriptions: tableCounts,
		Counters: HubCountersSnapshot{
			DroppedMessages: h.dropped.Load(),
		},
	}
}

// ConnectionsSnapshot is a transport-level connection view.
type ConnectionsSnapshot struct {
	SSE   int `json:"sse"`
	WS    int `json:"ws"`
	Total int `json:"total"`
}

// ChannelSubscriptionsSnapshot contains channel-level usage by feature.
type ChannelSubscriptionsSnapshot struct {
	Broadcast map[string]int `json:"broadcast"`
	Presence  map[string]int `json:"presence"`
}

// SubscriptionSnapshot contains all subscription breakdown dimensions.
type SubscriptionSnapshot struct {
	Tables   map[string]int               `json:"tables"`
	Channels ChannelSubscriptionsSnapshot `json:"channels"`
}

// CounterSnapshot contains aggregate reliability counters.
type CounterSnapshot struct {
	DroppedMessages   uint64 `json:"dropped_messages"`
	HeartbeatFailures uint64 `json:"heartbeat_failures"`
}

// Snapshot is the full inspector payload.
type Snapshot struct {
	Version       string               `json:"version"`
	Timestamp     time.Time            `json:"timestamp"`
	Connections   ConnectionsSnapshot  `json:"connections"`
	Subscriptions SubscriptionSnapshot `json:"subscriptions"`
	Counters      CounterSnapshot      `json:"counters"`
}

// WSSnapshotProvider allows server to pass ws.Handler (or mock).
type WSSnapshotProvider interface {
	StatsSnapshot() ws.StatsSnapshot
}

// Inspector composes hub + ws runtime state into one response payload.
type Inspector struct {
	hub *Hub
	ws  WSSnapshotProvider
}

func NewInspector(hub *Hub, wsProvider WSSnapshotProvider) *Inspector {
	return &Inspector{hub: hub, ws: wsProvider}
}

// Snapshot returns a read-only aggregate runtime view.
func (i *Inspector) Snapshot() Snapshot {
	hubSnap := HubStatsSnapshot{
		TableSubscriptions: map[string]int{},
	}
	if i.hub != nil {
		hubSnap = i.hub.StatsSnapshot()
	}

	wsSnap := ws.StatsSnapshot{
		TableSubscriptions: map[string]int{},
		ChannelSubscriptions: ws.ChannelSubscriptionSnapshot{
			Broadcast: map[string]int{},
			Presence:  map[string]int{},
		},
	}
	if i.ws != nil {
		wsSnap = i.ws.StatsSnapshot()
	}

	tables := make(map[string]int)
	if len(hubSnap.TableSubscriptions) > 0 || hubSnap.Connections > 0 {
		// Stage 6 mirrors WS table subscriptions into realtime.Hub. Use hub
		// table counts as the canonical SSE+WS total to avoid double counting.
		for table, count := range hubSnap.TableSubscriptions {
			tables[table] = count
		}
	} else {
		for table, count := range wsSnap.TableSubscriptions {
			tables[table] = count
		}
	}

	sseConnections := hubSnap.Connections - wsSnap.Connections
	if sseConnections < 0 {
		sseConnections = 0
	}

	return Snapshot{
		Version:   InspectorVersion,
		Timestamp: time.Now().UTC(),
		Connections: ConnectionsSnapshot{
			SSE:   sseConnections,
			WS:    wsSnap.Connections,
			Total: sseConnections + wsSnap.Connections,
		},
		Subscriptions: SubscriptionSnapshot{
			Tables: tables,
			Channels: ChannelSubscriptionsSnapshot{
				Broadcast: wsSnap.ChannelSubscriptions.Broadcast,
				Presence:  wsSnap.ChannelSubscriptions.Presence,
			},
		},
		Counters: CounterSnapshot{
			DroppedMessages:   hubSnap.Counters.DroppedMessages + wsSnap.Counters.DroppedMessages,
			HeartbeatFailures: wsSnap.Counters.HeartbeatFailures,
		},
	}
}
