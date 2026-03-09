package fbmigrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestParseFirestoreExport(t *testing.T) {
	t.Parallel()
	t.Run("valid export with two collections", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// users.json
		users := []FirestoreDocument{
			{ID: "projects/p/databases/(default)/documents/users/u1", Fields: map[string]any{"name": map[string]any{"stringValue": "Alice"}}},
			{ID: "projects/p/databases/(default)/documents/users/u2", Fields: map[string]any{"name": map[string]any{"stringValue": "Bob"}}},
		}
		writeJSON(t, filepath.Join(dir, "users.json"), users)

		// posts.json
		posts := []FirestoreDocument{
			{ID: "projects/p/databases/(default)/documents/posts/p1", Fields: map[string]any{"title": map[string]any{"stringValue": "Hello"}}},
		}
		writeJSON(t, filepath.Join(dir, "posts.json"), posts)

		collections, err := ParseFirestoreExport(dir)
		testutil.NoError(t, err)
		testutil.Equal(t, 2, len(collections))

		// Collections are sorted by filename.
		testutil.Equal(t, "posts", collections[0].Name)
		testutil.Equal(t, 1, len(collections[0].Documents))
		testutil.Equal(t, "p1", collections[0].Documents[0].ID)

		testutil.Equal(t, "users", collections[1].Name)
		testutil.Equal(t, 2, len(collections[1].Documents))
		testutil.Equal(t, "u1", collections[1].Documents[0].ID)
		testutil.Equal(t, "u2", collections[1].Documents[1].ID)
	})

	t.Run("empty directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		collections, err := ParseFirestoreExport(dir)
		testutil.NoError(t, err)
		testutil.Equal(t, 0, len(collections))
	})

	t.Run("skips non-json files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0644)
		testutil.NoError(t, err)
		writeJSON(t, filepath.Join(dir, "data.json"), []FirestoreDocument{})

		collections, err := ParseFirestoreExport(dir)
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(collections))
		testutil.Equal(t, "data", collections[0].Name)
	})

	t.Run("skips subdirectories", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		err := os.Mkdir(filepath.Join(dir, "subdir.json"), 0755)
		testutil.NoError(t, err)

		collections, err := ParseFirestoreExport(dir)
		testutil.NoError(t, err)
		testutil.Equal(t, 0, len(collections))
	})

	t.Run("invalid directory", func(t *testing.T) {
		t.Parallel()
		_, err := ParseFirestoreExport("/nonexistent/dir")
		testutil.NotNil(t, err)
	})

	t.Run("invalid json file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json"), 0644)
		testutil.NoError(t, err)
		_, err = ParseFirestoreExport(dir)
		testutil.ErrorContains(t, err, "parsing collection bad")
	})
}

func TestCreateCollectionTableSQL(t *testing.T) {
	t.Parallel()
	sql := CreateCollectionTableSQL("users")
	testutil.Contains(t, sql, `CREATE TABLE IF NOT EXISTS "users"`)
	testutil.Contains(t, sql, `"id" text NOT NULL`)
	testutil.Contains(t, sql, `"data" jsonb NOT NULL`)
	testutil.Contains(t, sql, `"created_at" timestamptz`)
	testutil.Contains(t, sql, `"updated_at" timestamptz`)
	testutil.Contains(t, sql, `PRIMARY KEY ("id")`)
}

func TestCreateCollectionIndexSQL(t *testing.T) {
	t.Parallel()
	sql := CreateCollectionIndexSQL("users")
	testutil.Contains(t, sql, `CREATE INDEX IF NOT EXISTS "idx_users_data"`)
	testutil.Contains(t, sql, `ON "users" USING GIN ("data")`)
}

func TestNormalizeCollectionName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"users", "users"},
		{"users/orders", "users_orders"},
		{"a/b/c", "a_b_c"},
		{"simple", "simple"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, NormalizeCollectionName(tt.input))
		})
	}
}

func TestFlattenFirestoreValue(t *testing.T) {
	t.Parallel()
	t.Run("string value", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{"stringValue": "hello"})
		testutil.Equal(t, "hello", v)
	})

	t.Run("integer value", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{"integerValue": "42"})
		testutil.Equal(t, "42", v)
	})

	t.Run("double value", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{"doubleValue": 3.14})
		testutil.Equal(t, 3.14, v)
	})

	t.Run("boolean value", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{"booleanValue": true})
		testutil.Equal(t, true, v)
	})

	t.Run("null value nil", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{"nullValue": nil})
		testutil.Nil(t, v)
	})

	t.Run("null value NULL_VALUE string", func(t *testing.T) {
		// Firestore exports represent null as {"nullValue": "NULL_VALUE"}
		t.Parallel()

		v := FlattenFirestoreValue(map[string]any{"nullValue": "NULL_VALUE"})
		testutil.Nil(t, v)
	})

	t.Run("timestamp value", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{"timestampValue": "2024-01-01T00:00:00Z"})
		testutil.Equal(t, "2024-01-01T00:00:00Z", v)
	})

	t.Run("reference value", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{"referenceValue": "projects/p/databases/d/documents/c/id"})
		testutil.Equal(t, "projects/p/databases/d/documents/c/id", v)
	})

	t.Run("array value", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{
			"arrayValue": map[string]any{
				"values": []any{
					map[string]any{"stringValue": "a"},
					map[string]any{"integerValue": "1"},
				},
			},
		})
		arr, ok := v.([]any)
		testutil.True(t, ok, "should be array")
		testutil.Equal(t, 2, len(arr))
		testutil.Equal(t, "a", arr[0])
		testutil.Equal(t, "1", arr[1])
	})

	t.Run("empty array", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{
			"arrayValue": map[string]any{},
		})
		arr, ok := v.([]any)
		testutil.True(t, ok, "should be empty array")
		testutil.Equal(t, 0, len(arr))
	})

	t.Run("map value", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{
			"mapValue": map[string]any{
				"fields": map[string]any{
					"name": map[string]any{"stringValue": "Alice"},
					"age":  map[string]any{"integerValue": "30"},
				},
			},
		})
		m, ok := v.(map[string]any)
		testutil.True(t, ok, "should be map")
		testutil.Equal(t, "Alice", m["name"])
		testutil.Equal(t, "30", m["age"])
	})

	t.Run("geoPoint value", func(t *testing.T) {
		t.Parallel()
		geo := map[string]any{"latitude": 40.7128, "longitude": -74.006}
		v := FlattenFirestoreValue(map[string]any{"geoPointValue": geo})
		m, ok := v.(map[string]any)
		testutil.True(t, ok, "should return geoPoint map")
		testutil.Equal(t, 40.7128, m["latitude"])
		testutil.Equal(t, -74.006, m["longitude"])
	})

	t.Run("empty map value", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{
			"mapValue": map[string]any{},
		})
		m, ok := v.(map[string]any)
		testutil.True(t, ok, "should be empty map")
		testutil.Equal(t, 0, len(m))
	})

	t.Run("non-map passthrough", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue("plain string")
		testutil.Equal(t, "plain string", v)
	})

	t.Run("unknown typed value passthrough", func(t *testing.T) {
		t.Parallel()
		v := FlattenFirestoreValue(map[string]any{"unknownType": "foo"})
		m, ok := v.(map[string]any)
		testutil.True(t, ok, "should return original map")
		testutil.Equal(t, "foo", m["unknownType"])
	})
}

func TestFlattenFirestoreFields(t *testing.T) {
	t.Parallel()
	fields := map[string]any{
		"name":   map[string]any{"stringValue": "Alice"},
		"age":    map[string]any{"integerValue": "30"},
		"active": map[string]any{"booleanValue": true},
		"address": map[string]any{
			"mapValue": map[string]any{
				"fields": map[string]any{
					"city":  map[string]any{"stringValue": "NYC"},
					"state": map[string]any{"stringValue": "NY"},
				},
			},
		},
	}

	result := FlattenFirestoreFields(fields)
	testutil.Equal(t, "Alice", result["name"])
	testutil.Equal(t, "30", result["age"])
	testutil.Equal(t, true, result["active"])

	addr, ok := result["address"].(map[string]any)
	testutil.True(t, ok, "address should be map")
	testutil.Equal(t, "NYC", addr["city"])
	testutil.Equal(t, "NY", addr["state"])
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	testutil.NoError(t, err)
	err = os.WriteFile(path, data, 0644)
	testutil.NoError(t, err)
}
