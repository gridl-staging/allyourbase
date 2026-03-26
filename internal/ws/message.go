// Package ws Message defines WebSocket message types and structures for server-client communication, including helper functions for constructing specific message types.
package ws

import (
	"encoding/json"
	"fmt"
)

// Client→server message types.
const (
	MsgTypeAuth               = "auth"
	MsgTypeSubscribe          = "subscribe"
	MsgTypeUnsubscribe        = "unsubscribe"
	MsgTypeChannelSubscribe   = "channel_subscribe"
	MsgTypeChannelUnsubscribe = "channel_unsubscribe"
	MsgTypePresenceTrack      = "presence_track"
	MsgTypePresenceUntrack    = "presence_untrack"
	MsgTypePresenceSync       = "presence_sync"
	// MsgTypeBroadcast is used in both client->server and server->client messages.
	MsgTypeBroadcast = "broadcast"
)

// Server→client message types.
const (
	MsgTypeConnected = "connected"
	MsgTypeReply     = "reply"
	MsgTypeEvent     = "event"
	MsgTypePresence  = "presence"
	MsgTypeError     = "error"
	MsgTypeSystem    = "system"
)

// ClientMessage represents a message sent from client to server.
type ClientMessage struct {
	Type     string         `json:"type"`
	Ref      string         `json:"ref,omitempty"`
	Token    string         `json:"token,omitempty"`    // for "auth"
	Tables   []string       `json:"tables,omitempty"`   // for "subscribe"/"unsubscribe"`
	Filter   string         `json:"filter,omitempty"`   // for "subscribe" - column-level filter
	Channel  string         `json:"channel,omitempty"`  // for broadcast channel messages
	Event    string         `json:"event,omitempty"`    // for "broadcast"
	Payload  map[string]any `json:"payload,omitempty"`  // for "broadcast"
	Self     bool           `json:"self,omitempty"`     // for "broadcast"
	Presence map[string]any `json:"presence,omitempty"` // for presence messages
}

// ServerMessage represents a message sent from server to client.
// ServerMessage represents a message sent from the server to the client over a WebSocket connection. Fields are populated based on the message Type: "connected" uses ClientID, "reply" uses Status and Message, "event" uses Action/Table/Record, "broadcast" uses Channel/Event/Payload, and presence messages use the presence-related fields.
type ServerMessage struct {
	Type           string                    `json:"type"`
	Ref            string                    `json:"ref,omitempty"`
	ClientID       string                    `json:"client_id,omitempty"`        // for "connected"
	Status         string                    `json:"status,omitempty"`           // for "reply": "ok" or "error"
	Message        string                    `json:"message,omitempty"`          // for "reply"/"error"/"system"
	Action         string                    `json:"action,omitempty"`           // for "event"
	Table          string                    `json:"table,omitempty"`            // for "event"
	Record         map[string]any            `json:"record,omitempty"`           // for "event"
	Channel        string                    `json:"channel,omitempty"`          // for "broadcast"
	Event          string                    `json:"event,omitempty"`            // for "broadcast"
	Payload        map[string]any            `json:"payload,omitempty"`          // for "broadcast"
	PresenceAction string                    `json:"presence_action,omitempty"`  // for "presence"
	Presence       map[string]any            `json:"presence,omitempty"`         // for "presence"
	Presences      map[string]map[string]any `json:"presences,omitempty"`        // for "presence_sync"
	PresenceConnID string                    `json:"presence_conn_id,omitempty"` // for "presence"
}

// parseClientMessage parses a raw JSON message from the client.
// Returns an error for malformed JSON or unknown type values.
func parseClientMessage(data []byte) (ClientMessage, error) {
	var msg ClientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return ClientMessage{}, fmt.Errorf("invalid JSON: %w", err)
	}
	switch msg.Type {
	case MsgTypeAuth, MsgTypeSubscribe, MsgTypeUnsubscribe, MsgTypeChannelSubscribe, MsgTypeChannelUnsubscribe, MsgTypeBroadcast, MsgTypePresenceTrack, MsgTypePresenceUntrack, MsgTypePresenceSync:
		return msg, nil
	case "":
		return ClientMessage{}, fmt.Errorf("missing message type")
	default:
		return ClientMessage{}, fmt.Errorf("unknown message type: %s", msg.Type)
	}
}

// replyOK creates a success reply with the given ref.
func replyOK(ref string) ServerMessage {
	return ServerMessage{Type: MsgTypeReply, Ref: ref, Status: "ok"}
}

// replyError creates an error reply with the given ref and message.
func replyError(ref, message string) ServerMessage {
	return ServerMessage{Type: MsgTypeReply, Ref: ref, Status: "error", Message: message}
}

// errorMsg creates a top-level error message.
func errorMsg(message string) ServerMessage {
	return ServerMessage{Type: MsgTypeError, Message: message}
}

// connectedMsg creates the initial connected message.
func connectedMsg(clientID string) ServerMessage {
	return ServerMessage{Type: MsgTypeConnected, ClientID: clientID}
}

// EventMsg creates an event message.
func EventMsg(action, table string, record map[string]any) ServerMessage {
	return ServerMessage{Type: MsgTypeEvent, Action: action, Table: table, Record: record}
}

// BroadcastMsg creates a broadcast relay message.
func BroadcastMsg(channel, event string, payload map[string]any) ServerMessage {
	return ServerMessage{Type: MsgTypeBroadcast, Channel: channel, Event: event, Payload: payload}
}

const (
	PresenceActionJoin   = "join"
	PresenceActionUpdate = "update"
	PresenceActionLeave  = "leave"
	PresenceActionSync   = "sync"
)

// PresenceDiffMsg creates a presence join/update/leave message.
func PresenceDiffMsg(channel, action, connID string, presence map[string]any) ServerMessage {
	return ServerMessage{
		Type:           MsgTypePresence,
		Channel:        channel,
		PresenceAction: action,
		PresenceConnID: connID,
		Presence:       presence,
	}
}

// PresenceSyncMsg creates a full presence sync message.
func PresenceSyncMsg(channel string, presences map[string]map[string]any) ServerMessage {
	return ServerMessage{
		Type:           MsgTypePresence,
		Channel:        channel,
		PresenceAction: PresenceActionSync,
		Presences:      presences,
	}
}
