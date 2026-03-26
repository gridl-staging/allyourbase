package ws

import (
	"log/slog"
	"testing"
	"time"
)

func TestPresenceHub_TrackJoinAndStore(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default())
	c := newBroadcastTestConn("c1")
	diff, err := hub.Track("room1", c, map[string]any{"user": "alice"})
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	if diff.Action != PresenceActionJoin {
		t.Fatalf("expected join action, got %+v", diff)
	}
	if got := hub.Sync("room1"); got["c1"]["user"] != "alice" {
		t.Fatalf("unexpected sync snapshot: %+v", got)
	}
}

func TestPresenceHub_TrackUpdate(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default())
	c := newBroadcastTestConn("c1")
	_, _ = hub.Track("room1", c, map[string]any{"status": "typing"})
	diff, err := hub.Track("room1", c, map[string]any{"status": "idle"})
	if err != nil {
		t.Fatalf("track update: %v", err)
	}
	if diff.Action != PresenceActionUpdate {
		t.Fatalf("expected update action, got %+v", diff)
	}
}

func TestPresenceHub_UntrackLeave(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default())
	c := newBroadcastTestConn("c1")
	_, _ = hub.Track("room1", c, map[string]any{"user": "alice"})
	diff := hub.Untrack("room1", c)
	if diff.Action != PresenceActionLeave {
		t.Fatalf("expected leave action, got %+v", diff)
	}
	if got := hub.Sync("room1"); len(got) != 0 {
		t.Fatalf("expected empty snapshot after untrack, got %+v", got)
	}
}

func TestPresenceHub_SyncReturnsCopy(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default())
	c := newBroadcastTestConn("c1")
	_, _ = hub.Track("room1", c, map[string]any{"user": "alice"})
	snap := hub.Sync("room1")
	snap["c1"]["user"] = "mutated"
	snap2 := hub.Sync("room1")
	if snap2["c1"]["user"] != "alice" {
		t.Fatalf("presence snapshot was not copied: %+v", snap2)
	}
}

func TestPresenceHub_UntrackAll(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default())
	c := newBroadcastTestConn("c1")
	_, _ = hub.Track("room1", c, map[string]any{"user": "alice"})
	_, _ = hub.Track("room2", c, map[string]any{"user": "alice"})
	_, _ = hub.Track("room3", c, map[string]any{"user": "alice"})
	diffs := hub.UntrackAll(c)
	if len(diffs) != 3 {
		t.Fatalf("expected 3 leave diffs, got %d", len(diffs))
	}
	if len(hub.Sync("room1")) != 0 || len(hub.Sync("room2")) != 0 || len(hub.Sync("room3")) != 0 {
		t.Fatal("expected all channels to be empty after untrack all")
	}
}

func TestPresenceHub_DeferredUntrackAllEmitsLeaveAfterTimeout(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default(), PresenceHubOptions{LeaveTimeout: 20 * time.Millisecond})
	c := newBroadcastTestConn("c1")
	_, _ = hub.Track("room1", c, map[string]any{"user": "alice"})

	leave := make(chan PresenceDiff, 1)
	hub.DeferredUntrackAll(c, func(diff PresenceDiff) {
		leave <- diff
	})

	select {
	case diff := <-leave:
		if diff.Action != PresenceActionLeave {
			t.Fatalf("expected leave diff, got %+v", diff)
		}
		if diff.ConnID != "c1" || diff.Channel != "room1" {
			t.Fatalf("unexpected leave diff: %+v", diff)
		}
		if diff.Presence["user"] != "alice" {
			t.Fatalf("unexpected presence payload: %+v", diff.Presence)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected leave diff after timeout")
	}

	hub.mu.RLock()
	pending := len(hub.pendingLeaves)
	hub.mu.RUnlock()
	if pending != 0 {
		t.Fatalf("expected no pending leaves after timer fired, got %d", pending)
	}
}

func TestPresenceHub_DeferredUntrackAllReTrackCancelsPendingLeave(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default(), PresenceHubOptions{LeaveTimeout: 30 * time.Millisecond})
	c := newBroadcastTestConn("c1")
	_, _ = hub.Track("room1", c, map[string]any{"user": "alice", "status": "online"})

	leave := make(chan PresenceDiff, 1)
	hub.DeferredUntrackAll(c, func(diff PresenceDiff) {
		leave <- diff
	})

	_, err := hub.Track("room1", c, map[string]any{"user": "alice", "status": "away"})
	if err != nil {
		t.Fatalf("track: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	select {
	case diff := <-leave:
		t.Fatalf("unexpected leave diff after re-track: %+v", diff)
	default:
	}

	snapshot := hub.Sync("room1")
	if snapshot["c1"]["status"] != "away" {
		t.Fatalf("unexpected snapshot after re-track: %+v", snapshot)
	}
}

func TestPresenceHub_DeferredUntrackAllImmediateWhenLeaveTimeoutZero(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default())
	hub.LeaveTimeout = 0
	c := newBroadcastTestConn("c1")
	_, _ = hub.Track("room1", c, map[string]any{"user": "alice"})

	leave := make(chan PresenceDiff, 1)
	hub.DeferredUntrackAll(c, func(diff PresenceDiff) {
		leave <- diff
	})

	select {
	case diff := <-leave:
		if diff.Action != PresenceActionLeave {
			t.Fatalf("expected leave diff, got %+v", diff)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected immediate leave when timeout <= 0")
	}
}

func TestPresenceHub_PayloadSizeLimit(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default(), PresenceHubOptions{MaxPayloadBytes: 8})
	c := newBroadcastTestConn("c1")
	if _, err := hub.Track("room1", c, map[string]any{"value": "this is too long"}); err == nil {
		t.Fatal("expected payload size error")
	}
}

func TestPresenceHub_TrackUpdatesUpdatedAtForLWW(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default())
	c := newBroadcastTestConn("c1")
	_, _ = hub.Track("room1", c, map[string]any{"status": "online"})

	hub.mu.RLock()
	entry1 := hub.channels["room1"]["c1"]
	hub.mu.RUnlock()
	time.Sleep(2 * time.Millisecond)
	diff, err := hub.Track("room1", c, map[string]any{"status": "away"})
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	if diff.Action != PresenceActionUpdate {
		t.Fatalf("expected update action, got %+v", diff)
	}

	hub.mu.RLock()
	entry2 := hub.channels["room1"]["c1"]
	hub.mu.RUnlock()
	if !entry2.updatedAt.After(entry1.updatedAt) {
		t.Fatalf("expected updatedAt to move forward, got first=%v second=%v", entry1.updatedAt, entry2.updatedAt)
	}

	snap := hub.Sync("room1")
	if snap["c1"]["status"] != "away" {
		t.Fatalf("expected latest payload, got %+v", snap)
	}
}

func TestPresenceHub_EmptyChannelNoop(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default())
	c := newBroadcastTestConn("c1")
	diff := hub.Untrack("missing", c)
	if diff.Action != "" {
		t.Fatalf("expected empty diff for missing channel, got %+v", diff)
	}
}

func TestPresenceHub_TrackPreservesSingleEntryPerConnID(t *testing.T) {
	t.Parallel()
	hub := NewPresenceHub(slog.Default())
	c := newBroadcastTestConn("c1")
	_, _ = hub.Track("room1", c, map[string]any{"user": "alice"})
	_, _ = hub.Track("room1", c, map[string]any{"user": "alice", "status": "idle"})
	sync := hub.Sync("room1")
	if len(sync) != 1 {
		t.Fatalf("expected one presence row per conn, got %d", len(sync))
	}
	if sync["c1"]["status"] != "idle" {
		t.Fatalf("expected latest payload to win: %+v", sync)
	}
}
