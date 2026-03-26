//go:build integration

package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgisPool returns a pool connected to the PostGIS-enabled database,
// or skips the test if AYB_TEST_POSTGIS_URL is unset.
func postgisPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("AYB_TEST_POSTGIS_URL")
	if url == "" {
		t.Skip("AYB_TEST_POSTGIS_URL not set — skipping PostGIS test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connecting to PostGIS database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// setupPostGISServer sets up a test server connected to a PostGIS-enabled database.
func setupPostGISServer(t *testing.T, ctx context.Context) *server.Server {
	t.Helper()
	pool := postgisPool(t)

	// Reset schema and create PostGIS extension + test tables.
	_, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS postgis")
	if err != nil {
		t.Fatalf("creating PostGIS extension: %v", err)
	}

	_, err = pool.Exec(ctx, `
		CREATE TABLE places (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			location geometry(Point, 4326)
		);
		CREATE TABLE places_3857 (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			location geometry(Point, 3857)
		);
		CREATE TABLE places_geog (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			location geography(Point, 4326)
		);
		CREATE TABLE routes (
			id SERIAL PRIMARY KEY,
			label TEXT NOT NULL,
			path geometry(LineString, 4326)
		);
		CREATE TABLE areas (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			boundary geometry(Polygon, 4326),
			center geometry(Point, 4326)
		);
		CREATE TABLE nullable_geo (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			location geometry(Point, 4326)
		);
		CREATE TABLE parent_with_geo (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			location geometry(Point, 4326)
		);
		CREATE TABLE child_of_geo (
			id SERIAL PRIMARY KEY,
			parent_id INTEGER REFERENCES parent_with_geo(id),
			note TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("creating test tables: %v", err)
	}

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	srv := server.New(cfg, logger, ch, pool, nil, nil)
	return srv
}

func readSSEEventLines(t *testing.T, scanner *bufio.Scanner, timeout time.Duration) []string {
	t.Helper()

	eventCh := make(chan []string, 1)
	errCh := make(chan error, 1)
	go func() {
		var lines []string
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if len(lines) > 0 {
					eventCh <- lines
					return
				}
				continue // skip leading blank lines
			}
			lines = append(lines, line)
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
			return
		}
		errCh <- fmt.Errorf("sse stream closed while waiting for event")
	}()

	select {
	case lines := <-eventCh:
		return lines
	case err := <-errCh:
		t.Fatalf("reading sse event: %v", err)
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for sse event after %s", timeout)
	}

	return nil
}

func parseSSEDataEvent(t *testing.T, lines []string) map[string]any {
	t.Helper()

	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		var event map[string]any
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			t.Fatalf("parsing sse data payload: %v", err)
		}
		return event
	}

	t.Fatalf("missing data line in sse event: %v", lines)
	return nil
}

// --- GeoJSON Point round-trip ---

func TestPostGISInsertAndReadPoint(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	// Insert a GeoJSON Point.
	body := map[string]any{
		"name": "Central Park",
		"location": map[string]any{
			"type":        "Point",
			"coordinates": []any{-73.965355, 40.782865},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/places/", body)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	created := parseJSON(t, w)
	id := created["id"]

	// Verify returned location is GeoJSON.
	loc, ok := created["location"].(map[string]any)
	testutil.True(t, ok, "location should be a GeoJSON object, got %T", created["location"])
	testutil.Equal(t, "Point", jsonStr(t, loc["type"]))
	coords, ok := loc["coordinates"].([]any)
	testutil.True(t, ok, "coordinates should be an array")
	testutil.Equal(t, 2, len(coords))

	// Read back the same record.
	w = doRequest(t, srv, "GET", fmt.Sprintf("/api/collections/places/%v", id), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	read := parseJSON(t, w)
	readLoc, ok := read["location"].(map[string]any)
	testutil.True(t, ok, "read location should be a GeoJSON object")
	testutil.Equal(t, "Point", jsonStr(t, readLoc["type"]))
}

// --- GeoJSON LineString round-trip ---

func TestPostGISInsertAndReadLineString(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	body := map[string]any{
		"label": "Route A",
		"path": map[string]any{
			"type":        "LineString",
			"coordinates": []any{[]any{-73.9, 40.7}, []any{-73.8, 40.8}, []any{-73.7, 40.9}},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/routes/", body)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	created := parseJSON(t, w)
	path, ok := created["path"].(map[string]any)
	testutil.True(t, ok, "path should be a GeoJSON object")
	testutil.Equal(t, "LineString", jsonStr(t, path["type"]))

	coords, ok := path["coordinates"].([]any)
	testutil.True(t, ok, "coordinates should be an array")
	testutil.Equal(t, 3, len(coords))
}

// --- GeoJSON Polygon round-trip ---

func TestPostGISInsertAndReadPolygon(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	body := map[string]any{
		"name": "District 1",
		"boundary": map[string]any{
			"type": "Polygon",
			"coordinates": []any{
				[]any{
					[]any{-73.9, 40.7},
					[]any{-73.8, 40.7},
					[]any{-73.8, 40.8},
					[]any{-73.9, 40.8},
					[]any{-73.9, 40.7},
				},
			},
		},
		"center": map[string]any{
			"type":        "Point",
			"coordinates": []any{-73.85, 40.75},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/areas/", body)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	created := parseJSON(t, w)
	boundary, ok := created["boundary"].(map[string]any)
	testutil.True(t, ok, "boundary should be a GeoJSON object")
	testutil.Equal(t, "Polygon", jsonStr(t, boundary["type"]))

	center, ok := created["center"].(map[string]any)
	testutil.True(t, ok, "center should be a GeoJSON object")
	testutil.Equal(t, "Point", jsonStr(t, center["type"]))
}

// --- Update a spatial column ---

func TestPostGISUpdateGeometry(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	// Create.
	createBody := map[string]any{
		"name": "Movable Place",
		"location": map[string]any{
			"type":        "Point",
			"coordinates": []any{-73.9, 40.7},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/places/", createBody)
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	created := parseJSON(t, w)
	id := created["id"]

	// Update with new location.
	updateBody := map[string]any{
		"location": map[string]any{
			"type":        "Point",
			"coordinates": []any{-122.4, 37.8},
		},
	}
	w = doRequest(t, srv, "PATCH", fmt.Sprintf("/api/collections/places/%v", id), updateBody)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	updated := parseJSON(t, w)
	loc, ok := updated["location"].(map[string]any)
	testutil.True(t, ok, "updated location should be a GeoJSON object")
	testutil.Equal(t, "Point", jsonStr(t, loc["type"]))

	coords := loc["coordinates"].([]any)
	testutil.True(t, coords[0].(float64) < -122.0, "longitude should be updated")
}

// --- List records with spatial columns ---

func TestPostGISListWithGeometry(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	// Create multiple records.
	for i, coord := range [][]float64{{-73.9, 40.7}, {-73.8, 40.8}} {
		body := map[string]any{
			"name": fmt.Sprintf("Place %d", i),
			"location": map[string]any{
				"type":        "Point",
				"coordinates": coord,
			},
		}
		w := doRequest(t, srv, "POST", "/api/collections/places/", body)
		testutil.StatusCode(t, http.StatusCreated, w.Code)
	}

	// List.
	w := doRequest(t, srv, "GET", "/api/collections/places/", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	items := jsonItems(t, body)
	testutil.True(t, len(items) >= 2, "should have at least 2 items")

	// Each item should have location as GeoJSON object.
	for _, item := range items {
		loc, ok := item["location"].(map[string]any)
		testutil.True(t, ok, "list item location should be GeoJSON object, got %T", item["location"])
		testutil.Equal(t, "Point", jsonStr(t, loc["type"]))
	}
}

// --- Null geometry column ---

func TestPostGISNullGeometry(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	body := map[string]any{
		"name":     "No Location",
		"location": nil,
	}
	w := doRequest(t, srv, "POST", "/api/collections/nullable_geo/", body)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	created := parseJSON(t, w)
	testutil.Nil(t, created["location"])

	// Read back.
	w = doRequest(t, srv, "GET", fmt.Sprintf("/api/collections/nullable_geo/%v", created["id"]), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	read := parseJSON(t, w)
	testutil.Nil(t, read["location"])
}

// --- Invalid GeoJSON input ---

func TestPostGISInvalidGeoJSONReturnsError(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	body := map[string]any{
		"name":     "Bad Place",
		"location": map[string]any{"type": "InvalidType", "coordinates": "not-an-array"},
	}
	w := doRequest(t, srv, "POST", "/api/collections/places/", body)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	errResp := parseJSON(t, w)
	testutil.Equal(t, "invalid GeoJSON geometry", jsonStr(t, errResp["message"]))
	testutil.True(t, strings.Contains(jsonStr(t, errResp["doc_url"]), "/guide/api-reference#error-format"),
		"expected doc_url for normalized error format")
}

// --- Batch create with geometry ---

func TestPostGISBatchCreateWithGeometry(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	batchBody := map[string]any{
		"operations": []map[string]any{
			{
				"method": "create",
				"body": map[string]any{
					"name": "Batch Place 1",
					"location": map[string]any{
						"type":        "Point",
						"coordinates": []any{-73.9, 40.7},
					},
				},
			},
			{
				"method": "create",
				"body": map[string]any{
					"name": "Batch Place 2",
					"location": map[string]any{
						"type":        "Point",
						"coordinates": []any{-122.4, 37.8},
					},
				},
			},
		},
	}

	w := doRequest(t, srv, "POST", "/api/collections/places/batch", batchBody)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var results []map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))
	testutil.Equal(t, 2, len(results))

	for _, result := range results {
		body, ok := result["body"].(map[string]any)
		testutil.True(t, ok, "batch result should have body")

		loc, ok := body["location"].(map[string]any)
		testutil.True(t, ok, "batch result location should be GeoJSON object")
		testutil.Equal(t, "Point", jsonStr(t, loc["type"]))
	}
}

// --- Expand on table with geometry columns ---

func TestPostGISExpandWithGeometry(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	// Create parent with geometry.
	parentBody := map[string]any{
		"name": "HQ",
		"location": map[string]any{
			"type":        "Point",
			"coordinates": []any{-73.9, 40.7},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/parent_with_geo/", parentBody)
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	parent := parseJSON(t, w)

	// Create child referencing parent.
	childBody := map[string]any{
		"parent_id": parent["id"],
		"note":      "test child",
	}
	w = doRequest(t, srv, "POST", "/api/collections/child_of_geo/", childBody)
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	child := parseJSON(t, w)

	// Read child with expand=parent (derived from parent_id FK by deriveFieldName).
	w = doRequest(t, srv, "GET", fmt.Sprintf("/api/collections/child_of_geo/%v?expand=parent", child["id"]), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	readChild := parseJSON(t, w)
	expand, ok := readChild["expand"].(map[string]any)
	testutil.True(t, ok, "expand key should be present in response")

	parentRec, ok := expand["parent"].(map[string]any)
	testutil.True(t, ok, "expanded parent should be present under expand.parent")

	loc, ok := parentRec["location"].(map[string]any)
	testutil.True(t, ok, "expanded parent location should be a GeoJSON object, got %T", parentRec["location"])
	testutil.Equal(t, "Point", jsonStr(t, loc["type"]))
}

func TestPostGISRealtimeSSEContainsGeoJSON(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/realtime?tables=places")
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
	testutil.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	scanner := bufio.NewScanner(resp.Body)
	connected := readSSEEventLines(t, scanner, 5*time.Second)
	testutil.True(t, len(connected) > 0, "expected connected event")
	testutil.Equal(t, "event: connected", connected[0])

	createBody := map[string]any{
		"name": "SSE Place",
		"location": map[string]any{
			"type":        "Point",
			"coordinates": []any{-73.9, 40.7},
		},
	}
	createResp := doRequest(t, srv, "POST", "/api/collections/places/", createBody)
	testutil.StatusCode(t, http.StatusCreated, createResp.Code)
	created := parseJSON(t, createResp)

	createEvent := parseSSEDataEvent(t, readSSEEventLines(t, scanner, 5*time.Second))
	testutil.Equal(t, "create", jsonStr(t, createEvent["action"]))
	testutil.Equal(t, "places", jsonStr(t, createEvent["table"]))

	createRecord, ok := createEvent["record"].(map[string]any)
	testutil.True(t, ok, "create event should include record object")
	createLoc, ok := createRecord["location"].(map[string]any)
	testutil.True(t, ok, "create event location should be GeoJSON object")
	testutil.Equal(t, "Point", jsonStr(t, createLoc["type"]))

	updateBody := map[string]any{
		"location": map[string]any{
			"type":        "Point",
			"coordinates": []any{-122.4, 37.8},
		},
	}
	updateResp := doRequest(t, srv, "PATCH", fmt.Sprintf("/api/collections/places/%v", created["id"]), updateBody)
	testutil.StatusCode(t, http.StatusOK, updateResp.Code)

	updateEvent := parseSSEDataEvent(t, readSSEEventLines(t, scanner, 5*time.Second))
	testutil.Equal(t, "update", jsonStr(t, updateEvent["action"]))
	testutil.Equal(t, "places", jsonStr(t, updateEvent["table"]))

	updateRecord, ok := updateEvent["record"].(map[string]any)
	testutil.True(t, ok, "update event should include record object")
	testutil.Equal(t, jsonNum(t, created["id"]), jsonNum(t, updateRecord["id"]))

	updateLoc, ok := updateRecord["location"].(map[string]any)
	testutil.True(t, ok, "update event location should be GeoJSON object")
	testutil.Equal(t, "Point", jsonStr(t, updateLoc["type"]))

	coords, ok := updateLoc["coordinates"].([]any)
	testutil.True(t, ok, "update event coordinates should be an array")
	testutil.True(t, len(coords) == 2, "update event coordinates should contain [lon, lat]")
	testutil.True(t, jsonNum(t, coords[0]) < -122.0, "update event longitude should reflect updated point")
}
