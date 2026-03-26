package realtime

import (
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"log/slog"
)

// TestFilterIntegration tests the complete filter flow:
// - Subscription with filter
// - INSERT match/non-match
// - UPDATE transition behavior (enter/leave filter)
func TestFilterIntegration(t *testing.T) {
	logger := slog.Default()
	hub := NewHub(logger)

	// Subscribe with a filter: status=pending
	filters, err := ParseFilters("status=eq.pending")
	testutil.NoError(t, err)

	client := hub.SubscribeWithFilter(map[string]bool{"orders": true}, filters)
	defer hub.Unsubscribe(client.ID)

	// Helper to receive and check if event passes filter
	receiveFiltered := func() (*Event, bool) {
		select {
		case e := <-client.Events():
			// Apply the same filter logic as the SSE/WS handlers
			if shouldDeliverEvent(e, filters) {
				return e, true
			}
			return nil, false
		case <-time.After(100 * time.Millisecond):
			return nil, false
		}
	}

	// Test 1: INSERT that matches filter
	t.Run("INSERT match", func(t *testing.T) {
		event := &Event{
			Action: "create",
			Table:  "orders",
			Record: map[string]any{"id": 1, "status": "pending"},
		}
		hub.Publish(event)

		e, ok := receiveFiltered()
		testutil.True(t, ok, "expected to receive matching INSERT event")
		testutil.Equal(t, "create", e.Action)
		testutil.Equal(t, "orders", e.Table)
	})

	// Test 2: INSERT that doesn't match filter
	t.Run("INSERT no match", func(t *testing.T) {
		event := &Event{
			Action: "create",
			Table:  "orders",
			Record: map[string]any{"id": 2, "status": "completed"},
		}
		hub.Publish(event)

		_, ok := receiveFiltered()
		testutil.False(t, ok, "should not receive non-matching event")
	})

	// Test 3: UPDATE that enters filter (status: completed -> pending)
	t.Run("UPDATE enters filter", func(t *testing.T) {
		event := &Event{
			Action:    "update",
			Table:     "orders",
			Record:    map[string]any{"id": 3, "status": "pending"},
			OldRecord: map[string]any{"id": 3, "status": "completed"},
		}
		hub.Publish(event)

		e, ok := receiveFiltered()
		testutil.True(t, ok, "expected to receive UPDATE event that entered filter")
		testutil.Equal(t, "update", e.Action)
	})

	// Test 4: UPDATE that leaves filter (status: pending -> completed)
	t.Run("UPDATE leaves filter", func(t *testing.T) {
		event := &Event{
			Action:    "update",
			Table:     "orders",
			Record:    map[string]any{"id": 4, "status": "completed"},
			OldRecord: map[string]any{"id": 4, "status": "pending"},
		}
		hub.Publish(event)

		e, ok := receiveFiltered()
		testutil.True(t, ok, "expected to receive UPDATE event that left filter")
		testutil.Equal(t, "update", e.Action)
	})

	// Test 5: UPDATE that stays in filter
	t.Run("UPDATE stays in filter", func(t *testing.T) {
		event := &Event{
			Action:    "update",
			Table:     "orders",
			Record:    map[string]any{"id": 5, "status": "pending", "priority": 2},
			OldRecord: map[string]any{"id": 5, "status": "pending", "priority": 1},
		}
		hub.Publish(event)

		e, ok := receiveFiltered()
		testutil.True(t, ok, "expected to receive UPDATE event that stayed in filter")
		testutil.Equal(t, "update", e.Action)
	})

	// Test 6: UPDATE that stays out of filter
	t.Run("UPDATE stays out of filter", func(t *testing.T) {
		event := &Event{
			Action:    "update",
			Table:     "orders",
			Record:    map[string]any{"id": 6, "status": "completed", "note": "done"},
			OldRecord: map[string]any{"id": 6, "status": "completed"},
		}
		hub.Publish(event)

		_, ok := receiveFiltered()
		testutil.False(t, ok, "should not receive event that stayed out of filter")
	})

	// Test 7: DELETE that matches filter
	t.Run("DELETE match", func(t *testing.T) {
		event := &Event{
			Action:    "delete",
			Table:     "orders",
			Record:    map[string]any{"id": 7},
			OldRecord: map[string]any{"id": 7, "status": "pending"},
		}
		hub.Publish(event)

		e, ok := receiveFiltered()
		testutil.True(t, ok, "expected to receive DELETE event that matched filter")
		testutil.Equal(t, "delete", e.Action)
	})

	// Test 8: DELETE that doesn't match filter
	t.Run("DELETE no match", func(t *testing.T) {
		event := &Event{
			Action:    "delete",
			Table:     "orders",
			Record:    map[string]any{"id": 8},
			OldRecord: map[string]any{"id": 8, "status": "completed"},
		}
		hub.Publish(event)

		_, ok := receiveFiltered()
		testutil.False(t, ok, "should not receive DELETE event that didn't match filter")
	})
}

// TestMultiFilterIntegration tests multiple filters with AND semantics.
func TestMultiFilterIntegration(t *testing.T) {
	logger := slog.Default()
	hub := NewHub(logger)

	// Subscribe with multiple filters: status=pending AND priority>5
	filters, err := ParseFilters("status=eq.pending,priority=gt.5")
	testutil.NoError(t, err)

	client := hub.SubscribeWithFilter(map[string]bool{"tasks": true}, filters)
	defer hub.Unsubscribe(client.ID)

	receiveFiltered := func() (*Event, bool) {
		select {
		case e := <-client.Events():
			if shouldDeliverEvent(e, filters) {
				return e, true
			}
			return nil, false
		case <-time.After(100 * time.Millisecond):
			return nil, false
		}
	}

	// Test: both filters match
	event1 := &Event{
		Action: "create",
		Table:  "tasks",
		Record: map[string]any{"id": 1, "status": "pending", "priority": 6},
	}
	hub.Publish(event1)

	e, ok := receiveFiltered()
	testutil.True(t, ok, "expected event matching both filters")
	testutil.Equal(t, "create", e.Action)

	// Test: only first filter matches
	event2 := &Event{
		Action: "create",
		Table:  "tasks",
		Record: map[string]any{"id": 2, "status": "pending", "priority": 3},
	}
	hub.Publish(event2)

	_, ok = receiveFiltered()
	testutil.False(t, ok, "should not receive event matching only first filter")

	// Test: only second filter matches
	event3 := &Event{
		Action: "create",
		Table:  "tasks",
		Record: map[string]any{"id": 3, "status": "done", "priority": 7},
	}
	hub.Publish(event3)

	_, ok = receiveFiltered()
	testutil.False(t, ok, "should not receive event matching only second filter")
}

// TestInOperatorIntegration tests the `in` operator.
func TestInOperatorIntegration(t *testing.T) {
	logger := slog.Default()
	hub := NewHub(logger)

	// Subscribe with in filter: status in (pending, active, review)
	filters, err := ParseFilters("status=in.pending|active|review")
	testutil.NoError(t, err)

	client := hub.SubscribeWithFilter(map[string]bool{"issues": true}, filters)
	defer hub.Unsubscribe(client.ID)

	receiveFiltered := func() (*Event, bool) {
		select {
		case e := <-client.Events():
			if shouldDeliverEvent(e, filters) {
				return e, true
			}
			return nil, false
		case <-time.After(100 * time.Millisecond):
			return nil, false
		}
	}

	// Test: status in list
	for _, status := range []string{"pending", "active", "review"} {
		event := &Event{
			Action: "create",
			Table:  "issues",
			Record: map[string]any{"id": status, "status": status},
		}
		hub.Publish(event)

		e, ok := receiveFiltered()
		testutil.True(t, ok, "expected event with status=%s", status)
		testutil.Equal(t, status, e.Record["status"].(string))
	}

	// Test: status not in list
	event := &Event{
		Action: "create",
		Table:  "issues",
		Record: map[string]any{"id": "closed", "status": "closed"},
	}
	hub.Publish(event)

	_, ok := receiveFiltered()
	testutil.False(t, ok, "should not receive event with status=closed")
}

// TestUnfilteredSubscriptionNoRegression ensures subscriptions without filters
// continue to work as before (all events delivered).
func TestUnfilteredSubscriptionNoRegression(t *testing.T) {
	logger := slog.Default()
	hub := NewHub(logger)

	// Subscribe WITHOUT filter
	client := hub.Subscribe(map[string]bool{"orders": true})
	defer hub.Unsubscribe(client.ID)

	// All events should be delivered
	for i := 0; i < 3; i++ {
		event := &Event{
			Action: "create",
			Table:  "orders",
			Record: map[string]any{"id": i, "status": "any"},
		}
		hub.Publish(event)

		select {
		case e := <-client.Events():
			testutil.Equal(t, i, int(e.Record["id"].(int)))
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("expected event %d", i)
		}
	}
}
