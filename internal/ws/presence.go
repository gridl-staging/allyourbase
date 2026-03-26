// Package ws PresenceHub tracks channel member presence state for WebSocket connections. It manages join, update, and deferred leave events across channels.
package ws

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// PresenceDiff represents a presence change for a single connection.
type PresenceDiff struct {
	Action   string
	Channel  string
	ConnID   string
	Presence map[string]any
}

type presenceEntry struct {
	payload   map[string]any
	updatedAt time.Time
}

// PresenceHub tracks channel member presence state.
type PresenceHub struct {
	mu       sync.RWMutex
	channels map[string]map[string]presenceEntry // channel -> connID -> entry
	logger   *slog.Logger

	MaxPayloadBytes int
	LeaveTimeout    time.Duration

	pendingLeaves map[string]scheduledLeave // key = connID + "\x00" + channel
	nextLeaveID   uint64
	syncsSent     atomic.Uint64
}

type PresenceHubOptions struct {
	MaxPayloadBytes int
	LeaveTimeout    time.Duration
}

const (
	defaultPresenceMaxPayloadBytes = 4096
	defaultPresenceLeaveTimeout    = 10 * time.Second
)

// NewPresenceHub builds a presence hub with optional explicit limits.
func NewPresenceHub(logger *slog.Logger, opts ...PresenceHubOptions) *PresenceHub {
	if logger == nil {
		logger = slog.Default()
	}
	cfg := normalizePresenceHubOptions(opts...)
	return &PresenceHub{
		channels:        make(map[string]map[string]presenceEntry),
		logger:          logger,
		MaxPayloadBytes: cfg.MaxPayloadBytes,
		LeaveTimeout:    cfg.LeaveTimeout,
		pendingLeaves:   make(map[string]scheduledLeave),
	}
}

type scheduledLeave struct {
	timer *time.Timer
	id    uint64
}

// returns normalized options with default values applied to any zero-valued fields.
func normalizePresenceHubOptions(opts ...PresenceHubOptions) PresenceHubOptions {
	cfg := PresenceHubOptions{
		MaxPayloadBytes: defaultPresenceMaxPayloadBytes,
		LeaveTimeout:    defaultPresenceLeaveTimeout,
	}
	if len(opts) == 0 {
		return cfg
	}
	cfg = opts[0]
	if cfg.MaxPayloadBytes == 0 {
		cfg.MaxPayloadBytes = defaultPresenceMaxPayloadBytes
	}
	if cfg.LeaveTimeout == 0 {
		cfg.LeaveTimeout = defaultPresenceLeaveTimeout
	}
	return cfg
}

// Track upserts channel presence for a connection and returns join/update diff.
func (h *PresenceHub) Track(channel string, c *Conn, payload map[string]any) (PresenceDiff, error) {
	if err := h.checkPayloadSize(payload); err != nil {
		return PresenceDiff{}, err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.cancelPendingLeaveLocked(c.ID(), channel)

	members, ok := h.channels[channel]
	if !ok {
		members = make(map[string]presenceEntry)
		h.channels[channel] = members
	}

	_, exists := members[c.ID()]
	action := PresenceActionJoin
	if exists {
		action = PresenceActionUpdate
	}

	members[c.ID()] = presenceEntry{
		payload:   copyMap(payload),
		updatedAt: time.Now(),
	}

	return PresenceDiff{Action: action, Channel: channel, ConnID: c.ID(), Presence: copyMap(payload)}, nil
}

// Untrack removes a connection's presence from a channel and returns leave diff when present.
func (h *PresenceHub) Untrack(channel string, c *Conn) PresenceDiff {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cancelPendingLeaveLocked(c.ID(), channel)

	members, ok := h.channels[channel]
	if !ok {
		return PresenceDiff{}
	}
	entry, exists := members[c.ID()]
	if !exists {
		return PresenceDiff{}
	}
	delete(members, c.ID())
	if len(members) == 0 {
		delete(h.channels, channel)
	}
	return PresenceDiff{
		Action:   PresenceActionLeave,
		Channel:  channel,
		ConnID:   c.ID(),
		Presence: copyMap(entry.payload),
	}
}

// Sync returns a deep copy of the channel presence snapshot.
func (h *PresenceHub) Sync(channel string) map[string]map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()

	members, ok := h.channels[channel]
	if !ok {
		return map[string]map[string]any{}
	}
	cp := make(map[string]map[string]any, len(members))
	for connID, entry := range members {
		cp[connID] = copyMap(entry.payload)
	}
	return cp
}

// UntrackAll removes presence from all channels for a connection and returns leave diffs.
func (h *PresenceHub) UntrackAll(c *Conn) []PresenceDiff {
	h.mu.Lock()
	defer h.mu.Unlock()

	diffs := make([]PresenceDiff, 0)
	for channel, members := range h.channels {
		entry, exists := members[c.ID()]
		if !exists {
			continue
		}
		delete(members, c.ID())
		diffs = append(diffs, PresenceDiff{
			Action:   PresenceActionLeave,
			Channel:  channel,
			ConnID:   c.ID(),
			Presence: copyMap(entry.payload),
		})
		h.cancelPendingLeaveLocked(c.ID(), channel)
		if len(members) == 0 {
			delete(h.channels, channel)
		}
	}
	return diffs
}

// DeferredUntrackAll schedules leave diffs for later broadcast.
//
// If LeaveTimeout <= 0, it delegates to UntrackAll immediately.
func (h *PresenceHub) DeferredUntrackAll(c *Conn, callbacks ...func(PresenceDiff)) {
	var callback func(PresenceDiff)
	if len(callbacks) > 0 {
		callback = callbacks[0]
	}
	if h.LeaveTimeout <= 0 {
		diffs := h.UntrackAll(c)
		for _, diff := range diffs {
			if callback != nil {
				callback(diff)
			}
		}
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for channel := range h.channels {
		if _, exists := h.channels[channel][c.ID()]; !exists {
			continue
		}
		h.scheduleLeaveLocked(c.ID(), channel, callback)
	}
}

// cancelPendingLeaveLocked clears any scheduled leave timer for a conn/channel pair.
// Caller must hold h.mu.
func (h *PresenceHub) cancelPendingLeaveLocked(connID, channel string) {
	key := presenceLeaveKey(connID, channel)
	pending, ok := h.pendingLeaves[key]
	if !ok {
		return
	}
	delete(h.pendingLeaves, key)
	pending.timer.Stop()
}

// schedules a deferred removal of a connection from a channel after LeaveTimeout, invoking the callback with the leave diff when the timeout fires. Caller must hold h.mu.
func (h *PresenceHub) scheduleLeaveLocked(connID, channel string, callback func(PresenceDiff)) {
	key := presenceLeaveKey(connID, channel)
	h.cancelPendingLeaveLocked(connID, channel)
	h.nextLeaveID++
	leaveID := h.nextLeaveID

	timer := time.AfterFunc(h.LeaveTimeout, func() {
		diff := h.consumeLeaveDiff(connID, channel, leaveID)
		if callback != nil && diff.Action != "" {
			callback(diff)
		}
	})
	h.pendingLeaves[key] = scheduledLeave{
		timer: timer,
		id:    leaveID,
	}
}

// removes a connection's presence from a channel when a scheduled leave timeout fires. Returns an empty diff if the leave was already cancelled or consumed.
func (h *PresenceHub) consumeLeaveDiff(connID, channel string, leaveID uint64) PresenceDiff {
	key := presenceLeaveKey(connID, channel)

	h.mu.Lock()
	defer h.mu.Unlock()

	current, ok := h.pendingLeaves[key]
	if !ok || current.id != leaveID {
		return PresenceDiff{}
	}
	delete(h.pendingLeaves, key)

	members, ok := h.channels[channel]
	if !ok {
		return PresenceDiff{}
	}
	entry, exists := members[connID]
	if !exists {
		return PresenceDiff{}
	}
	delete(members, connID)
	if len(members) == 0 {
		delete(h.channels, channel)
	}

	return PresenceDiff{
		Action:   PresenceActionLeave,
		Channel:  channel,
		ConnID:   connID,
		Presence: copyMap(entry.payload),
	}
}

func presenceLeaveKey(connID, channel string) string {
	return connID + "\x00" + channel
}

func (h *PresenceHub) checkPayloadSize(payload map[string]any) error {
	if h.MaxPayloadBytes <= 0 {
		return nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("invalid presence payload: %w", err)
	}
	if len(encoded) > h.MaxPayloadBytes {
		return fmt.Errorf("presence payload exceeds %d bytes", h.MaxPayloadBytes)
	}
	return nil
}

// ChannelCounts returns a snapshot of presence member counts per channel.
func (h *PresenceHub) ChannelCounts() map[string]int {
	h.mu.RLock()
	cp := make(map[string]int, len(h.channels))
	for channel, members := range h.channels {
		cp[channel] = len(members)
	}
	h.mu.RUnlock()
	return cp
}

// RecordSync increments the counter for presence sync/diff messages sent.
func (h *PresenceHub) RecordSync() {
	h.syncsSent.Add(1)
}

// SyncedCount returns the total number of presence sync/diff messages sent.
func (h *PresenceHub) SyncedCount() uint64 {
	return h.syncsSent.Load()
}
