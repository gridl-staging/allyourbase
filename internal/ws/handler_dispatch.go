// Package ws handles message dispatch for authenticated WebSocket clients.
package ws

import (
	"context"
	"strings"
	"time"
)

// handleAuth processes an auth message.
func (h *Handler) handleAuth(ctx context.Context, c *Conn, msg ClientMessage, authTimer *time.Timer) {
	if h.authValidator == nil {
		c.setAuth(nil)
		c.Send(replyOK(msg.Ref))
		return
	}

	if msg.Token == "" {
		c.Send(replyError(msg.Ref, "missing token"))
		return
	}

	claims, err := h.validateToken(ctx, msg.Token)
	if err != nil {
		c.Send(replyError(msg.Ref, "authentication failed"))
		return
	}

	c.setAuth(claims)
	if authTimer != nil {
		authTimer.Stop()
	}
	c.Send(replyOK(msg.Ref))
}

// handleSubscribe processes a subscribe message.
func (h *Handler) handleSubscribe(c *Conn, msg ClientMessage) {
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}

	previous := c.Subscriptions()
	c.Subscribe(msg.Tables)
	if h.OnSubscribe != nil {
		if err := h.OnSubscribe(c, msg.Tables, msg.Filter); err != nil {
			// Roll back only tables introduced by this request. Previously
			// subscribed tables must remain subscribed.
			c.Unsubscribe(newlySubscribedTables(previous, msg.Tables))
			c.Send(replyError(msg.Ref, err.Error()))
			return
		}
	}
	c.Send(replyOK(msg.Ref))
}

func newlySubscribedTables(previous map[string]bool, requested []string) []string {
	if len(requested) == 0 {
		return nil
	}
	added := make([]string, 0, len(requested))
	for _, table := range requested {
		if !previous[table] {
			added = append(added, table)
		}
	}
	return added
}

// handleUnsubscribe processes an unsubscribe message.
func (h *Handler) handleUnsubscribe(c *Conn, msg ClientMessage) {
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	c.Unsubscribe(msg.Tables)
	if h.OnUnsubscribe != nil {
		h.OnUnsubscribe(c, msg.Tables)
	}
	c.Send(replyOK(msg.Ref))
}

// handleChannelSubscribe processes a channel subscribe message, validating that broadcast is available and the client is authenticated, then subscribes the connection to the channel and sends the current presence state if presence tracking is enabled.
func (h *Handler) handleChannelSubscribe(c *Conn, msg ClientMessage) {
	if h.Broadcast == nil {
		c.Send(replyError(msg.Ref, "broadcast not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}

	c.SubscribeChannel(channel)
	h.Broadcast.Subscribe(channel, c)
	c.Send(replyOK(msg.Ref))
	if h.Presence != nil {
		h.sendPresenceSync(c, channel)
	}
}

// handleChannelUnsubscribe processes a channel unsubscribe message, validating that broadcast is available and the client is authenticated, then unsubscribes the connection from the channel.
func (h *Handler) handleChannelUnsubscribe(c *Conn, msg ClientMessage) {
	if h.Broadcast == nil {
		c.Send(replyError(msg.Ref, "broadcast not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}

	c.UnsubscribeChannel(channel)
	h.Broadcast.Unsubscribe(channel, c)
	c.Send(replyOK(msg.Ref))
}

// handleBroadcast processes a broadcast message, validating that broadcast is available, the client is authenticated, and the client is subscribed to the target channel, then relays the message to other clients in that channel.
func (h *Handler) handleBroadcast(c *Conn, msg ClientMessage) {
	if h.Broadcast == nil {
		c.Send(replyError(msg.Ref, "broadcast not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}
	if !c.HasChannel(channel) {
		c.Send(replyError(msg.Ref, "not subscribed to channel"))
		return
	}

	if err := h.Broadcast.Relay(channel, c, msg.Event, msg.Payload, msg.Self); err != nil {
		c.Send(replyError(msg.Ref, err.Error()))
		return
	}
	c.Send(replyOK(msg.Ref))
}

// handlePresenceTrack processes a presence track message, validating that presence is available, the client is authenticated, and the client is subscribed to the target channel, then updates the presence state and broadcasts the change to other clients in that channel.
func (h *Handler) handlePresenceTrack(c *Conn, msg ClientMessage) {
	if h.Presence == nil {
		c.Send(replyError(msg.Ref, "presence not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}
	if !c.HasChannel(channel) {
		c.Send(replyError(msg.Ref, "not subscribed to channel"))
		return
	}

	diff, err := h.Presence.Track(channel, c, msg.Presence)
	if err != nil {
		c.Send(replyError(msg.Ref, err.Error()))
		return
	}
	c.SetPresence(channel, msg.Presence)
	h.sendPresenceDiff(diff)
	c.Send(replyOK(msg.Ref))
}

// handlePresenceUntrack processes a presence untrack message, validating that presence is available, the client is authenticated, and the client is subscribed to the target channel, then removes the presence state and broadcasts the change to other clients in that channel.
func (h *Handler) handlePresenceUntrack(c *Conn, msg ClientMessage) {
	if h.Presence == nil {
		c.Send(replyError(msg.Ref, "presence not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}
	if !c.HasChannel(channel) {
		c.Send(replyError(msg.Ref, "not subscribed to channel"))
		return
	}

	diff := h.Presence.Untrack(channel, c)
	c.ClearPresence(channel)
	if diff.Action != "" {
		h.sendPresenceDiff(diff)
	}
	c.Send(replyOK(msg.Ref))
}

// handlePresenceSync processes a presence sync message, validating that presence is available, the client is authenticated, and the client is subscribed to the target channel, then sends the full presence state for that channel to the client.
func (h *Handler) handlePresenceSync(c *Conn, msg ClientMessage) {
	if h.Presence == nil {
		c.Send(replyError(msg.Ref, "presence not available"))
		return
	}
	if h.authValidator != nil && !c.Authenticated() {
		c.Send(replyError(msg.Ref, "authentication required"))
		return
	}
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		c.Send(replyError(msg.Ref, "channel is required"))
		return
	}
	if !c.HasChannel(channel) {
		c.Send(replyError(msg.Ref, "not subscribed to channel"))
		return
	}

	h.sendPresenceSync(c, channel)
	c.Send(replyOK(msg.Ref))
}

func (h *Handler) sendPresenceSync(c *Conn, channel string) {
	if h.Presence == nil {
		return
	}
	h.Presence.RecordSync()
	c.Send(PresenceSyncMsg(channel, h.Presence.Sync(channel)))
}

// sendPresenceDiff sends a presence diff message to all connected clients that are subscribed to the channel, notifying them of presence state changes.
func (h *Handler) sendPresenceDiff(diff PresenceDiff) {
	if diff.Action == "" {
		return
	}
	if h.Presence != nil {
		h.Presence.RecordSync()
	}
	msg := PresenceDiffMsg(diff.Channel, diff.Action, diff.ConnID, diff.Presence)
	h.mu.Lock()
	conns := make([]*Conn, 0, len(h.conns))
	for _, conn := range h.conns {
		conns = append(conns, conn)
	}
	h.mu.Unlock()
	for _, conn := range conns {
		if conn.HasChannel(diff.Channel) {
			conn.Send(msg)
		}
	}
}
