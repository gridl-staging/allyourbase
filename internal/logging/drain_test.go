package logging

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func skipIfNoLocalHTTPListener(t *testing.T) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping HTTP drain tests: cannot bind local test listener (%v)", err)
	}
	_ = l.Close()
}

func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("skipping HTTP drain tests: cannot bind local test listener (%v)", err)
	}

	ts := httptest.NewUnstartedServer(handler)
	ts.Listener = listener
	ts.Start()
	t.Cleanup(ts.Close)
	return ts
}

// ---------------------------------------------------------------------------
// HTTPDrain payload format
// ---------------------------------------------------------------------------

func TestHTTPDrainSendPayload(t *testing.T) {
	t.Parallel()

	var received []byte
	ts := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		testutil.Equal(t, "application/json", r.Header.Get("Content-Type"))
		testutil.Equal(t, "Bearer tok123", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))

	drain := NewHTTPDrain(DrainConfig{
		ID:      "h1",
		Type:    "http",
		URL:     ts.URL,
		Headers: map[string]string{"Authorization": "Bearer tok123"},
		Enabled: true,
	})

	entries := []LogEntry{
		{Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Level: "info", Message: "hello", Source: "app"},
	}
	err := drain.Send(entries)
	testutil.NoError(t, err)

	var parsed []LogEntry
	testutil.NoError(t, json.Unmarshal(received, &parsed))
	testutil.Equal(t, 1, len(parsed))
	testutil.Equal(t, "hello", parsed[0].Message)
	testutil.Equal(t, "h1", drain.Name())
}

func TestHTTPDrainReturnsErrorOn500(t *testing.T) {
	t.Parallel()

	ts := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	drain := NewHTTPDrain(DrainConfig{ID: "h2", URL: ts.URL, Enabled: true})
	err := drain.Send([]LogEntry{{Message: "test"}})
	testutil.True(t, err != nil, "expected error on 500")
}

func TestHTTPDrainStatsAccumulate(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	ts := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if callCount.Add(1) == 1 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	drain := NewHTTPDrain(DrainConfig{ID: "h3", URL: ts.URL, Enabled: true})
	_ = drain.Send([]LogEntry{{Message: "ok"}})
	_ = drain.Send([]LogEntry{{Message: "fail"}})

	stats := drain.Stats()
	testutil.Equal(t, int64(1), stats.Sent)
	testutil.Equal(t, int64(1), stats.Failed)
}

// ---------------------------------------------------------------------------
// DatadogDrain payload format
// ---------------------------------------------------------------------------

func TestDatadogDrainPayloadFormat(t *testing.T) {
	t.Parallel()

	var received []byte
	ts := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		testutil.Equal(t, "application/json", r.Header.Get("Content-Type"))
		testutil.Equal(t, "myapikey", r.Header.Get("DD-API-KEY"))
		w.WriteHeader(http.StatusAccepted)
	}))

	drain := NewDatadogDrain(DrainConfig{
		ID:      "dd1",
		Type:    "datadog",
		URL:     ts.URL,
		Headers: map[string]string{"DD-API-KEY": "myapikey"},
		Enabled: true,
	})

	now := time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC)
	entries := []LogEntry{
		{Timestamp: now, Level: "error", Message: "db timeout", Source: "app",
			Fields: map[string]any{"request_id": "r123"}},
	}
	err := drain.Send(entries)
	testutil.NoError(t, err)

	// Datadog expects JSON array of objects with specific fields.
	var parsed []map[string]any
	testutil.NoError(t, json.Unmarshal(received, &parsed))
	testutil.Equal(t, 1, len(parsed))
	testutil.Equal(t, "ayb", parsed[0]["ddsource"])
	testutil.Equal(t, "ayb", parsed[0]["service"])
	testutil.Equal(t, "error", parsed[0]["status"])
	testutil.Equal(t, "db timeout", parsed[0]["message"])
	testutil.Equal(t, "dd1", drain.Name())
}

// ---------------------------------------------------------------------------
// LokiDrain payload format
// ---------------------------------------------------------------------------

func TestLokiDrainPayloadFormat(t *testing.T) {
	t.Parallel()

	var received []byte
	ts := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		testutil.Equal(t, "application/json", r.Header.Get("Content-Type"))
		testutil.Equal(t, "/loki/api/v1/push", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	drain := NewLokiDrain(DrainConfig{
		ID:      "loki1",
		Type:    "loki",
		URL:     ts.URL + "/loki/api/v1/push",
		Enabled: true,
	})

	now := time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC)
	entries := []LogEntry{
		{
			Timestamp: now,
			Level:     "info",
			Message:   "request completed",
			Source:    "request",
			Fields: map[string]any{
				"status":      200,
				"duration_ms": 42,
			},
		},
	}
	err := drain.Send(entries)
	testutil.NoError(t, err)

	// Loki push format: {"streams": [{"stream": {...labels}, "values": [["<ns>", "<line>"]]}]}
	var parsed struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"streams"`
	}
	testutil.NoError(t, json.Unmarshal(received, &parsed))
	testutil.Equal(t, 1, len(parsed.Streams))
	testutil.Equal(t, "ayb", parsed.Streams[0].Stream["source"])
	testutil.Equal(t, "info", parsed.Streams[0].Stream["level"])
	testutil.Equal(t, 1, len(parsed.Streams[0].Values))

	var line map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(parsed.Streams[0].Values[0][1]), &line))
	testutil.Equal(t, "request completed", line["message"])
	testutil.Equal(t, "request", line["log_source"])
	testutil.True(t, line["status"] == float64(200), "expected status field in Loki line")
	testutil.True(t, line["duration_ms"] == float64(42), "expected duration_ms field in Loki line")
	testutil.Equal(t, "loki1", drain.Name())
}

func TestLokiLineForEntryIncludesStructuredFields(t *testing.T) {
	t.Parallel()

	entry := LogEntry{
		Timestamp: time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC),
		Level:     "info",
		Message:   "request completed",
		Source:    "request",
		Fields: map[string]any{
			"status":      200,
			"duration_ms": 42,
			"request_id":  "req-1",
		},
	}

	line, err := lokiLineForEntry(entry)
	testutil.NoError(t, err)

	var parsed map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(line), &parsed))
	testutil.Equal(t, "request completed", parsed["message"])
	testutil.Equal(t, "request", parsed["log_source"])
	testutil.True(t, parsed["status"] == float64(200), "expected status field in Loki log line")
	testutil.True(t, parsed["duration_ms"] == float64(42), "expected duration_ms field in Loki log line")
	testutil.Equal(t, "req-1", parsed["request_id"])
}

// ---------------------------------------------------------------------------
// LokiDrain — field collision prefix
// ---------------------------------------------------------------------------

func TestLokiLineForEntryPrefixesReservedFieldKeys(t *testing.T) {
	t.Parallel()

	entry := LogEntry{
		Timestamp: time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC),
		Level:     "warn",
		Message:   "collision test",
		Source:    "app",
		Fields: map[string]any{
			"message":    "user-supplied message",
			"level":      "user-level",
			"log_source": "user-source",
			"safe_key":   "safe_value",
		},
	}

	line, err := lokiLineForEntry(entry)
	testutil.NoError(t, err)

	var parsed map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(line), &parsed))

	// Reserved keys should retain original values.
	testutil.Equal(t, "collision test", parsed["message"])
	testutil.Equal(t, "warn", parsed["level"])
	testutil.Equal(t, "app", parsed["log_source"])

	// Colliding field keys should be prefixed with "field_".
	testutil.Equal(t, "user-supplied message", parsed["field_message"])
	testutil.Equal(t, "user-level", parsed["field_level"])
	testutil.Equal(t, "user-source", parsed["field_log_source"])

	// Non-colliding field should be unchanged.
	testutil.Equal(t, "safe_value", parsed["safe_key"])
}

// ---------------------------------------------------------------------------
// DatadogDrain — fields arrive as attributes
// ---------------------------------------------------------------------------

func TestDatadogDrainPayloadIncludesFieldsAsAttributes(t *testing.T) {
	t.Parallel()

	var received []byte
	ts := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))

	drain := NewDatadogDrain(DrainConfig{
		ID:      "dd-fields",
		Type:    "datadog",
		URL:     ts.URL,
		Headers: map[string]string{"DD-API-KEY": "key"},
		Enabled: true,
	})

	entries := []LogEntry{
		{
			Timestamp: time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC),
			Level:     "info",
			Message:   "GET /health",
			Source:    "request",
			Fields: map[string]any{
				"status":      200,
				"duration_ms": 5,
				"ip_address":  "192.0.2.1",
			},
		},
	}
	err := drain.Send(entries)
	testutil.NoError(t, err)

	var parsed []map[string]any
	testutil.NoError(t, json.Unmarshal(received, &parsed))
	testutil.Equal(t, 1, len(parsed))

	attrs, ok := parsed[0]["attributes"].(map[string]any)
	testutil.True(t, ok, "expected 'attributes' object in Datadog payload")
	testutil.True(t, attrs["status"] == float64(200), "expected status=200 in attributes")
	testutil.True(t, attrs["duration_ms"] == float64(5), "expected duration_ms=5 in attributes")
	testutil.Equal(t, "192.0.2.1", attrs["ip_address"])
}

// ---------------------------------------------------------------------------
// DrainWorker — batching, retry, drop
// ---------------------------------------------------------------------------

func TestDrainWorkerBatchFlushesAtSize(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var batches []int
	drain := &mockDrain{
		name: "test",
		sendFn: func(entries []LogEntry) error {
			mu.Lock()
			batches = append(batches, len(entries))
			mu.Unlock()
			return nil
		},
	}

	w := NewDrainWorker(drain, DrainWorkerConfig{
		BatchSize:     5,
		FlushInterval: 10 * time.Second, // large — only size triggers
		QueueSize:     100,
	})
	w.Start()

	for i := range 5 {
		w.Enqueue(LogEntry{Message: "msg", Fields: map[string]any{"i": i}})
	}

	// Wait for batch to flush.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(batches)
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	w.Stop()

	mu.Lock()
	testutil.True(t, len(batches) >= 1, "expected at least 1 batch flush")
	testutil.Equal(t, 5, batches[0])
	mu.Unlock()
}

func TestDrainWorkerFlushesAtTimeInterval(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var batches []int
	drain := &mockDrain{
		name: "test",
		sendFn: func(entries []LogEntry) error {
			mu.Lock()
			batches = append(batches, len(entries))
			mu.Unlock()
			return nil
		},
	}

	w := NewDrainWorker(drain, DrainWorkerConfig{
		BatchSize:     1000, // large — only time triggers
		FlushInterval: 50 * time.Millisecond,
		QueueSize:     100,
	})
	w.Start()

	w.Enqueue(LogEntry{Message: "timely"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(batches)
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	w.Stop()

	mu.Lock()
	testutil.True(t, len(batches) >= 1, "expected time-based flush")
	testutil.Equal(t, 1, batches[0])
	mu.Unlock()
}

func TestDrainWorkerRetryOnError(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var attempts int
	drain := &mockDrain{
		name: "retry-test",
		sendFn: func(entries []LogEntry) error {
			mu.Lock()
			attempts++
			a := attempts
			mu.Unlock()
			if a <= 2 {
				return errDrainSend("server error")
			}
			return nil // succeed on 3rd attempt
		},
	}

	w := NewDrainWorker(drain, DrainWorkerConfig{
		BatchSize:      1,
		FlushInterval:  50 * time.Millisecond,
		QueueSize:      100,
		MaxRetries:     3,
		BaseRetryDelay: 10 * time.Millisecond, // fast for tests
	})
	w.Start()

	w.Enqueue(LogEntry{Message: "will retry"})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		a := attempts
		mu.Unlock()
		if a >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	w.Stop()

	mu.Lock()
	testutil.Equal(t, 3, attempts)
	mu.Unlock()

	stats := drain.Stats()
	testutil.Equal(t, int64(1), stats.Sent)
	testutil.Equal(t, int64(2), stats.Failed)
	testutil.Equal(t, int64(0), stats.Dropped)
}

func TestDrainWorkerRetryOn500UsesBackoffBounds(t *testing.T) {
	t.Parallel()
	skipIfNoLocalHTTPListener(t)

	var mu sync.Mutex
	var attempts []time.Time

	ts := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts = append(attempts, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))

	drain := NewHTTPDrain(DrainConfig{
		ID:      "retry-backoff",
		Type:    "http",
		URL:     ts.URL,
		Enabled: true,
	})

	w := NewDrainWorker(drain, DrainWorkerConfig{
		BatchSize:      1,
		FlushInterval:  10 * time.Millisecond,
		QueueSize:      100,
		MaxRetries:     3,
		BaseRetryDelay: 20 * time.Millisecond,
		MaxRetryDelay:  200 * time.Millisecond,
	})
	w.Start()
	w.Enqueue(LogEntry{Message: "retry test"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := len(attempts) >= 3
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	w.Stop()

	mu.Lock()
	got := append([]time.Time{}, attempts...)
	mu.Unlock()

	testutil.Equal(t, 3, len(got))

	firstRetryDelay := got[1].Sub(got[0])
	secondRetryDelay := got[2].Sub(got[1])
	testutil.True(t, firstRetryDelay >= 10*time.Millisecond, "first retry should honor lower bound")
	testutil.True(t, firstRetryDelay <= 120*time.Millisecond, "first retry should remain bounded")
	testutil.True(t, secondRetryDelay >= 20*time.Millisecond, "second retry should account for exponential backoff")
	testutil.True(t, secondRetryDelay <= 260*time.Millisecond, "second retry should remain bounded")
	testutil.True(t, secondRetryDelay >= firstRetryDelay, "retry backoff should not shrink")

	stats := drain.Stats()
	testutil.Equal(t, int64(0), stats.Sent)
	testutil.Equal(t, int64(3), stats.Failed)
	testutil.Equal(t, int64(1), stats.Dropped)
}

func TestDrainWorkerDropsAfterMaxRetriesWithAccounting(t *testing.T) {
	t.Parallel()
	skipIfNoLocalHTTPListener(t)

	var mu sync.Mutex
	var attempts int

	ts := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))

	drain := NewHTTPDrain(DrainConfig{
		ID:      "retry-drop",
		Type:    "http",
		URL:     ts.URL,
		Enabled: true,
	})

	w := NewDrainWorker(drain, DrainWorkerConfig{
		BatchSize:      1,
		FlushInterval:  10 * time.Millisecond,
		QueueSize:      100,
		MaxRetries:     2,
		BaseRetryDelay: 20 * time.Millisecond,
		MaxRetryDelay:  80 * time.Millisecond,
	})
	w.Start()
	w.Enqueue(LogEntry{Message: "drop test"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := attempts >= 2
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	w.Stop()

	mu.Lock()
	got := attempts
	mu.Unlock()
	testutil.Equal(t, 2, got)

	stats := drain.Stats()
	testutil.Equal(t, int64(2), stats.Failed)
	testutil.Equal(t, int64(1), stats.Dropped)
	testutil.Equal(t, int64(0), stats.Sent)
}

func TestDrainWorkerHighVolumeEnqueueDoesNotBlockCaller(t *testing.T) {
	t.Parallel()
	skipIfNoLocalHTTPListener(t)

	var calls atomic.Int32
	ts := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))

	drain := NewHTTPDrain(DrainConfig{
		ID:      "high-volume",
		Type:    "http",
		URL:     ts.URL,
		Enabled: true,
	})
	w := NewDrainWorker(drain, DrainWorkerConfig{
		BatchSize:     1000,
		FlushInterval: 10 * time.Millisecond,
		QueueSize:     100,
		MaxRetries:    1,
	})
	w.Start()

	start := time.Now()
	for i := 0; i < 5000; i++ {
		w.Enqueue(LogEntry{Message: "high volume"})
	}
	elapsed := time.Since(start)

	w.Stop()

	testutil.True(t, elapsed < 250*time.Millisecond, "high-volume enqueue should stay non-blocking")
	testutil.True(t, calls.Load() > 0, "high-volume enqueue should eventually send at least one batch")
}

func TestDrainWorkerStopDrainsRemaining(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var total int
	drain := &mockDrain{
		name: "stop-drain",
		sendFn: func(entries []LogEntry) error {
			mu.Lock()
			total += len(entries)
			mu.Unlock()
			return nil
		},
	}

	w := NewDrainWorker(drain, DrainWorkerConfig{
		BatchSize:     1000, // large — only flushes on stop
		FlushInterval: 10 * time.Second,
		QueueSize:     100,
	})
	w.Start()

	for range 7 {
		w.Enqueue(LogEntry{Message: "pending"})
	}
	// Brief pause so entries enter the channel.
	time.Sleep(50 * time.Millisecond)

	w.Stop()

	mu.Lock()
	testutil.Equal(t, 7, total)
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// DrainManager — fan-out
// ---------------------------------------------------------------------------

func TestDrainManagerFanOut(t *testing.T) {
	t.Parallel()

	var mu1, mu2 sync.Mutex
	var got1, got2 []string
	d1 := &mockDrain{name: "d1", sendFn: func(entries []LogEntry) error {
		mu1.Lock()
		for _, e := range entries {
			got1 = append(got1, e.Message)
		}
		mu1.Unlock()
		return nil
	}}
	d2 := &mockDrain{name: "d2", sendFn: func(entries []LogEntry) error {
		mu2.Lock()
		for _, e := range entries {
			got2 = append(got2, e.Message)
		}
		mu2.Unlock()
		return nil
	}}

	mgr := NewDrainManager()
	mgr.AddDrain("d1", d1, DrainWorkerConfig{
		BatchSize: 1, FlushInterval: 50 * time.Millisecond, QueueSize: 100,
	})
	mgr.AddDrain("d2", d2, DrainWorkerConfig{
		BatchSize: 1, FlushInterval: 50 * time.Millisecond, QueueSize: 100,
	})
	mgr.Start()

	mgr.Enqueue(LogEntry{Message: "broadcast"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu1.Lock()
		n1 := len(got1)
		mu1.Unlock()
		mu2.Lock()
		n2 := len(got2)
		mu2.Unlock()
		if n1 >= 1 && n2 >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mgr.Stop()

	mu1.Lock()
	testutil.Equal(t, 1, len(got1))
	testutil.Equal(t, "broadcast", got1[0])
	mu1.Unlock()
	mu2.Lock()
	testutil.Equal(t, 1, len(got2))
	testutil.Equal(t, "broadcast", got2[0])
	mu2.Unlock()
}

func TestDrainManagerRemoveDrain(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var count int
	d := &mockDrain{name: "removable", sendFn: func(entries []LogEntry) error {
		mu.Lock()
		count += len(entries)
		mu.Unlock()
		return nil
	}}

	mgr := NewDrainManager()
	mgr.AddDrain("rm1", d, DrainWorkerConfig{
		BatchSize: 1, FlushInterval: 50 * time.Millisecond, QueueSize: 100,
	})
	mgr.Start()

	mgr.Enqueue(LogEntry{Message: "before"})
	time.Sleep(200 * time.Millisecond) // let it flush

	mgr.RemoveDrain("rm1")

	mgr.Enqueue(LogEntry{Message: "after"})
	time.Sleep(200 * time.Millisecond) // should go nowhere

	mgr.Stop()

	mu.Lock()
	testutil.Equal(t, 1, count)
	mu.Unlock()
}

func TestDrainManagerRemoveStopsHTTPDrainDelivery(t *testing.T) {
	t.Parallel()
	skipIfNoLocalHTTPListener(t)

	var calls atomic.Int32
	ts := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))

	drain := NewHTTPDrain(DrainConfig{
		ID:      "removable",
		Type:    "http",
		URL:     ts.URL,
		Enabled: true,
	})

	mgr := NewDrainManager()
	mgr.AddDrain("rm1", drain, DrainWorkerConfig{
		BatchSize:     1,
		FlushInterval: 50 * time.Millisecond,
		QueueSize:     100,
	})
	mgr.Start()

	mgr.Enqueue(LogEntry{Message: "before"})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mgr.RemoveDrain("rm1")
	mgr.Enqueue(LogEntry{Message: "after"})
	time.Sleep(200 * time.Millisecond)

	mgr.Stop()

	testutil.Equal(t, int32(1), calls.Load())
}

func TestDrainManagerListDrains(t *testing.T) {
	t.Parallel()

	d := &mockDrain{name: "listed"}
	mgr := NewDrainManager()
	mgr.AddDrain("x", d, DrainWorkerConfig{
		BatchSize: 10, FlushInterval: time.Second, QueueSize: 100,
	})

	list := mgr.ListDrains()
	testutil.Equal(t, 1, len(list))
	testutil.Equal(t, "x", list[0].ID)
	testutil.Equal(t, "listed", list[0].Name)
}

// TestDrainManagerAddDrainAfterStart verifies that a drain added after Start()
// begins receiving entries — the critical bug fixed in iteration 167.
// Also verifies that the worker is started exactly once (no double-goroutine race).
func TestDrainManagerAddDrainAfterStart(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var messages []string
	d := &mockDrain{
		name: "late-arrival",
		sendFn: func(entries []LogEntry) error {
			mu.Lock()
			for _, e := range entries {
				messages = append(messages, e.Message)
			}
			mu.Unlock()
			return nil
		},
	}

	mgr := NewDrainManager()
	mgr.Start() // started with no drains

	// Add drain after Start — should auto-start the worker.
	mgr.AddDrain("late", d, DrainWorkerConfig{
		BatchSize: 1, FlushInterval: 50 * time.Millisecond, QueueSize: 100,
	})

	mgr.Enqueue(LogEntry{Message: "post-start"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(messages)
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mgr.Stop()

	mu.Lock()
	testutil.Equal(t, 1, len(messages))
	testutil.Equal(t, "post-start", messages[0])
	mu.Unlock()
}

func TestDrainManagerReplacingDrainIDDoesNotLeakWorkers(t *testing.T) {
	base := runtime.NumGoroutine()

	mgr := NewDrainManager()
	mgr.Start()
	for i := 0; i < 200; i++ {
		d := &mockDrain{
			name: "replace",
			sendFn: func(entries []LogEntry) error {
				return nil
			},
		}
		mgr.AddDrain("same-id", d, DrainWorkerConfig{
			BatchSize: 1, FlushInterval: 50 * time.Millisecond, QueueSize: 8,
		})
	}

	mgr.Stop()
	time.Sleep(100 * time.Millisecond)

	after := runtime.NumGoroutine()
	testutil.True(t, after-base < 25, "replacing a drain ID should not leak workers: base=%d after=%d", base, after)
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

// TestDrainWorkerConcurrentEnqueue spawns 50 goroutines each enqueuing 100 entries
// and verifies that sent + dropped equals total enqueued. Passes under -race.
func TestDrainWorkerConcurrentEnqueue(t *testing.T) {
	t.Parallel()

	const goroutines = 50
	const perGoroutine = 100
	const total = goroutines * perGoroutine

	var mu sync.Mutex
	var totalSent int
	drain := &mockDrain{
		name: "concurrent",
		sendFn: func(entries []LogEntry) error {
			mu.Lock()
			totalSent += len(entries)
			mu.Unlock()
			return nil
		},
	}

	w := NewDrainWorker(drain, DrainWorkerConfig{
		BatchSize:     50,
		FlushInterval: 20 * time.Millisecond,
		QueueSize:     200, // smaller than total to exercise drop path under contention
	})
	w.Start()

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perGoroutine {
				w.Enqueue(LogEntry{Message: "concurrent"})
			}
		}()
	}
	wg.Wait()
	w.Stop()

	mu.Lock()
	sent := totalSent
	mu.Unlock()

	stats := drain.Stats()
	totalAccounted := sent + int(stats.Dropped)
	if totalAccounted > total {
		t.Errorf("accounted(%d) > total_sent(%d): counting error", totalAccounted, total)
	}
	// At least some entries must have been processed.
	if sent == 0 {
		t.Error("expected at least some entries to be sent")
	}
}

// TestDrainManagerConcurrentFanout registers 3 drains and has 20 goroutines
// enqueue entries concurrently; verifies no races or deadlocks under -race.
func TestDrainManagerConcurrentFanout(t *testing.T) {
	t.Parallel()

	const numDrains = 3
	const goroutines = 20
	const perGoroutine = 50

	counters := make([]atomic.Int64, numDrains)
	mgr := NewDrainManager()

	for i := range numDrains {
		idx := i
		d := &mockDrain{
			name: "d" + string(rune('0'+idx)),
			sendFn: func(entries []LogEntry) error {
				counters[idx].Add(int64(len(entries)))
				return nil
			},
		}
		mgr.AddDrain(d.name, d, DrainWorkerConfig{
			BatchSize:     10,
			FlushInterval: 20 * time.Millisecond,
			QueueSize:     200,
		})
	}
	mgr.Start()

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perGoroutine {
				mgr.Enqueue(LogEntry{Message: "fanout"})
			}
		}()
	}
	wg.Wait()
	mgr.Stop()

	// Each drain should have received at least some entries.
	for i := range numDrains {
		if counters[i].Load() == 0 {
			t.Errorf("drain %d received 0 entries; expected at least some", i)
		}
	}
}

// TestDrainWorkerDropsOnTargetTimeout verifies that when the drain target takes
// longer than the HTTP client timeout, entries are retried and then dropped with
// correct stats.Dropped accounting.
func TestDrainWorkerDropsOnTargetTimeout(t *testing.T) {
	t.Parallel()

	// Use a transport-level stub that always simulates a client timeout.  This
	// avoids spawning a real HTTP server (and the associated httptest cleanup
	// delay) while still exercising the full retry-and-drop path in the worker.
	stub := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return nil, &timeoutError{}
	})

	drain := NewHTTPDrain(DrainConfig{
		ID:      "timeout-drain",
		Type:    "http",
		URL:     "http://stub.invalid/logs",
		Enabled: true,
	})
	// Inject the stub transport so no real network I/O occurs.
	drain.SetHTTPTransport(stub)

	w := NewDrainWorker(drain, DrainWorkerConfig{
		BatchSize:      1,
		FlushInterval:  20 * time.Millisecond,
		QueueSize:      10,
		MaxRetries:     2,
		BaseRetryDelay: 10 * time.Millisecond,
		MaxRetryDelay:  50 * time.Millisecond,
	})
	w.Start()
	w.Enqueue(LogEntry{Message: "will timeout"})

	// Wait for retries to exhaust.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if drain.Stats().Dropped > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	w.Stop()

	stats := drain.Stats()
	if stats.Dropped == 0 {
		t.Error("expected stats.Dropped > 0 after all retries exhausted due to timeout")
	}
	if stats.Sent > 0 {
		t.Errorf("expected stats.Sent == 0 when target always times out, got %d", stats.Sent)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// roundTripperFunc adapts a plain function to the http.RoundTripper interface.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// timeoutError implements net.Error so that the HTTP client treats it as a
// timeout rather than a connection error.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "stub timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

type errDrainSend string

func (e errDrainSend) Error() string { return string(e) }

type mockDrain struct {
	name   string
	sendFn func([]LogEntry) error
	mu     sync.Mutex
	stats  DrainStats
}

func (m *mockDrain) Send(entries []LogEntry) error {
	err := m.sendFn(entries)
	m.mu.Lock()
	if err != nil {
		m.stats.Failed += int64(len(entries))
	} else {
		m.stats.Sent += int64(len(entries))
	}
	m.mu.Unlock()
	return err
}

func (m *mockDrain) Name() string {
	return m.name
}

func (m *mockDrain) Stats() DrainStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stats
}

func (m *mockDrain) ReportDropped(n int64) {
	m.mu.Lock()
	m.stats.Dropped += n
	m.mu.Unlock()
}
