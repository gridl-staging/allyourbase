//go:build integration

package graphql_test

import (
	"context"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/graphql"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func postgisGraphQLPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("AYB_TEST_POSTGIS_URL")
	if url == "" {
		t.Skip("AYB_TEST_POSTGIS_URL not set — skipping PostGIS GraphQL integration test")
	}

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connecting to PostGIS database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func setupSpatialGraphQLHandler(t *testing.T, ctx context.Context) *graphql.Handler {
	t.Helper()

	pool := postgisGraphQLPool(t)
	_, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS postgis")
	testutil.NoError(t, err)

	_, err = pool.Exec(ctx, `
		CREATE TABLE places (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			location geometry(Point, 4326)
		);
		CREATE TABLE places_geog (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			location geography(Point, 4326)
		);
		CREATE TABLE notes (
			id SERIAL PRIMARY KEY,
			body TEXT NOT NULL
		);
	`)
	testutil.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO places(name, location) VALUES
			('origin', ST_SetSRID(ST_MakePoint(0, 0), 4326)),
			('east', ST_SetSRID(ST_MakePoint(0.05, 0), 4326)),
			('far', ST_SetSRID(ST_MakePoint(1, 1), 4326));

		INSERT INTO places_geog(name, location) VALUES
			('origin', ST_SetSRID(ST_MakePoint(0, 0), 4326)::geography),
			('near', ST_SetSRID(ST_MakePoint(0.01, 0), 4326)::geography),
			('far', ST_SetSRID(ST_MakePoint(10, 10), 4326)::geography);

		INSERT INTO notes(body) VALUES ('a'), ('b');
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	cacheHolder := schema.NewCacheHolder(pool, logger)
	testutil.NoError(t, cacheHolder.Load(ctx))

	return graphql.NewHandler(pool, cacheHolder, logger)
}

func TestSpatialIntegrationNearGeometryReturnsGeoJSONObjects(t *testing.T) {
	ctx := context.Background()
	h := setupSpatialGraphQLHandler(t, ctx)

	_, resp := doGQL(t, h, `
		{
			places(
				order_by: { id: ASC }
				near: { column: "location", longitude: 0, latitude: 0, distance: 0.1 }
			) {
				name
				location
			}
		}
	`, nil)
	testutil.Equal(t, 0, len(resp.Errors))

	rows := resp.Data["places"].([]interface{})
	testutil.Equal(t, 2, len(rows))
	first := rows[0].(map[string]interface{})
	_, isString := first["location"].(string)
	testutil.False(t, isString, "location must be structured GeoJSON object, not string")

	location, ok := first["location"].(map[string]interface{})
	testutil.True(t, ok, "location should decode as object")
	testutil.Equal(t, "Point", location["type"])
}

func TestSpatialIntegrationNearGeographyUsesMeterDistance(t *testing.T) {
	ctx := context.Background()
	h := setupSpatialGraphQLHandler(t, ctx)

	_, resp := doGQL(t, h, `
		{
			places_geog(
				order_by: { id: ASC }
				near: { column: "location", longitude: 0, latitude: 0, distance: 2000 }
			) {
				name
			}
		}
	`, nil)
	testutil.Equal(t, 0, len(resp.Errors))

	rows := resp.Data["places_geog"].([]interface{})
	testutil.Equal(t, 2, len(rows))
	testutil.Equal(t, "origin", rows[0].(map[string]interface{})["name"])
	testutil.Equal(t, "near", rows[1].(map[string]interface{})["name"])
}

func TestSpatialIntegrationNearWithWhereAppliesANDSemantics(t *testing.T) {
	ctx := context.Background()
	h := setupSpatialGraphQLHandler(t, ctx)

	_, resp := doGQL(t, h, `
		{
			places(
				where: { name: { _eq: "origin" } }
				near: { column: "location", longitude: 0, latitude: 0, distance: 0.1 }
			) {
				name
			}
		}
	`, nil)
	testutil.Equal(t, 0, len(resp.Errors))

	rows := resp.Data["places"].([]interface{})
	testutil.Equal(t, 1, len(rows))
	testutil.Equal(t, "origin", rows[0].(map[string]interface{})["name"])
}

func TestSpatialIntegrationBBoxFiltersRows(t *testing.T) {
	ctx := context.Background()
	h := setupSpatialGraphQLHandler(t, ctx)

	_, resp := doGQL(t, h, `
		{
			places(
				order_by: { id: ASC }
				bbox: { column: "location", minLng: -0.1, minLat: -0.1, maxLng: 0.1, maxLat: 0.1 }
			) {
				name
			}
		}
	`, nil)
	testutil.Equal(t, 0, len(resp.Errors))

	rows := resp.Data["places"].([]interface{})
	testutil.Equal(t, 2, len(rows))
	testutil.Equal(t, "origin", rows[0].(map[string]interface{})["name"])
	testutil.Equal(t, "east", rows[1].(map[string]interface{})["name"])
}

func TestSpatialIntegrationSchemaGatesSpatialArgs(t *testing.T) {
	ctx := context.Background()
	h := setupSpatialGraphQLHandler(t, ctx)

	_, resp := doGQL(t, h, `
		{
			__schema {
				queryType {
					fields {
						name
						args { name }
					}
				}
			}
		}
	`, nil)
	testutil.Equal(t, 0, len(resp.Errors))

	fields := resp.Data["__schema"].(map[string]interface{})["queryType"].(map[string]interface{})["fields"].([]interface{})
	fieldArgs := make(map[string]map[string]bool)
	for _, fieldValue := range fields {
		fieldMap := fieldValue.(map[string]interface{})
		name := fieldMap["name"].(string)
		argNames := make(map[string]bool)
		for _, argValue := range fieldMap["args"].([]interface{}) {
			argMap := argValue.(map[string]interface{})
			argNames[argMap["name"].(string)] = true
		}
		fieldArgs[name] = argNames
	}

	testutil.True(t, fieldArgs["places"]["near"], "places should expose near")
	testutil.True(t, fieldArgs["places"]["within"], "places should expose within")
	testutil.True(t, fieldArgs["places"]["bbox"], "places should expose bbox")
	testutil.False(t, fieldArgs["notes"]["near"], "notes should not expose near")
	testutil.False(t, fieldArgs["notes"]["within"], "notes should not expose within")
	testutil.False(t, fieldArgs["notes"]["bbox"], "notes should not expose bbox")
}

func TestSpatialIntegrationSchemaGatingWithoutPostGISEvenForSpatialTable(t *testing.T) {
	cache := &schema.SchemaCache{
		HasPostGIS: false,
		Schemas:    []string{"public"},
		Tables: map[string]*schema.Table{
			"public.places": {
				Schema: "public",
				Name:   "places",
				Kind:   "table",
				Columns: []*schema.Column{
					{Name: "id", TypeName: "integer", IsPrimaryKey: true},
					{Name: "location", TypeName: "geometry", IsGeometry: true, GeometryType: "Point"},
				},
				PrimaryKey: []string{"id"},
			},
		},
	}

	graphSchema, err := graphql.BuildSchemaWithFactories(cache, nil, nil, nil)
	testutil.NoError(t, err)

	field := graphSchema.QueryType().Fields()["places"]
	testutil.NotNil(t, field)
	argNames := make(map[string]bool, len(field.Args))
	for _, arg := range field.Args {
		argNames[arg.Name()] = true
	}
	testutil.False(t, argNames["near"], "near should not be present when PostGIS is disabled")
	testutil.False(t, argNames["within"], "within should not be present when PostGIS is disabled")
	testutil.False(t, argNames["bbox"], "bbox should not be present when PostGIS is disabled")
}
