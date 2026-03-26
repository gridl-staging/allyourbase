package ws

import (
	"encoding/json"
	"testing"
)

func TestParseClientMessage_Auth(t *testing.T) {
	t.Parallel()
	msg, err := parseClientMessage([]byte(`{"type":"auth","token":"my-jwt","ref":"1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != MsgTypeAuth {
		t.Fatalf("got type %q, want %q", msg.Type, MsgTypeAuth)
	}
	if msg.Token != "my-jwt" {
		t.Fatalf("got token %q, want %q", msg.Token, "my-jwt")
	}
	if msg.Ref != "1" {
		t.Fatalf("got ref %q, want %q", msg.Ref, "1")
	}
}

func TestParseClientMessage_Subscribe(t *testing.T) {
	t.Parallel()
	msg, err := parseClientMessage([]byte(`{"type":"subscribe","tables":["users","logs"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != MsgTypeSubscribe {
		t.Fatalf("got type %q, want %q", msg.Type, MsgTypeSubscribe)
	}
	if len(msg.Tables) != 2 || msg.Tables[0] != "users" || msg.Tables[1] != "logs" {
		t.Fatalf("got tables %v, want [users logs]", msg.Tables)
	}
}

func TestParseClientMessage_Unsubscribe(t *testing.T) {
	t.Parallel()
	msg, err := parseClientMessage([]byte(`{"type":"unsubscribe","tables":["logs"],"ref":"2"}`))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != MsgTypeUnsubscribe {
		t.Fatalf("got type %q, want %q", msg.Type, MsgTypeUnsubscribe)
	}
	if msg.Ref != "2" {
		t.Fatalf("got ref %q, want %q", msg.Ref, "2")
	}
}

func TestParseClientMessage_ChannelSubscribe(t *testing.T) {
	t.Parallel()
	msg, err := parseClientMessage([]byte(`{"type":"channel_subscribe","channel":"room1","ref":"r1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != MsgTypeChannelSubscribe {
		t.Fatalf("got type %q, want %q", msg.Type, MsgTypeChannelSubscribe)
	}
	if msg.Channel != "room1" {
		t.Fatalf("got channel %q, want room1", msg.Channel)
	}
}

func TestParseClientMessage_Broadcast(t *testing.T) {
	t.Parallel()
	msg, err := parseClientMessage([]byte(`{"type":"broadcast","channel":"room1","event":"move","payload":{"x":1},"self":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != MsgTypeBroadcast {
		t.Fatalf("got type %q, want %q", msg.Type, MsgTypeBroadcast)
	}
	if msg.Channel != "room1" || msg.Event != "move" || !msg.Self {
		t.Fatalf("unexpected message: %+v", msg)
	}
}

func TestParseClientMessage_UnknownType(t *testing.T) {
	t.Parallel()
	_, err := parseClientMessage([]byte(`{"type":"bogus"}`))
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestParseClientMessage_MissingType(t *testing.T) {
	t.Parallel()
	_, err := parseClientMessage([]byte(`{"token":"abc"}`))
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestParseClientMessage_MalformedJSON(t *testing.T) {
	t.Parallel()
	_, err := parseClientMessage([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestServerMessage_ConnectedJSON(t *testing.T) {
	t.Parallel()
	msg := connectedMsg("abc-123")
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["type"] != "connected" {
		t.Fatalf("got type %v, want connected", decoded["type"])
	}
	if decoded["client_id"] != "abc-123" {
		t.Fatalf("got client_id %v, want abc-123", decoded["client_id"])
	}
}

func TestServerMessage_ReplyOK(t *testing.T) {
	t.Parallel()
	msg := replyOK("ref-1")
	if msg.Status != "ok" || msg.Ref != "ref-1" {
		t.Fatalf("unexpected reply: %+v", msg)
	}
}

func TestServerMessage_ReplyError(t *testing.T) {
	t.Parallel()
	msg := replyError("ref-2", "bad token")
	if msg.Status != "error" || msg.Message != "bad token" || msg.Ref != "ref-2" {
		t.Fatalf("unexpected reply: %+v", msg)
	}
}

func TestServerMessage_EventJSON(t *testing.T) {
	t.Parallel()
	msg := EventMsg("create", "users", map[string]any{"id": 1, "name": "alice"})
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["type"] != "event" {
		t.Fatalf("got type %v, want event", decoded["type"])
	}
	if decoded["action"] != "create" {
		t.Fatalf("got action %v, want create", decoded["action"])
	}
	if decoded["table"] != "users" {
		t.Fatalf("got table %v, want users", decoded["table"])
	}
	rec := decoded["record"].(map[string]any)
	if rec["name"] != "alice" {
		t.Fatalf("got record.name %v, want alice", rec["name"])
	}
}

func TestServerMessage_OmitsEmptyFields(t *testing.T) {
	t.Parallel()
	msg := errorMsg("something broke")
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	// ref, client_id, status, action, table, record should all be omitted
	for _, key := range []string{"ref", "client_id", "status", "action", "table", "record", "channel", "event", "payload"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("expected %q to be omitted, but it was present", key)
		}
	}
}

func TestServerMessage_Broadcast(t *testing.T) {
	t.Parallel()
	msg := BroadcastMsg("room1", "move", map[string]any{"x": 1})
	if msg.Type != MsgTypeBroadcast || msg.Channel != "room1" || msg.Event != "move" {
		t.Fatalf("unexpected broadcast message: %+v", msg)
	}
}

func TestParseClientMessage_PresenceTrack(t *testing.T) {
	t.Parallel()
	msg, err := parseClientMessage([]byte(`{"type":"presence_track","channel":"room1","presence":{"user":"alice"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != MsgTypePresenceTrack || msg.Channel != "room1" {
		t.Fatalf("unexpected message: %+v", msg)
	}
}

func TestServerMessage_PresenceDiff(t *testing.T) {
	t.Parallel()
	msg := PresenceDiffMsg("room1", PresenceActionJoin, "c1", map[string]any{"user": "alice"})
	if msg.Type != MsgTypePresence || msg.Channel != "room1" || msg.PresenceAction != PresenceActionJoin {
		t.Fatalf("unexpected presence diff message: %+v", msg)
	}
}
