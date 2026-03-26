//go:build integration

package api_test

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"testing"

	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

func insertSpatialPoint(t *testing.T, srv *server.Server, table, name string, lng, lat float64) {
	t.Helper()
	body := map[string]any{
		"name": name,
		"location": map[string]any{
			"type":        "Point",
			"coordinates": []any{lng, lat},
		},
	}
	w := doRequest(t, srv, "POST", fmt.Sprintf("/api/collections/%s/", table), body)
	testutil.StatusCode(t, http.StatusCreated, w.Code)
}

func spatialItemNames(t *testing.T, body map[string]any) map[string]bool {
	t.Helper()
	items := jsonItems(t, body)
	names := make(map[string]bool, len(items))
	for _, item := range items {
		names[jsonStr(t, item["name"])] = true
	}
	return names
}

func spatialItemDistances(t *testing.T, body map[string]any) []float64 {
	t.Helper()
	items := jsonItems(t, body)
	distances := make([]float64, len(items))
	for i, item := range items {
		value, ok := item["_distance"].(float64)
		testutil.True(t, ok, "expected _distance to be numeric, got %T", item["_distance"])
		distances[i] = value
	}
	return distances
}

func assertAscendingDistances(t *testing.T, distances []float64) {
	t.Helper()
	for i := 1; i < len(distances); i++ {
		testutil.True(t, distances[i-1] <= distances[i]+1e-9, "distances must be ascending: %v", distances)
	}
}

func spatialItemNameList(t *testing.T, body map[string]any) []string {
	t.Helper()
	items := jsonItems(t, body)
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = jsonStr(t, item["name"])
	}
	return names
}

func aggregateResults(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["results"].([]any)
	testutil.True(t, ok, "expected results array, got %T", body["results"])
	results := make([]map[string]any, len(raw))
	for i, v := range raw {
		results[i] = v.(map[string]any)
	}
	return results
}

func TestPostGISSpatialNearGeometry(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places", "origin", 0, 0)
	insertSpatialPoint(t, srv, "places", "east", 0.05, 0)
	insertSpatialPoint(t, srv, "places", "far", 1, 1)

	w := doRequest(t, srv, "GET", "/api/collections/places/?near=location,0,0,0.1", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	names := spatialItemNames(t, body)
	testutil.Equal(t, 2, len(names))
	testutil.True(t, names["origin"])
	testutil.True(t, names["east"])
	testutil.False(t, names["far"])
}

func TestPostGISSpatialWithinGeometry(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places", "inside", 0, 0)
	insertSpatialPoint(t, srv, "places", "inside2", 0.05, 0.02)
	insertSpatialPoint(t, srv, "places", "outside", 0.3, 0.3)

	poly := `{"type":"Polygon","coordinates":[[[-0.1,-0.1],[0.1,-0.1],[0.1,0.1],[-0.1,0.1],[-0.1,-0.1]]]}`
	urlPolygon := url.QueryEscape(poly)

	w := doRequest(t, srv, "GET", fmt.Sprintf("/api/collections/places/?within=location,%s", urlPolygon), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	names := spatialItemNames(t, body)
	testutil.Equal(t, 2, len(names))
	testutil.True(t, names["inside"])
	testutil.True(t, names["inside2"])
	testutil.False(t, names["outside"])
}

func TestPostGISSpatialIntersectsGeometry(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places", "north", 0, 0)
	insertSpatialPoint(t, srv, "places", "south", 0, 0.05)
	insertSpatialPoint(t, srv, "places", "other", 1, 1)

	line := `{"type":"LineString","coordinates":[[-1,0],[1,0]]}`
	urlLine := url.QueryEscape(line)

	w := doRequest(t, srv, "GET", fmt.Sprintf("/api/collections/places/?intersects=location,%s", urlLine), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	names := spatialItemNames(t, body)
	testutil.Equal(t, 1, len(names))
	testutil.True(t, names["north"])
	testutil.False(t, names["south"])
	testutil.False(t, names["other"])
}

func TestPostGISSpatialBBoxGeometry(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places", "low", -0.05, -0.05)
	insertSpatialPoint(t, srv, "places", "high", 0.05, 0.05)
	insertSpatialPoint(t, srv, "places", "out", 1, 1)

	w := doRequest(t, srv, "GET", "/api/collections/places/?bbox=location,-0.1,-0.1,0.1,0.1", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	names := spatialItemNames(t, body)
	testutil.Equal(t, 2, len(names))
	testutil.True(t, names["low"])
	testutil.True(t, names["high"])
	testutil.False(t, names["out"])
}

func TestPostGISSpatialNearGeography(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places_geog", "origin", 0, 0)
	insertSpatialPoint(t, srv, "places_geog", "near", 0.01, 0)
	insertSpatialPoint(t, srv, "places_geog", "far", 10, 10)

	w := doRequest(t, srv, "GET", "/api/collections/places_geog/?near=location,0,0,2000", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	names := spatialItemNames(t, body)
	testutil.Equal(t, 2, len(names))
	testutil.True(t, names["origin"])
	testutil.True(t, names["near"])
	testutil.False(t, names["far"])
}

func TestPostGISSpatialFilterAndNearCombination(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places", "foo-close", 0, 0)
	insertSpatialPoint(t, srv, "places", "foo-far", 0.2, 0)
	insertSpatialPoint(t, srv, "places", "bar-close", 0.05, 0)

	path := "/api/collections/places/?filter=name='foo-close'&near=location,0,0,0.1"
	w := doRequest(t, srv, "GET", path, nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	names := spatialItemNames(t, body)
	testutil.Equal(t, 1, len(names))
	testutil.True(t, names["foo-close"])
	testutil.False(t, names["foo-far"])
	testutil.False(t, names["bar-close"])
}

func TestPostGISDistanceSortGeometryAndDistanceColumn(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places", "origin", 0, 0)
	insertSpatialPoint(t, srv, "places", "east", 0.01, 0)
	insertSpatialPoint(t, srv, "places", "north", 0, 0.02)

	w := doRequest(t, srv, "GET", "/api/collections/places/?sort=location.distance(0,0)", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	names := spatialItemNameList(t, body)
	testutil.Equal(t, "origin", names[0])
	testutil.Equal(t, "east", names[1])
	testutil.Equal(t, "north", names[2])
	assertAscendingDistances(t, spatialItemDistances(t, body))
}

func TestPostGISDistanceSortGeographyAndDistanceColumn(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places_geog", "origin", 0, 0)
	insertSpatialPoint(t, srv, "places_geog", "near", 0.01, 0)
	insertSpatialPoint(t, srv, "places_geog", "far", 0.03, 0)

	w := doRequest(t, srv, "GET", "/api/collections/places_geog/?sort=location.distance(0,0)", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	names := spatialItemNameList(t, body)
	testutil.Equal(t, "origin", names[0])
	testutil.Equal(t, "near", names[1])
	testutil.Equal(t, "far", names[2])
	assertAscendingDistances(t, spatialItemDistances(t, body))
}

func TestPostGISDistanceSortCombinedWithNearFilter(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places", "origin", 0, 0)
	insertSpatialPoint(t, srv, "places", "mid", 0.02, 0)
	insertSpatialPoint(t, srv, "places", "outside", 1, 1)

	w := doRequest(t, srv, "GET", "/api/collections/places/?near=location,0,0,0.05&sort=location.distance(0,0)", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	names := spatialItemNames(t, body)
	testutil.Equal(t, 2, len(names))
	testutil.True(t, names["origin"])
	testutil.True(t, names["mid"])
	testutil.False(t, names["outside"])
	assertAscendingDistances(t, spatialItemDistances(t, body))
}

func TestPostGISDistanceSortGeometry3857UsesTransform(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)
	pool := postgisPool(t)

	_, err := pool.Exec(ctx, `
		INSERT INTO places_3857(name, location) VALUES
			('origin', ST_SetSRID(ST_MakePoint(0, 0), 3857)),
			('near', ST_SetSRID(ST_MakePoint(1000, 0), 3857)),
			('far', ST_SetSRID(ST_MakePoint(5000, 0), 3857))
	`)
	testutil.NoError(t, err)

	w := doRequest(t, srv, "GET", "/api/collections/places_3857/?sort=location.distance(0,0)", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	body := parseJSON(t, w)
	names := spatialItemNameList(t, body)
	testutil.Equal(t, "origin", names[0])
	testutil.Equal(t, "near", names[1])
	testutil.Equal(t, "far", names[2])
	assertAscendingDistances(t, spatialItemDistances(t, body))
}

func TestPostGISDistanceSortOffsetAndCursorPagination(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places", "p0", 0, 0)
	insertSpatialPoint(t, srv, "places", "p1", 0.01, 0)
	insertSpatialPoint(t, srv, "places", "p2", 0.02, 0)
	insertSpatialPoint(t, srv, "places", "p3", 0.03, 0)
	insertSpatialPoint(t, srv, "places", "p4", 0.04, 0)

	offsetPath := "/api/collections/places/?sort=location.distance(0,0),id&perPage=2&page=1"
	w := doRequest(t, srv, "GET", offsetPath, nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body1 := parseJSON(t, w)
	dist1 := spatialItemDistances(t, body1)
	assertAscendingDistances(t, dist1)

	w = doRequest(t, srv, "GET", "/api/collections/places/?sort=location.distance(0,0),id&perPage=2&page=2", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	body2 := parseJSON(t, w)
	dist2 := spatialItemDistances(t, body2)
	assertAscendingDistances(t, dist2)
	testutil.True(t, dist1[len(dist1)-1] <= dist2[0]+1e-9)

	w = doRequest(t, srv, "GET", "/api/collections/places/?cursor=&perPage=2&sort=location.distance(0,0),id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	cursorPage1 := parseJSON(t, w)
	cursorItems1 := jsonItems(t, cursorPage1)
	testutil.Equal(t, 2, len(cursorItems1))
	nextCursor, ok := cursorPage1["nextCursor"].(string)
	testutil.True(t, ok, "expected nextCursor string")
	d1, ok := cursorItems1[1]["_distance"].(float64)
	testutil.True(t, ok, "expected _distance on cursor page 1")

	w = doRequest(t, srv, "GET", "/api/collections/places/?cursor="+url.QueryEscape(nextCursor)+"&perPage=2&sort=location.distance(0,0),id", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	cursorPage2 := parseJSON(t, w)
	cursorItems2 := jsonItems(t, cursorPage2)
	testutil.Equal(t, 2, len(cursorItems2))
	d2, ok := cursorItems2[0]["_distance"].(float64)
	testutil.True(t, ok, "expected _distance on cursor page 2")
	testutil.True(t, d1 <= d2+1e-9)
}

func TestPostGISSpatialAggregateBBoxGeometryAndGeography(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places", "a", 0, 0)
	insertSpatialPoint(t, srv, "places", "b", 1, 1)
	insertSpatialPoint(t, srv, "places_geog", "c", 0, 0)
	insertSpatialPoint(t, srv, "places_geog", "d", 1, 1)

	w := doRequest(t, srv, "GET", "/api/collections/places/?aggregate=bbox(location)", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	results := aggregateResults(t, parseJSON(t, w))
	testutil.Equal(t, 1, len(results))
	bboxGeometry, ok := results[0]["bbox_location"].(map[string]any)
	testutil.True(t, ok, "expected bbox_location GeoJSON")
	testutil.Equal(t, "Polygon", jsonStr(t, bboxGeometry["type"]))

	w = doRequest(t, srv, "GET", "/api/collections/places_geog/?aggregate=bbox(location)", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	results = aggregateResults(t, parseJSON(t, w))
	bboxGeog, ok := results[0]["bbox_location"].(map[string]any)
	testutil.True(t, ok, "expected geography bbox GeoJSON")
	testutil.Equal(t, "Polygon", jsonStr(t, bboxGeog["type"]))
}

func TestPostGISSpatialAggregateCentroid(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places", "a", 0, 0)
	insertSpatialPoint(t, srv, "places", "b", 2, 2)

	w := doRequest(t, srv, "GET", "/api/collections/places/?aggregate=centroid(location)", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	results := aggregateResults(t, parseJSON(t, w))
	testutil.Equal(t, 1, len(results))
	centroid, ok := results[0]["centroid_location"].(map[string]any)
	testutil.True(t, ok, "expected centroid GeoJSON")
	testutil.Equal(t, "Point", jsonStr(t, centroid["type"]))
	coords, ok := centroid["coordinates"].([]any)
	testutil.True(t, ok, "expected coordinates array")
	testutil.True(t, math.Abs(coords[0].(float64)-1.0) < 0.0001)
	testutil.True(t, math.Abs(coords[1].(float64)-1.0) < 0.0001)
}

func TestPostGISGeographyInsertReadFilterRoundTrip(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	body := map[string]any{
		"name": "Geo Point",
		"location": map[string]any{
			"type":        "Point",
			"coordinates": []any{-73.99, 40.73},
		},
	}
	w := doRequest(t, srv, "POST", "/api/collections/places_geog/", body)
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	created := parseJSON(t, w)
	id := created["id"]
	createdLoc, ok := created["location"].(map[string]any)
	testutil.True(t, ok, "expected created location GeoJSON")
	testutil.Equal(t, "Point", jsonStr(t, createdLoc["type"]))

	w = doRequest(t, srv, "GET", fmt.Sprintf("/api/collections/places_geog/%v", id), nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	read := parseJSON(t, w)
	readLoc, ok := read["location"].(map[string]any)
	testutil.True(t, ok, "expected read location GeoJSON")
	testutil.Equal(t, "Point", jsonStr(t, readLoc["type"]))

	w = doRequest(t, srv, "GET", "/api/collections/places_geog/?near=location,-73.99,40.73,200", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	names := spatialItemNames(t, parseJSON(t, w))
	testutil.True(t, names["Geo Point"])
}

func TestPostGISGeographyUpdateRoundTrip(t *testing.T) {
	ctx := context.Background()
	srv := setupPostGISServer(t, ctx)

	insertSpatialPoint(t, srv, "places_geog", "Movable", -73.9, 40.7)
	w := doRequest(t, srv, "GET", "/api/collections/places_geog/?filter=name='Movable'", nil)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	items := jsonItems(t, parseJSON(t, w))
	testutil.Equal(t, 1, len(items))
	id := items[0]["id"]

	updateBody := map[string]any{
		"location": map[string]any{
			"type":        "Point",
			"coordinates": []any{-122.4, 37.8},
		},
	}
	w = doRequest(t, srv, "PATCH", fmt.Sprintf("/api/collections/places_geog/%v", id), updateBody)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	updated := parseJSON(t, w)
	updatedLoc, ok := updated["location"].(map[string]any)
	testutil.True(t, ok, "expected updated location GeoJSON")
	coords := updatedLoc["coordinates"].([]any)
	testutil.True(t, math.Abs(coords[0].(float64)-(-122.4)) < 0.0001)
	testutil.True(t, math.Abs(coords[1].(float64)-37.8) < 0.0001)
}
