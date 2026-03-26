//go:build integration

package openapi_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/openapi"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func execSQL(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatalf("exec SQL: %v\nSQL: %s", err, sql)
	}
}

func TestIntegration_OpenAPIFromLiveSchema(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	t.Cleanup(func() {
		execSQL(t, pool, `DROP TABLE IF EXISTS oa_test_items CASCADE`)
		execSQL(t, pool, `DROP TYPE IF EXISTS oa_test_priority CASCADE`)
	})

	// Create a table with varied column types.
	execSQL(t, pool, `CREATE TYPE oa_test_priority AS ENUM ('low', 'medium', 'high')`)
	execSQL(t, pool, `
		CREATE TABLE oa_test_items (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			name text NOT NULL,
			priority oa_test_priority DEFAULT 'medium',
			metadata jsonb,
			score numeric(10,2),
			created_at timestamptz DEFAULT now(),
			tags text[]
		)
	`)

	// Build schema cache from live DB.
	sc, err := schema.BuildCache(ctx, pool)
	if err != nil {
		t.Fatalf("BuildCache: %v", err)
	}

	// Generate spec.
	data, err := openapi.Generate(sc, openapi.Options{Title: "Integration Test API"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Validate JSON.
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Verify OpenAPI version.
	if doc["openapi"] != "3.1.0" {
		t.Errorf("openapi = %v, want 3.1.0", doc["openapi"])
	}

	// Verify paths include our test table.
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths is not an object: %T", doc["paths"])
	}
	if _, ok := paths["/oa_test_items"]; !ok {
		t.Error("expected /oa_test_items path")
	}

	// Add a column and regenerate.
	execSQL(t, pool, `ALTER TABLE oa_test_items ADD COLUMN description text`)

	sc2, err := schema.BuildCache(ctx, pool)
	if err != nil {
		t.Fatalf("BuildCache after alter: %v", err)
	}

	data2, err := openapi.Generate(sc2, openapi.Options{})
	if err != nil {
		t.Fatalf("Generate after alter: %v", err)
	}

	var doc2 map[string]any
	if err := json.Unmarshal(data2, &doc2); err != nil {
		t.Fatalf("invalid JSON after alter: %v", err)
	}

	paths2, ok := doc2["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths after alter is not an object: %T", doc2["paths"])
	}
	tablePath, ok := paths2["/oa_test_items"].(map[string]any)
	if !ok {
		t.Fatal("missing /oa_test_items path after alter")
	}
	getOp, ok := tablePath["get"].(map[string]any)
	if !ok {
		t.Fatal("missing GET operation for /oa_test_items after alter")
	}
	resp, ok := getOp["responses"].(map[string]any)["200"].(map[string]any)
	if !ok {
		t.Fatal("missing 200 response for /oa_test_items GET")
	}
	content, ok := resp["content"].(map[string]any)["application/json"].(map[string]any)
	if !ok {
		t.Fatal("missing application/json response content")
	}
	schemaObj, ok := content["schema"].(map[string]any)
	if !ok {
		t.Fatal("response schema is not an object")
	}
	itemsSchema, ok := schemaObj["items"].(map[string]any)
	if !ok {
		t.Fatal("response schema items is not an object")
	}
	ref, ok := itemsSchema["$ref"].(string)
	if !ok || ref == "" {
		t.Fatalf("response items missing $ref: %v", itemsSchema)
	}
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		t.Fatalf("unexpected response items $ref: %s", ref)
	}
	schemaName := strings.TrimPrefix(ref, prefix)
	components, ok := doc2["components"].(map[string]any)
	if !ok {
		t.Fatal("components is not an object")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("components.schemas is not an object")
	}
	rowSchema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		t.Fatalf("missing referenced schema %q", schemaName)
	}
	props, ok := rowSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("referenced schema %q has no properties object", schemaName)
	}

	if _, ok := props["description"]; !ok {
		t.Error("regenerated spec should include newly added 'description' column")
	}
}
