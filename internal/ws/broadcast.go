// Package ws implements a broadcast hub that manages channel subscriptions and relays messages between clients with rate limiting and payload size constraints.
package ws

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// BroadcastHub manages channel subscriptions and client-to-client broadcasts.
type BroadcastHub struct {
	mu       sync.RWMutex
	channels map[string]map[string]*Conn // channel -> connID -> conn
	logger   *slog.Logger

	MaxPayloadBytes int
	RateLimit       int
	RateWindow      time.Duration

	rateMu       sync.Mutex
	rates        map[string]*rateBucket // connID -> rate bucket
	messagesSent atomic.Uint64
}

type rateBucket struct {
	times []time.Time
}

type BroadcastHubOptions struct {
	RateLimit       int
	RateWindow      time.Duration
	MaxPayloadBytes int
}

const (
	defaultBroadcastRateLimit       = 100
	defaultBroadcastRateWindow      = time.Second
	defaultBroadcastMaxPayloadBytes = 262144
)

// NewBroadcastHub builds a broadcast hub with spec-aligned defaults and optional
// explicit overrides.
func NewBroadcastHub(logger *slog.Logger, opts ...BroadcastHubOptions) *BroadcastHub {
	if logger == nil {
		logger = slog.Default()
	}
	cfg := normalizeBroadcastHubOptions(opts...)
	return &BroadcastHub{
		channels:        make(map[string]map[string]*Conn),
		rates:           make(map[string]*rateBucket),
		logger:          logger,
		MaxPayloadBytes: cfg.MaxPayloadBytes,
		RateLimit:       cfg.RateLimit,
		RateWindow:      cfg.RateWindow,
	}
}

// normalizeBroadcastHubOptions applies defaults to any zero-valued fields in the first provided option, returning a fully initialized BroadcastHubOptions.
func normalizeBroadcastHubOptions(opts ...BroadcastHubOptions) BroadcastHubOptions {
	cfg := BroadcastHubOptions{
		RateLimit:       defaultBroadcastRateLimit,
		RateWindow:      defaultBroadcastRateWindow,
		MaxPayloadBytes: defaultBroadcastMaxPayloadBytes,
	}
	if len(opts) == 0 {
		return cfg
	}
	cfg = opts[0]
	if cfg.RateLimit == 0 {
		cfg.RateLimit = defaultBroadcastRateLimit
	}
	if cfg.RateWindow == 0 {
		cfg.RateWindow = defaultBroadcastRateWindow
	}
	if cfg.MaxPayloadBytes == 0 {
		cfg.MaxPayloadBytes = defaultBroadcastMaxPayloadBytes
	}
	return cfg
}

// Subscribe subscribes a connection to a channel.
func (h *BroadcastHub) Subscribe(channel string, c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.channels[channel]; !ok {
		h.channels[channel] = make(map[string]*Conn)
	}
	h.channels[channel][c.ID()] = c
}

// Unsubscribe removes a connection from a channel.
func (h *BroadcastHub) Unsubscribe(channel string, c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	members, ok := h.channels[channel]
	if !ok {
		return
	}
	delete(members, c.ID())
	if len(members) == 0 {
		delete(h.channels, channel)
	}
}

// UnsubscribeAll removes a connection from all channels and clears rate tracking.
func (h *BroadcastHub) UnsubscribeAll(c *Conn) {
	h.mu.Lock()
	for channel, members := range h.channels {
		delete(members, c.ID())
		if len(members) == 0 {
			delete(h.channels, channel)
		}
	}
	h.mu.Unlock()

	h.rateMu.Lock()
	delete(h.rates, c.ID())
	h.rateMu.Unlock()
}

// Relay relays a broadcast event to channel subscribers.
func (h *BroadcastHub) Relay(channel string, sender *Conn, event string, payload map[string]any, self bool) error {
	if err := h.checkRateLimit(sender.ID()); err != nil {
		return err
	}
	if err := h.checkPayloadSize(payload); err != nil {
		return err
	}

	h.mu.RLock()
	members, ok := h.channels[channel]
	if !ok || len(members) == 0 {
		h.mu.RUnlock()
		return nil
	}
	recipients := make([]*Conn, 0, len(members))
	for _, member := range members {
		recipients = append(recipients, member)
	}
	h.mu.RUnlock()

	msg := BroadcastMsg(channel, event, payload)
	relayed := false
	for _, recipient := range recipients {
		if !self && recipient.ID() == sender.ID() {
			continue
		}
		recipient.Send(msg)
		relayed = true
	}
	if relayed {
		h.messagesSent.Add(1)
	}
	return nil
}

func (h *BroadcastHub) checkPayloadSize(payload map[string]any) error {
	if h.MaxPayloadBytes <= 0 {
		return nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("invalid broadcast payload: %w", err)
	}
	if len(encoded) > h.MaxPayloadBytes {
		return fmt.Errorf("broadcast payload exceeds %d bytes", h.MaxPayloadBytes)
	}
	return nil
}

// checkRateLimit enforces a sliding-window rate limit for the given connection, returning an error if the limit is exceeded.
func (h *BroadcastHub) checkRateLimit(connID string) error {
	if h.RateLimit <= 0 || h.RateWindow <= 0 {
		return nil
	}

	now := time.Now()
	windowStart := now.Add(-h.RateWindow)

	h.rateMu.Lock()
	bucket, ok := h.rates[connID]
	if !ok {
		bucket = &rateBucket{}
		h.rates[connID] = bucket
	}

	pruned := bucket.times[:0]
	for _, ts := range bucket.times {
		if ts.After(windowStart) {
			pruned = append(pruned, ts)
		}
	}
	bucket.times = pruned

	if len(bucket.times) >= h.RateLimit {
		h.rateMu.Unlock()
		return fmt.Errorf("broadcast rate limit exceeded")
	}
	bucket.times = append(bucket.times, now)
	h.rateMu.Unlock()
	return nil
}

// ChannelCounts returns a snapshot of channel subscriber counts.
func (h *BroadcastHub) ChannelCounts() map[string]int {
	h.mu.RLock()
	cp := make(map[string]int, len(h.channels))
	for channel, members := range h.channels {
		cp[channel] = len(members)
	}
	h.mu.RUnlock()
	return cp
}

// MessagesSent returns the total number of broadcast messages relayed.
func (h *BroadcastHub) MessagesSent() uint64 {
	return h.messagesSent.Load()
}
