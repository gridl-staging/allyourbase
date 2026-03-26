//go:build integration

package schema_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgisPool returns a connection pool to the PostGIS-enabled database,
// or skips the test if AYB_TEST_POSTGIS_URL is not set.
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

// resetPostgisDB drops and recreates the public schema on the PostGIS database.
func resetPostgisDB(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting PostGIS schema: %v", err)
	}
	// Ensure PostGIS extension is available.
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS postgis")
	if err != nil {
		t.Fatalf("creating PostGIS extension: %v", err)
	}
}

// createSpatialTestSchema creates test tables with geometry and geography columns.
func createSpatialTestSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	sqls := []string{
		`CREATE TABLE places (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			location geometry(Point, 4326),
			area geometry(Polygon, 4326)
		)`,
		`CREATE TABLE tracks (
			id SERIAL PRIMARY KEY,
			label TEXT NOT NULL,
			path geography(LineString, 4326)
		)`,
		`CREATE TABLE unconstrained_geo (
			id SERIAL PRIMARY KEY,
			shape geometry
		)`,
		`CREATE TABLE test_3d (
			id SERIAL PRIMARY KEY,
			loc geometry(PointZ, 4326)
		)`,
		`CREATE INDEX idx_places_location ON places USING GIST (location)`,
		`CREATE INDEX idx_places_area ON places USING GIST (area)`,
		`CREATE INDEX idx_tracks_path ON tracks USING GIST (path)`,
	}
	for _, sql := range sqls {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("creating spatial schema: %s: %v", sql[:40], err)
		}
	}
}

func TestPostGISExtensionDetected(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)

	cache, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)
	testutil.True(t, cache.HasPostGIS, "expected HasPostGIS=true")
	testutil.True(t, cache.PostGISVersion != "", "expected non-empty PostGISVersion")
}

func TestPostGISAbsentDetection(t *testing.T) {
	// Uses the regular shared test database (no PostGIS).
	ctx := context.Background()
	resetDB(t, ctx)

	cache, err := schema.BuildCache(ctx, sharedPG.Pool)
	testutil.NoError(t, err)
	testutil.False(t, cache.HasPostGIS, "expected HasPostGIS=false on standard Postgres")
	testutil.Equal(t, "", cache.PostGISVersion)
}

func TestPostGISGeometryIntrospection(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)
	createSpatialTestSchema(t, ctx, pool)

	cache, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)

	// places table — geometry(Point, 4326)
	places := cache.Tables["public.places"]
	testutil.NotNil(t, places)
	testutil.True(t, places.HasGeometry(), "places should have geometry")

	locCol := places.ColumnByName("location")
	testutil.NotNil(t, locCol)
	testutil.True(t, locCol.IsGeometry, "location should be geometry")
	testutil.False(t, locCol.IsGeography, "location should not be geography")
	testutil.Equal(t, "Point", locCol.GeometryType)
	testutil.Equal(t, 4326, locCol.SRID)
	testutil.Equal(t, 2, locCol.CoordDimension)
	testutil.Equal(t, "object", locCol.JSONType)

	areaCol := places.ColumnByName("area")
	testutil.NotNil(t, areaCol)
	testutil.True(t, areaCol.IsGeometry, "area should be geometry")
	testutil.False(t, areaCol.IsGeography, "area should not be geography")
	testutil.Equal(t, "Polygon", areaCol.GeometryType)
	testutil.Equal(t, 4326, areaCol.SRID)
	testutil.Equal(t, 2, areaCol.CoordDimension)
	testutil.Equal(t, "object", areaCol.JSONType)

	// Non-geometry column on same table.
	nameCol := places.ColumnByName("name")
	testutil.NotNil(t, nameCol)
	testutil.False(t, nameCol.IsGeometry, "name should not be geometry")
}

func TestGeographyColumnIntrospection(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)
	createSpatialTestSchema(t, ctx, pool)

	cache, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)

	tracks := cache.Tables["public.tracks"]
	testutil.NotNil(t, tracks)
	testutil.True(t, tracks.HasGeometry(), "tracks should have geometry")

	pathCol := tracks.ColumnByName("path")
	testutil.NotNil(t, pathCol)
	testutil.True(t, pathCol.IsGeometry, "path should be geometry")
	testutil.True(t, pathCol.IsGeography, "path should be geography")
	testutil.Equal(t, "LineString", pathCol.GeometryType)
	testutil.Equal(t, 4326, pathCol.SRID)
	testutil.Equal(t, 2, pathCol.CoordDimension)
	testutil.Equal(t, "object", pathCol.JSONType)
}

func TestUnconstrainedGeometryColumn(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)
	createSpatialTestSchema(t, ctx, pool)

	cache, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)

	tbl := cache.Tables["public.unconstrained_geo"]
	testutil.NotNil(t, tbl)
	testutil.True(t, tbl.HasGeometry(), "unconstrained_geo should have geometry")

	shapeCol := tbl.ColumnByName("shape")
	testutil.NotNil(t, shapeCol)
	testutil.True(t, shapeCol.IsGeometry, "shape should be geometry")
	testutil.False(t, shapeCol.IsGeography, "shape should not be geography")
	testutil.Equal(t, "", shapeCol.GeometryType)
	testutil.Equal(t, 0, shapeCol.SRID)
	testutil.Equal(t, 2, shapeCol.CoordDimension)
	testutil.Equal(t, "object", shapeCol.JSONType)
}

func TestPostGIS3DGeometryCoordDimension(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)
	createSpatialTestSchema(t, ctx, pool)

	cache, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)

	tbl := cache.Tables["public.test_3d"]
	testutil.NotNil(t, tbl)

	locCol := tbl.ColumnByName("loc")
	testutil.NotNil(t, locCol)
	testutil.True(t, locCol.IsGeometry, "loc should be geometry")
	testutil.False(t, locCol.IsGeography, "loc should not be geography")
	testutil.Equal(t, "Point", locCol.GeometryType)
	testutil.Equal(t, 4326, locCol.SRID)
	testutil.Equal(t, 3, locCol.CoordDimension)
}

func TestGISTIndexDetection(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)
	createSpatialTestSchema(t, ctx, pool)

	cache, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)

	places := cache.Tables["public.places"]
	testutil.NotNil(t, places)

	var gistIdx *schema.Index
	for _, idx := range places.Indexes {
		if idx.Name == "idx_places_location" {
			gistIdx = idx
			break
		}
	}
	testutil.NotNil(t, gistIdx)
	testutil.Equal(t, "gist", gistIdx.Method)
	testutil.SliceLen(t, gistIdx.Columns, 1)
	testutil.Equal(t, "location", gistIdx.Columns[0])
	testutil.False(t, gistIdx.IsUnique, "GIST index should not be unique")
	testutil.False(t, gistIdx.IsPrimary, "GIST index should not be primary")
}

func TestSchemaReloadDetectsPostGISChanges(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)
	createSpatialTestSchema(t, ctx, pool)

	// Build cache — should detect geometry columns.
	cache1, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)
	testutil.True(t, cache1.HasPostGIS, "initial cache should have PostGIS")

	places := cache1.Tables["public.places"]
	testutil.NotNil(t, places)
	testutil.True(t, places.HasGeometry(), "places should have geometry in initial cache")

	// Add a new spatial column.
	_, err = pool.Exec(ctx, `ALTER TABLE places ADD COLUMN boundary geometry(Polygon, 4326)`)
	testutil.NoError(t, err)

	// Rebuild cache — should detect the new column.
	cache2, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)

	places2 := cache2.Tables["public.places"]
	testutil.NotNil(t, places2)

	boundaryCol := places2.ColumnByName("boundary")
	testutil.NotNil(t, boundaryCol)
	testutil.True(t, boundaryCol.IsGeometry, "boundary should be geometry")
	testutil.Equal(t, "Polygon", boundaryCol.GeometryType)
	testutil.Equal(t, 4326, boundaryCol.SRID)

	// Drop the spatial column.
	_, err = pool.Exec(ctx, `ALTER TABLE places DROP COLUMN boundary`)
	testutil.NoError(t, err)

	// Rebuild cache — should not have the dropped column.
	cache3, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)

	places3 := cache3.Tables["public.places"]
	testutil.NotNil(t, places3)
	testutil.Nil(t, places3.ColumnByName("boundary"))
}

func TestSpatialColumnsWithoutIndexIntegration(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)
	createSpatialTestSchema(t, ctx, pool)

	_, err := pool.Exec(ctx, `ALTER TABLE places ADD COLUMN unindexed_shape geometry(Polygon, 4326)`)
	testutil.NoError(t, err)

	cache, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)

	places := cache.Tables["public.places"]
	testutil.NotNil(t, places)

	missing := places.SpatialColumnsWithoutIndex()
	testutil.SliceLen(t, missing, 1)
	testutil.Equal(t, "unindexed_shape", missing[0].Name)
}

func TestLookupSRIDIntegration(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)

	info, err := schema.LookupSRID(ctx, pool, 4326)
	testutil.NoError(t, err)
	testutil.NotNil(t, info)
	testutil.Equal(t, "EPSG", info.AuthName)
	testutil.Equal(t, 4326, info.AuthSRID)
	testutil.True(t, info.Description != "", "expected non-empty SRID description")
}

func TestPostGISRasterDetectionAndColumns(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)

	var installedBefore bool
	err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'postgis_raster')`).Scan(&installedBefore)
	testutil.NoError(t, err)

	initialCache, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)
	testutil.Equal(t, installedBefore, initialCache.HasPostGISRaster)

	_, err = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS postgis_raster`)
	if err != nil {
		t.Skipf("postgis_raster not available in this environment: %v", err)
	}

	_, err = pool.Exec(ctx, `CREATE TABLE raster_tiles (id SERIAL PRIMARY KEY, rast raster)`)
	testutil.NoError(t, err)

	cache, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)
	testutil.True(t, cache.HasPostGISRaster, "expected HasPostGISRaster=true after extension install")
	testutil.True(t, cache.PostGISRasterVersion != "", "expected raster version when extension is installed")

	tiles := cache.Tables["public.raster_tiles"]
	testutil.NotNil(t, tiles)
	rastCol := tiles.ColumnByName("rast")
	testutil.NotNil(t, rastCol)
	testutil.True(t, rastCol.IsRaster, "expected rast column to be marked as raster")
	testutil.Equal(t, "string", rastCol.JSONType)
}

func TestPostGISExtensionsListExcludesRaster(t *testing.T) {
	pool := postgisPool(t)
	ctx := context.Background()
	resetPostgisDB(t, ctx, pool)

	cache, err := schema.BuildCache(ctx, pool)
	testutil.NoError(t, err)
	testutil.NotNil(t, cache.PostGISExtensions)

	for _, ext := range cache.PostGISExtensions {
		testutil.False(t, ext == "postgis_raster", "postgis_raster must be represented only via HasPostGISRaster/PostGISRasterVersion")
	}

	var expected []string
	rows, err := pool.Query(ctx, `
		SELECT extname
		FROM pg_extension
		WHERE extname IN ('postgis_topology', 'postgis_sfcgal', 'address_standardizer')
		ORDER BY extname`)
	testutil.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		testutil.NoError(t, err)
		expected = append(expected, name)
	}
	testutil.NoError(t, rows.Err())

	testutil.Equal(t, len(expected), len(cache.PostGISExtensions))
	for i := range expected {
		testutil.Equal(t, expected[i], cache.PostGISExtensions[i])
	}
}
