//go:build integration

package edgefunc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

func TestEdgeSpatialNearAndBBoxIntegration(t *testing.T) {
	ctx := context.Background()
	ensurePostGISAvailable(t, ctx)

	tableName := createEdgeSpatialTestTable(t, ctx)
	cache := edgeSpatialCache(tableName)
	qe := edgefunc.NewPostgresQueryExecutor(testPool, []string{tableName})
	pool := edgefunc.NewPool(1, edgefunc.WithPoolSchemaCache(func() *schema.SchemaCache { return cache }))
	t.Cleanup(pool.Close)

	_, err := testPool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (name, location, geog) VALUES
			('nearby',
			 ST_SetSRID(ST_MakePoint(-73.9857, 40.7484), 4326),
			 ST_SetSRID(ST_MakePoint(-73.9857, 40.7484), 4326)::geography
			),
			('far',
			 ST_SetSRID(ST_MakePoint(-73.9857, 40.8484), 4326),
			 ST_SetSRID(ST_MakePoint(-73.9857, 40.8484), 4326)::geography
			)
	`, sqlutil.QuoteIdent(tableName)))
	testutil.NoError(t, err)

	code := fmt.Sprintf(`function handler() {
		const nearGeom = ayb.spatial.near("%s", "location", -73.9857, 40.7484, 1000);
		const nearGeog = ayb.spatial.near("%s", "geog", -73.9857, 40.7484, 1000);
		const inBox = ayb.spatial.bbox("%s", "location", -74.0, 40.70, -73.9, 40.80);
		return {
			statusCode: 200,
			body: JSON.stringify({nearGeom, nearGeog, inBox})
		};
	}`, tableName, tableName, tableName)

	resp, err := pool.Execute(ctx, code, "handler", edgefunc.Request{Method: "GET", Path: "/"}, nil, qe)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)

	var payload struct {
		NearGeom []map[string]any `json:"nearGeom"`
		NearGeog []map[string]any `json:"nearGeog"`
		InBox    []map[string]any `json:"inBox"`
	}
	testutil.NoError(t, json.Unmarshal(resp.Body, &payload))
	testutil.SliceLen(t, payload.NearGeom, 1)
	testutil.SliceLen(t, payload.NearGeog, 1)
	testutil.SliceLen(t, payload.InBox, 1)
	testutil.Equal(t, "nearby", payload.NearGeom[0]["name"].(string))
	testutil.Equal(t, "nearby", payload.NearGeog[0]["name"].(string))
	testutil.Equal(t, "nearby", payload.InBox[0]["name"].(string))

	location, ok := payload.NearGeom[0]["location"].(map[string]any)
	testutil.True(t, ok, "expected location to be GeoJSON object")
	testutil.Equal(t, "Point", location["type"].(string))
}

func createEdgeSpatialTestTable(t *testing.T, ctx context.Context) string {
	t.Helper()
	tableName := "test_edge_spatial_" + strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	_, err := testPool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			location geometry(Point, 4326) NOT NULL,
			geog geography(Point, 4326) NOT NULL
		)
	`, sqlutil.QuoteIdent(tableName)))
	testutil.NoError(t, err)

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS %s`, sqlutil.QuoteIdent(tableName)))
	})
	return tableName
}

func ensurePostGISAvailable(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := testPool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS postgis`)
	if err != nil {
		t.Skipf("postgis extension unavailable in integration database: %v", err)
	}
}

func edgeSpatialCache(tableName string) *schema.SchemaCache {
	return &schema.SchemaCache{
		HasPostGIS: true,
		Tables: map[string]*schema.Table{
			"public." + tableName: {
				Schema: "public",
				Name:   tableName,
				Columns: []*schema.Column{
					{Name: "id"},
					{Name: "name"},
					{Name: "location", IsGeometry: true, SRID: 4326, GeometryType: "Point"},
					{Name: "geog", IsGeometry: true, IsGeography: true, SRID: 4326, GeometryType: "Point"},
				},
			},
		},
	}
}
