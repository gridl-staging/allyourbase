package realtime_test

import (
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSubscribeAndPublish(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	testutil.Equal(t, 1, hub.ClientCount())
	testutil.True(t, client.ID != "", "client should have an ID")

	hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "posts",
		Record: map[string]any{"id": 1, "title": "Hello"},
	})

	select {
	case event := <-client.Events():
		testutil.Equal(t, "create", event.Action)
		testutil.Equal(t, "posts", event.Table)
		testutil.Equal(t, "Hello", event.Record["title"])
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestPublishToNonSubscribedTable(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	hub.Publish(&realtime.Event{
		Action: "create",
		Table:  "comments",
		Record: map[string]any{"id": 1},
	})

	select {
	case <-client.Events():
		t.Fatal("should not receive event for unsubscribed table")
	case <-time.After(10 * time.Millisecond):
		// Expected: no event received.
	}
}

func TestUnsubscribeRemovesClient(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	testutil.Equal(t, 1, hub.ClientCount())

	hub.Unsubscribe(client.ID)
	testutil.Equal(t, 0, hub.ClientCount())

	// Channel should be closed.
	_, ok := <-client.Events()
	testutil.False(t, ok, "channel should be closed after unsubscribe")
}

func TestUnsubscribeIdempotent(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	hub.Unsubscribe(client.ID)
	hub.Unsubscribe(client.ID) // Should not panic.
	testutil.Equal(t, 0, hub.ClientCount())
}

func TestMultipleClients(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	c1 := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(c1.ID)
	c2 := hub.Subscribe(map[string]bool{"posts": true, "comments": true})
	defer hub.Unsubscribe(c2.ID)
	c3 := hub.Subscribe(map[string]bool{"comments": true})
	defer hub.Unsubscribe(c3.ID)

	testutil.Equal(t, 3, hub.ClientCount())

	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})

	// c1 and c2 subscribed to posts.
	for _, c := range []*realtime.Client{c1, c2} {
		select {
		case event := <-c.Events():
			testutil.Equal(t, "posts", event.Table)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("client %s should receive posts event", c.ID)
		}
	}

	// c3 not subscribed to posts.
	select {
	case <-c3.Events():
		t.Fatal("c3 should not receive posts event")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestPublishMultipleActions(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	events := []realtime.Event{
		{Action: "create", Table: "posts", Record: map[string]any{"id": 1}},
		{Action: "update", Table: "posts", Record: map[string]any{"id": 1, "title": "Updated"}},
		{Action: "delete", Table: "posts", Record: map[string]any{"id": 1}},
	}

	for i := range events {
		hub.Publish(&events[i])
	}

	for _, want := range events {
		select {
		case got := <-client.Events():
			testutil.Equal(t, want.Action, got.Action)
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timed out waiting for %s event", want.Action)
		}
	}
}

func TestClientIDsAreUnique(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	c1 := hub.Subscribe(map[string]bool{"posts": true})
	c2 := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(c1.ID)
	defer hub.Unsubscribe(c2.ID)

	testutil.NotEqual(t, c1.ID, c2.ID)
}

func TestPublishNoClientsIsNoop(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	// Should not panic.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 1}})
}

func TestBufferFullDropsEvent(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	// Fill the 256-event buffer.
	for i := 0; i < 256; i++ {
		hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": i}})
	}

	// The 257th event should be dropped (non-blocking), not block the publisher.
	hub.Publish(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": 257}})

	// Drain and verify we got exactly 256 events.
	count := 0
	for count < 256 {
		select {
		case <-client.Events():
			count++
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("expected 256 events, got %d", count)
		}
	}

	// Channel should be empty now (the 257th was dropped).
	select {
	case <-client.Events():
		t.Fatal("should not receive the dropped event")
	case <-time.After(10 * time.Millisecond):
		// Expected.
	}
}

func TestCloseDisconnectsAllClients(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	c1 := hub.Subscribe(map[string]bool{"posts": true})
	c2 := hub.Subscribe(map[string]bool{"comments": true})
	testutil.Equal(t, 2, hub.ClientCount())

	hub.Close()
	testutil.Equal(t, 0, hub.ClientCount())

	// Both client channels should be closed.
	_, ok1 := <-c1.Events()
	testutil.False(t, ok1, "c1 channel should be closed")
	_, ok2 := <-c2.Events()
	testutil.False(t, ok2, "c2 channel should be closed")
}

func TestCloseIdempotent(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	hub.Subscribe(map[string]bool{"posts": true})
	hub.Close()
	hub.Close() // Should not panic.
	testutil.Equal(t, 0, hub.ClientCount())
}

// --- OAuth Hub Tests ---

func TestSubscribeOAuthCreatesClient(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.SubscribeOAuth()
	defer hub.Unsubscribe(client.ID)

	testutil.True(t, client.ID != "", "oauth client should have an ID")
	testutil.Equal(t, 1, hub.ClientCount())
	testutil.NotNil(t, client.OAuthEvents())
}

func TestSubscribeOAuthUniqueIDs(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	c1 := hub.SubscribeOAuth()
	c2 := hub.SubscribeOAuth()
	defer hub.Unsubscribe(c1.ID)
	defer hub.Unsubscribe(c2.ID)

	testutil.NotEqual(t, c1.ID, c2.ID)
}

func TestHasClientReturnsTrue(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.SubscribeOAuth()
	defer hub.Unsubscribe(client.ID)

	testutil.True(t, hub.HasClient(client.ID), "HasClient should return true for connected client")
}

func TestHasClientReturnsFalse(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	testutil.False(t, hub.HasClient("nonexistent"), "HasClient should return false for unknown client")
}

func TestHasClientAfterUnsubscribe(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.SubscribeOAuth()
	hub.Unsubscribe(client.ID)

	testutil.False(t, hub.HasClient(client.ID), "HasClient should return false after unsubscribe")
}

func TestPublishOAuthTargetedDelivery(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	c1 := hub.SubscribeOAuth()
	c2 := hub.SubscribeOAuth()
	defer hub.Unsubscribe(c1.ID)
	defer hub.Unsubscribe(c2.ID)

	event := &auth.OAuthEvent{
		Token:        "test-token",
		RefreshToken: "test-refresh",
	}
	hub.PublishOAuth(c1.ID, event)

	// c1 should receive the event.
	select {
	case got := <-c1.OAuthEvents():
		testutil.Equal(t, "test-token", got.Token)
		testutil.Equal(t, "test-refresh", got.RefreshToken)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("c1 should receive oauth event")
	}

	// c2 should NOT receive anything.
	select {
	case <-c2.OAuthEvents():
		t.Fatal("c2 should not receive c1's oauth event")
	case <-time.After(10 * time.Millisecond):
		// Expected.
	}
}

func TestPublishOAuthWithError(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.SubscribeOAuth()
	defer hub.Unsubscribe(client.ID)

	hub.PublishOAuth(client.ID, &auth.OAuthEvent{Error: "access denied"})

	select {
	case got := <-client.OAuthEvents():
		testutil.Equal(t, "access denied", got.Error)
		testutil.Equal(t, "", got.Token)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("should receive oauth error event")
	}
}

func TestPublishOAuthToUnknownClientIsNoop(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	// Should not panic.
	hub.PublishOAuth("nonexistent", &auth.OAuthEvent{Token: "tok"})
}

func TestPublishOAuthToNonOAuthClientIsNoop(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	// Subscribe a regular (non-OAuth) client.
	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	// Should not panic — the client has no oauthCh.
	hub.PublishOAuth(client.ID, &auth.OAuthEvent{Token: "tok"})
}

func TestOAuthClientUnsubscribeClosesChannels(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.SubscribeOAuth()
	hub.Unsubscribe(client.ID)

	// Both channels should be closed.
	_, eventsOk := <-client.Events()
	testutil.False(t, eventsOk, "events channel should be closed")

	_, oauthOk := <-client.OAuthEvents()
	testutil.False(t, oauthOk, "oauth channel should be closed")
}

func TestRegularClientHasNilOAuthEvents(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.Subscribe(map[string]bool{"posts": true})
	defer hub.Unsubscribe(client.ID)

	testutil.Nil(t, client.OAuthEvents())
}

func TestConcurrentSubscribeOAuthCreatesDistinctClients(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	// Subscribe 10 OAuth clients concurrently.
	const n = 10
	clients := make([]*realtime.Client, n)
	done := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(idx int) {
			clients[idx] = hub.SubscribeOAuth()
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}

	// All should have unique IDs.
	ids := make(map[string]bool)
	for _, c := range clients {
		testutil.True(t, c.ID != "", "client should have an ID")
		testutil.False(t, ids[c.ID], "all client IDs should be unique, got duplicate: "+c.ID)
		ids[c.ID] = true
	}
	testutil.Equal(t, n, hub.ClientCount())

	// Cleanup.
	for _, c := range clients {
		hub.Unsubscribe(c.ID)
	}
	testutil.Equal(t, 0, hub.ClientCount())
}

func TestPublishOAuthAfterClientDisconnectIsNoop(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.SubscribeOAuth()
	clientID := client.ID
	hub.Unsubscribe(clientID)

	// Should not panic and should be a no-op.
	hub.PublishOAuth(clientID, &auth.OAuthEvent{Token: "tok"})

	// Verify client is gone.
	testutil.False(t, hub.HasClient(clientID), "client should be removed after unsubscribe")
}

func TestPublishOAuthBufferFullDropsEvent(t *testing.T) {
	t.Parallel()
	hub := realtime.NewHub(testutil.DiscardLogger())

	client := hub.SubscribeOAuth()
	defer hub.Unsubscribe(client.ID)

	// OAuth channel has buffer size 1. Fill it.
	hub.PublishOAuth(client.ID, &auth.OAuthEvent{Token: "first"})

	// Second publish should be dropped (not block).
	hub.PublishOAuth(client.ID, &auth.OAuthEvent{Token: "second"})

	// Drain and verify we only get the first.
	select {
	case got := <-client.OAuthEvents():
		testutil.Equal(t, "first", got.Token)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("should receive first event")
	}

	// Channel should be empty now.
	select {
	case <-client.OAuthEvents():
		t.Fatal("should not receive the dropped event")
	case <-time.After(10 * time.Millisecond):
		// Expected.
	}
}
