package edgefunc

import (
	"context"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

type mockSpatialBridgeExecutor struct {
	lastRawSQL  string
	lastRawArgs []any
	rawResult   QueryResult
	rawErr      error
}

func (m *mockSpatialBridgeExecutor) Execute(_ context.Context, _ Query) (QueryResult, error) {
	return QueryResult{}, nil
}

func (m *mockSpatialBridgeExecutor) QueryRaw(_ context.Context, sql string, args ...any) (QueryResult, error) {
	m.lastRawSQL = sql
	m.lastRawArgs = append([]any(nil), args...)
	if m.rawErr != nil {
		return QueryResult{}, m.rawErr
	}
	return m.rawResult, nil
}

func TestPoolSpatialNearBuildsGeometrySQL(t *testing.T) {
	t.Parallel()

	cache := spatialBridgeCache(true, false)
	executor := &mockSpatialBridgeExecutor{
		rawResult: QueryResult{
			Rows: []map[string]any{
				{"id": int64(1), "name": "point-a", "location": map[string]any{"type": "Point"}},
			},
		},
	}
	pool := NewPool(1, WithPoolSchemaCache(func() *schema.SchemaCache { return cache }))
	t.Cleanup(pool.Close)

	code := `function handler() {
		const rows = ayb.spatial.near("places", "location", -73.9, 40.7, 1000, {limit: 10, offset: 2});
		return { statusCode: 200, body: JSON.stringify(rows) };
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, executor)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Contains(t, string(resp.Body), `"point-a"`)
	testutil.Contains(t, executor.lastRawSQL, `FROM "public"."places"`)
	testutil.Contains(t, executor.lastRawSQL, `ST_AsGeoJSON("location")::jsonb AS "location"`)
	testutil.Contains(t, executor.lastRawSQL, `ST_DWithin("location", ST_SetSRID(ST_MakePoint($1, $2), 4326), $3)`)
	testutil.Contains(t, executor.lastRawSQL, `LIMIT $4`)
	testutil.Contains(t, executor.lastRawSQL, `OFFSET $5`)
	testutil.Equal(t, 5, len(executor.lastRawArgs))
	testutil.Equal(t, -73.9, executor.lastRawArgs[0].(float64))
	testutil.Equal(t, 40.7, executor.lastRawArgs[1].(float64))
	testutil.Equal(t, 1000.0, executor.lastRawArgs[2].(float64))
	testutil.Equal(t, 10, executor.lastRawArgs[3].(int))
	testutil.Equal(t, 2, executor.lastRawArgs[4].(int))
}

func TestPoolSpatialNearBuildsGeographySQL(t *testing.T) {
	t.Parallel()

	cache := spatialBridgeCache(true, true)
	executor := &mockSpatialBridgeExecutor{
		rawResult: QueryResult{Rows: []map[string]any{{"id": int64(1), "name": "point-g"}}},
	}
	pool := NewPool(1, WithPoolSchemaCache(func() *schema.SchemaCache { return cache }))
	t.Cleanup(pool.Close)

	code := `function handler() {
		const rows = ayb.spatial.near("places", "location", -73.9, 40.7, 500);
		return { statusCode: 200, body: JSON.stringify(rows) };
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, executor)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Contains(t, executor.lastRawSQL, `"location"::geography`)
	testutil.Contains(t, executor.lastRawSQL, `ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography`)
}

func TestPoolSpatialWithinRejectsNonPolygonGeoJSON(t *testing.T) {
	t.Parallel()

	cache := spatialBridgeCache(true, false)
	executor := &mockSpatialBridgeExecutor{}
	pool := NewPool(1, WithPoolSchemaCache(func() *schema.SchemaCache { return cache }))
	t.Cleanup(pool.Close)

	code := `function handler() {
		ayb.spatial.within("places", "location", '{"type":"Point","coordinates":[-73.9,40.7]}');
		return { statusCode: 200, body: "unexpected" };
	}`

	_, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, executor)
	testutil.ErrorContains(t, err, "Polygon or MultiPolygon")
}

func TestPoolSpatialNearRejectsOutOfRangeCoordinates(t *testing.T) {
	t.Parallel()

	cache := spatialBridgeCache(true, false)
	executor := &mockSpatialBridgeExecutor{}
	pool := NewPool(1, WithPoolSchemaCache(func() *schema.SchemaCache { return cache }))
	t.Cleanup(pool.Close)

	code := `function handler() {
		ayb.spatial.near("places", "location", 181, 40.7, 1000);
		return { statusCode: 200, body: "unexpected" };
	}`

	_, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, executor)
	testutil.ErrorContains(t, err, "valid WGS84 coordinates")
}

func TestPoolSpatialBBoxRejectsInvalidBounds(t *testing.T) {
	t.Parallel()

	cache := spatialBridgeCache(true, false)
	executor := &mockSpatialBridgeExecutor{}
	pool := NewPool(1, WithPoolSchemaCache(func() *schema.SchemaCache { return cache }))
	t.Cleanup(pool.Close)

	code := `function handler() {
		ayb.spatial.bbox("places", "location", 10, 0, 9, 1);
		return { statusCode: 200, body: "unexpected" };
	}`

	_, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, executor)
	testutil.ErrorContains(t, err, "minLng must be less than maxLng")
}

func TestPoolSpatialInfoIncludesPostGISMetadata(t *testing.T) {
	t.Parallel()

	cache := spatialBridgeCache(true, false)
	cache.PostGISVersion = "3.4"
	executor := &mockSpatialBridgeExecutor{}
	pool := NewPool(1, WithPoolSchemaCache(func() *schema.SchemaCache { return cache }))
	t.Cleanup(pool.Close)

	code := `function handler() {
		const info = ayb.spatial.info();
		return { statusCode: 200, body: JSON.stringify(info) };
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, executor)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	body := string(resp.Body)
	testutil.Contains(t, body, `"hasPostGIS":true`)
	testutil.Contains(t, body, `"postGISVersion":"3.4"`)
	testutil.Contains(t, body, `"table":"places"`)
	testutil.Contains(t, body, `"column":"location"`)
}

func TestPoolSpatialNearRejectsNonPublicTable(t *testing.T) {
	t.Parallel()

	cache := spatialBridgeCache(true, false)
	cache.Tables = map[string]*schema.Table{
		"private.secret_places": {
			Schema: "private",
			Name:   "secret_places",
			Columns: []*schema.Column{
				{Name: "id"},
				{
					Name:         "location",
					IsGeometry:   true,
					GeometryType: "Point",
					SRID:         4326,
				},
			},
		},
	}

	executor := &mockSpatialBridgeExecutor{}
	pool := NewPool(1, WithPoolSchemaCache(func() *schema.SchemaCache { return cache }))
	t.Cleanup(pool.Close)

	code := `function handler() {
		ayb.spatial.near("secret_places", "location", -73.9, 40.7, 1000);
		return { statusCode: 200, body: "unexpected" };
	}`

	_, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, executor)
	testutil.ErrorContains(t, err, `table "secret_places" not found in public schema`)
}

func TestPoolSpatialInfoOmitsNonPublicTables(t *testing.T) {
	t.Parallel()

	cache := spatialBridgeCache(true, false)
	cache.Tables["private.secret_places"] = &schema.Table{
		Schema: "private",
		Name:   "secret_places",
		Columns: []*schema.Column{
			{Name: "id"},
			{
				Name:         "location",
				IsGeometry:   true,
				GeometryType: "Point",
				SRID:         4326,
			},
		},
	}

	executor := &mockSpatialBridgeExecutor{}
	pool := NewPool(1, WithPoolSchemaCache(func() *schema.SchemaCache { return cache }))
	t.Cleanup(pool.Close)

	code := `function handler() {
		const info = ayb.spatial.info();
		return { statusCode: 200, body: JSON.stringify(info) };
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, executor)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	body := string(resp.Body)
	testutil.Contains(t, body, `"table":"places"`)
	if strings.Contains(body, `"table":"secret_places"`) {
		t.Fatalf("expected non-public spatial table to be omitted from ayb.spatial.info(): %s", body)
	}
}

func TestPoolSpatialNamespaceAbsentWhenPostGISDisabled(t *testing.T) {
	t.Parallel()

	cache := spatialBridgeCache(false, false)
	executor := &mockSpatialBridgeExecutor{}
	pool := NewPool(1, WithPoolSchemaCache(func() *schema.SchemaCache { return cache }))
	t.Cleanup(pool.Close)

	code := `function handler() {
		return { statusCode: 200, body: String(typeof ayb.spatial) };
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, executor)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "undefined", strings.TrimSpace(string(resp.Body)))
}

func spatialBridgeCache(hasPostGIS, geography bool) *schema.SchemaCache {
	spatialColumn := &schema.Column{
		Name:         "location",
		IsGeometry:   true,
		IsGeography:  geography,
		SRID:         4326,
		GeometryType: "Point",
	}
	return &schema.SchemaCache{
		HasPostGIS: hasPostGIS,
		Tables: map[string]*schema.Table{
			"public.places": {
				Schema: "public",
				Name:   "places",
				Columns: []*schema.Column{
					{Name: "id"},
					{Name: "name"},
					spatialColumn,
				},
			},
		},
	}
}
