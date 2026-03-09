//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

// setupBenchServer resets the DB, seeds it with N rows, and returns a ready server.
func setupBenchServer(b *testing.B, ctx context.Context, seedRows int) *server.Server {
	b.Helper()

	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		b.Fatalf("resetting schema: %v", err)
	}

	_, err = sharedPG.Pool.Exec(ctx, `
		CREATE TABLE bench_items (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			body TEXT,
			status TEXT DEFAULT 'active',
			created_at TIMESTAMPTZ DEFAULT now()
		)
	`)
	if err != nil {
		b.Fatalf("creating bench table: %v", err)
	}

	// Bulk-insert seed rows using generate_series for speed.
	if seedRows > 0 {
		_, err = sharedPG.Pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO bench_items (title, body, status)
			SELECT
				'Item ' || i,
				'Body for item ' || i,
				CASE WHEN i %% 2 = 0 THEN 'active' ELSE 'archived' END
			FROM generate_series(1, %d) AS i
		`, seedRows))
		if err != nil {
			b.Fatalf("seeding %d rows: %v", seedRows, err)
		}
	}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		b.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	return srv
}

func benchRequest(b *testing.B, srv *server.Server, method, path string, body any) *httptest.ResponseRecorder {
	b.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reqBody = bytes.NewReader(data)
	}
	var req *http.Request
	if reqBody != nil {
		req = httptest.NewRequest(method, path, reqBody)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

// BenchmarkReadSingle measures GET /api/collections/bench_items/1 throughput.
func BenchmarkReadSingle(b *testing.B) {
	ctx := context.Background()
	srv := setupBenchServer(b, ctx, 100)

	// Verify the endpoint works before benchmarking.
	w := benchRequest(b, srv, "GET", "/api/collections/bench_items/1", nil)
	if w.Code != http.StatusOK {
		b.Fatalf("setup check failed: status %d, body: %s", w.Code, w.Body.String())
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchRequest(b, srv, "GET", "/api/collections/bench_items/1", nil)
		}
	})
}

// BenchmarkListDefault measures GET /api/collections/bench_items (default pagination, 20 rows).
func BenchmarkListDefault(b *testing.B) {
	ctx := context.Background()
	srv := setupBenchServer(b, ctx, 100)

	w := benchRequest(b, srv, "GET", "/api/collections/bench_items", nil)
	if w.Code != http.StatusOK {
		b.Fatalf("setup check failed: status %d, body: %s", w.Code, w.Body.String())
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchRequest(b, srv, "GET", "/api/collections/bench_items", nil)
		}
	})
}

// BenchmarkListFiltered measures GET with filter parameter.
func BenchmarkListFiltered(b *testing.B) {
	ctx := context.Background()
	srv := setupBenchServer(b, ctx, 1000)

	path := "/api/collections/bench_items?filter=status%3D%27active%27&perPage=50"
	w := benchRequest(b, srv, "GET", path, nil)
	if w.Code != http.StatusOK {
		b.Fatalf("setup check failed: status %d, body: %s", w.Code, w.Body.String())
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchRequest(b, srv, "GET", path, nil)
		}
	})
}

// BenchmarkListLargeTable measures list performance against 10k rows.
func BenchmarkListLargeTable(b *testing.B) {
	ctx := context.Background()
	srv := setupBenchServer(b, ctx, 10000)

	w := benchRequest(b, srv, "GET", "/api/collections/bench_items?perPage=100", nil)
	if w.Code != http.StatusOK {
		b.Fatalf("setup check failed: status %d, body: %s", w.Code, w.Body.String())
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchRequest(b, srv, "GET", "/api/collections/bench_items?perPage=100", nil)
		}
	})
}

// BenchmarkCreate measures POST /api/collections/bench_items throughput.
func BenchmarkCreate(b *testing.B) {
	ctx := context.Background()
	srv := setupBenchServer(b, ctx, 0)

	body := map[string]any{
		"title":  "Benchmark item",
		"body":   "Created during benchmark",
		"status": "active",
	}
	w := benchRequest(b, srv, "POST", "/api/collections/bench_items", body)
	if w.Code != http.StatusCreated {
		b.Fatalf("setup check failed: status %d, body: %s", w.Code, w.Body.String())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchRequest(b, srv, "POST", "/api/collections/bench_items", body)
	}
}

// BenchmarkUpdate measures PATCH /api/collections/bench_items/{id} throughput.
func BenchmarkUpdate(b *testing.B) {
	ctx := context.Background()
	srv := setupBenchServer(b, ctx, 100)

	body := map[string]any{
		"title": "Updated title",
	}
	w := benchRequest(b, srv, "PATCH", "/api/collections/bench_items/1", body)
	if w.Code != http.StatusOK {
		b.Fatalf("setup check failed: status %d, body: %s", w.Code, w.Body.String())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Update different rows to avoid contention
		id := (i % 100) + 1
		benchRequest(b, srv, "PATCH", fmt.Sprintf("/api/collections/bench_items/%d", id), body)
	}
}

// BenchmarkDelete measures DELETE /api/collections/bench_items/{id} throughput.
// Each iteration creates a row then deletes it.
func BenchmarkDelete(b *testing.B) {
	ctx := context.Background()
	srv := setupBenchServer(b, ctx, 0)

	// Pre-create rows to delete.
	for i := 0; i < b.N+10; i++ {
		body := map[string]any{"title": fmt.Sprintf("Del %d", i), "body": "x"}
		benchRequest(b, srv, "POST", "/api/collections/bench_items", body)
	}

	b.ResetTimer()
	for i := 1; i <= b.N; i++ {
		benchRequest(b, srv, "DELETE", fmt.Sprintf("/api/collections/bench_items/%d", i), nil)
	}
}

// BenchmarkBatchCreate measures POST /api/collections/bench_items/batch throughput.
func BenchmarkBatchCreate(b *testing.B) {
	ctx := context.Background()
	srv := setupBenchServer(b, ctx, 0)

	ops := make([]map[string]any, 50)
	for i := range ops {
		ops[i] = map[string]any{
			"method": "create",
			"body": map[string]any{
				"title":  fmt.Sprintf("Batch item %d", i),
				"body":   "Batch created",
				"status": "active",
			},
		}
	}
	body := map[string]any{"operations": ops}

	w := benchRequest(b, srv, "POST", "/api/collections/bench_items/batch", body)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		b.Fatalf("setup check failed: status %d, body: %s", w.Code, w.Body.String())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchRequest(b, srv, "POST", "/api/collections/bench_items/batch", body)
	}
}

// BenchmarkHealthCheck measures GET /health throughput (baseline, no DB).
func BenchmarkHealthCheck(b *testing.B) {
	ctx := context.Background()
	srv := setupBenchServer(b, ctx, 0)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchRequest(b, srv, "GET", "/health", nil)
		}
	})
}
