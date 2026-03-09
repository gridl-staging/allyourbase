package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/testutil"
)

type mockLister struct {
	hooks []Webhook
	err   error
}

func (m *mockLister) ListEnabled(_ context.Context) ([]Webhook, error) {
	return m.hooks, m.err
}

// fastBackoff is used by test dispatchers — short delays, no global mutation.
var fastBackoff = [maxRetries]time.Duration{1 * time.Millisecond, 1 * time.Millisecond, 1 * time.Millisecond}

func testDispatcher(lister WebhookLister) *Dispatcher {
	d := &Dispatcher{
		store:   lister,
		client:  &http.Client{Timeout: 2 * time.Second},
		logger:  testutil.DiscardLogger(),
		queue:   make(chan *realtime.Event, queueSize),
		done:    make(chan struct{}),
		backoff: fastBackoff, // per-instance fast retries — safe to run in parallel
	}
	// Don't start background worker — we call processEvent directly in tests.
	return d
}

func TestSign(t *testing.T) {
	t.Parallel()
	sig := Sign("my-secret", []byte(`{"action":"create","table":"posts","record":{"id":1}}`))
	// Pre-computed with: echo -n '{"action":"create","table":"posts","record":{"id":1}}' | openssl dgst -sha256 -hmac 'my-secret'
	testutil.Equal(t, "d09b0b97b9e912a5c0de9bd1eb4714617c7cc1b7a52e656384e76a469b4584bd", sig)
}

func TestDeliverSuccess(t *testing.T) {
	t.Parallel()
	var received atomic.Int32
	var body []byte
	var sigHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ = io.ReadAll(r.Body)
		sigHeader = r.Header.Get("X-AYB-Signature")
		testutil.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(200)
	}))
	defer srv.Close()

	lister := &mockLister{hooks: []Webhook{{
		ID: "wh1", URL: srv.URL, Events: []string{"create"}, Tables: []string{"posts"}, Enabled: true,
	}}}
	d := testDispatcher(lister)

	event := &realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": float64(1)}}
	d.processEvent(event)

	testutil.Equal(t, int32(1), received.Load())

	var got realtime.Event
	testutil.NoError(t, json.Unmarshal(body, &got))
	testutil.Equal(t, "create", got.Action)
	testutil.Equal(t, "posts", got.Table)

	// No secret configured → signature header must be absent.
	testutil.Equal(t, "", sigHeader)
}

func TestDeliverWithSignature(t *testing.T) {
	t.Parallel()
	var sigHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader = r.Header.Get("X-AYB-Signature")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	lister := &mockLister{hooks: []Webhook{{
		ID: "wh1", URL: srv.URL, Secret: "test-secret", Events: []string{"create"}, Tables: []string{}, Enabled: true,
	}}}
	d := testDispatcher(lister)

	event := &realtime.Event{Action: "create", Table: "posts", Record: map[string]any{"id": float64(1)}}
	payload, _ := json.Marshal(event)
	d.processEvent(event)

	testutil.True(t, sigHeader != "", "X-AYB-Signature header should be set")
	testutil.Equal(t, Sign("test-secret", payload), sigHeader)
}

func TestDeliverRetryOn500(t *testing.T) {
	// testDispatcher uses fastBackoff — no global mutation, safe to run in parallel.
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	lister := &mockLister{hooks: []Webhook{{
		ID: "wh1", URL: srv.URL, Events: []string{"create"}, Enabled: true,
	}}}
	d := testDispatcher(lister)

	d.processEvent(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{}})
	testutil.Equal(t, int32(3), attempts.Load())
}

func TestDeliverExhaustsRetries(t *testing.T) {
	// testDispatcher uses fastBackoff — no global mutation, safe to run in parallel.
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	lister := &mockLister{hooks: []Webhook{{
		ID: "wh1", URL: srv.URL, Events: []string{"create"}, Enabled: true,
	}}}
	d := testDispatcher(lister)

	d.processEvent(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{}})
	testutil.Equal(t, int32(maxRetries), attempts.Load())
}

func TestEventFilteringByTable(t *testing.T) {
	t.Parallel()
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	lister := &mockLister{hooks: []Webhook{{
		ID: "wh1", URL: srv.URL, Events: []string{"create", "update", "delete"}, Tables: []string{"posts"}, Enabled: true,
	}}}
	d := testDispatcher(lister)

	// Should match.
	d.processEvent(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{}})
	testutil.Equal(t, int32(1), received.Load())

	// Should NOT match — wrong table.
	d.processEvent(&realtime.Event{Action: "create", Table: "comments", Record: map[string]any{}})
	testutil.Equal(t, int32(1), received.Load())
}

func TestEventFilteringByAction(t *testing.T) {
	t.Parallel()
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	lister := &mockLister{hooks: []Webhook{{
		ID: "wh1", URL: srv.URL, Events: []string{"delete"}, Tables: []string{}, Enabled: true,
	}}}
	d := testDispatcher(lister)

	// Should NOT match — wrong action.
	d.processEvent(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{}})
	testutil.Equal(t, int32(0), received.Load())

	// Should match.
	d.processEvent(&realtime.Event{Action: "delete", Table: "posts", Record: map[string]any{}})
	testutil.Equal(t, int32(1), received.Load())
}

func TestEventFilteringWildcardTables(t *testing.T) {
	t.Parallel()
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Empty tables = all tables; explicit events list.
	lister := &mockLister{hooks: []Webhook{{
		ID: "wh1", URL: srv.URL, Events: []string{"create", "update", "delete"}, Tables: []string{}, Enabled: true,
	}}}
	d := testDispatcher(lister)

	d.processEvent(&realtime.Event{Action: "create", Table: "anything", Record: map[string]any{}})
	d.processEvent(&realtime.Event{Action: "update", Table: "something_else", Record: map[string]any{}})
	testutil.Equal(t, int32(2), received.Load())
}

func TestEventFilteringWildcardEvents(t *testing.T) {
	t.Parallel()
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// nil Events = all actions (true wildcard for events).
	lister := &mockLister{hooks: []Webhook{{
		ID: "wh1", URL: srv.URL, Events: nil, Tables: nil, Enabled: true,
	}}}
	d := testDispatcher(lister)

	d.processEvent(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{}})
	d.processEvent(&realtime.Event{Action: "delete", Table: "users", Record: map[string]any{}})
	testutil.Equal(t, int32(2), received.Load())
}

func TestDeliverMultipleWebhooks(t *testing.T) {
	t.Parallel()
	var countA, countB atomic.Int32
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		countA.Add(1)
		w.WriteHeader(200)
	}))
	defer srvA.Close()
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		countB.Add(1)
		w.WriteHeader(200)
	}))
	defer srvB.Close()

	lister := &mockLister{hooks: []Webhook{
		{ID: "wh1", URL: srvA.URL, Events: []string{"create"}, Tables: []string{}, Enabled: true},
		{ID: "wh2", URL: srvB.URL, Events: []string{"create"}, Tables: []string{}, Enabled: true},
	}}
	d := testDispatcher(lister)

	d.processEvent(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{}})
	testutil.Equal(t, int32(1), countA.Load())
	testutil.Equal(t, int32(1), countB.Load())
}

func TestDeliverConnectionError(t *testing.T) {
	// testDispatcher uses fastBackoff — no global mutation, safe to run in parallel.
	t.Parallel()

	// Use a server that we immediately close — connection will be refused.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	lister := &mockLister{hooks: []Webhook{{
		ID: "wh1", URL: url, Events: []string{"create"}, Tables: []string{}, Enabled: true,
	}}}
	d := testDispatcher(lister)

	// Add a delivery store so we can verify delivery records.
	ds := newMockDeliveryStore()
	d.deliveryS = ds

	d.processEvent(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{}})

	// All retries should be recorded as failed deliveries.
	testutil.Equal(t, maxRetries, len(ds.deliveries))
	for _, del := range ds.deliveries {
		testutil.Equal(t, false, del.Success)
		testutil.Equal(t, 0, del.StatusCode)
		testutil.Equal(t, "wh1", del.WebhookID)
		testutil.True(t, del.Error != "", "error message should be recorded")
		testutil.Contains(t, del.Error, "connect")
	}
}

func TestListEnabledErrorSkipsDelivery(t *testing.T) {
	t.Parallel()
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	lister := &mockLister{
		hooks: []Webhook{{ID: "wh1", URL: srv.URL, Events: nil, Enabled: true}},
		err:   fmt.Errorf("database connection lost"),
	}
	d := testDispatcher(lister)

	d.processEvent(&realtime.Event{Action: "create", Table: "posts", Record: map[string]any{}})
	testutil.Equal(t, int32(0), received.Load())
}

func TestEnqueueNonBlocking(t *testing.T) {
	t.Parallel()
	d := &Dispatcher{
		logger: testutil.DiscardLogger(),
		queue:  make(chan *realtime.Event, 2), // small buffer
		done:   make(chan struct{}),
	}

	event := &realtime.Event{Action: "create", Table: "posts", Record: map[string]any{}}

	// Fill the buffer.
	d.Enqueue(event)
	d.Enqueue(event)

	// This should not block — it drops.
	d.Enqueue(event)
	d.Enqueue(event)

	testutil.Equal(t, 2, len(d.queue))
}
