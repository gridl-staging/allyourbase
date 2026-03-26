package ws

import "sync/atomic"

// HandlerCountersSnapshot is a stable counter view for inspector consumers.
type HandlerCountersSnapshot struct {
	DroppedMessages   uint64 `json:"dropped_messages"`
	HeartbeatFailures uint64 `json:"heartbeat_failures"`
}

// ChannelSubscriptionSnapshot contains channel-level counts by feature.
type ChannelSubscriptionSnapshot struct {
	Broadcast map[string]int `json:"broadcast"`
	Presence  map[string]int `json:"presence"`
}

// StatsSnapshot is the inspector-facing WebSocket runtime snapshot.
type StatsSnapshot struct {
	Connections          int                         `json:"connections"`
	TableSubscriptions   map[string]int              `json:"table_subscriptions"`
	ChannelSubscriptions ChannelSubscriptionSnapshot `json:"channel_subscriptions"`
	Counters             HandlerCountersSnapshot     `json:"counters"`
}

func (h *Handler) recordHeartbeatFailure() {
	h.heartbeatFailures.Add(1)
}

func (h *Handler) recordDroppedMessage() {
	h.droppedMessages.Add(1)
}

func (h *Handler) counterSnapshot() HandlerCountersSnapshot {
	return HandlerCountersSnapshot{
		DroppedMessages:   h.droppedMessages.Load(),
		HeartbeatFailures: h.heartbeatFailures.Load(),
	}
}

func (h *Handler) setConnDropHook(c *Conn) {
	c.onDrop = h.recordDroppedMessage
}

// StatsSnapshot returns an inspector-safe view of ws runtime state.
func (h *Handler) StatsSnapshot() StatsSnapshot {
	h.mu.Lock()
	conns := make([]*Conn, 0, len(h.conns))
	for _, c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.Unlock()

	tableCounts := make(map[string]int)
	for _, c := range conns {
		for table := range c.Subscriptions() {
			tableCounts[table]++
		}
	}

	broadcastCounts := map[string]int{}
	if h.Broadcast != nil {
		broadcastCounts = h.Broadcast.ChannelCounts()
	}
	presenceCounts := map[string]int{}
	if h.Presence != nil {
		presenceCounts = h.Presence.ChannelCounts()
	}

	return StatsSnapshot{
		Connections:        len(conns),
		TableSubscriptions: tableCounts,
		ChannelSubscriptions: ChannelSubscriptionSnapshot{
			Broadcast: broadcastCounts,
			Presence:  presenceCounts,
		},
		Counters: h.counterSnapshot(),
	}
}

// compile-time assertion that typed atomics remain used for hot counters.
var (
	_ atomic.Uint64
)
