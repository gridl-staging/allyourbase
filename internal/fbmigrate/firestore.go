package fbmigrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParseFirestoreExport reads a Firestore export directory.
// Each .json file in the directory represents a collection.
// The file name (without extension) becomes the collection name.
// Subcollections detected by "/" in document IDs are namespaced with "_"
// (e.g., "users/123/orders" → collection "users_orders").
func ParseFirestoreExport(dir string) ([]FirestoreCollection, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading export directory: %w", err)
	}

	var collections []FirestoreCollection
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".json")
		path := filepath.Join(dir, entry.Name())

		docs, err := parseCollectionFile(path)
		if err != nil {
			return nil, fmt.Errorf("parsing collection %s: %w", name, err)
		}

		collections = append(collections, FirestoreCollection{
			Name:      name,
			Documents: docs,
		})
	}

	return collections, nil
}

// parseCollectionFile reads a single Firestore collection JSON file.
// Expected format: array of documents, each with an "__name__" field and "fields" object.
func parseCollectionFile(path string) ([]FirestoreDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var docs []FirestoreDocument
	if err := json.Unmarshal(data, &docs); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	// Extract document IDs from the __name__ field.
	for i := range docs {
		if docs[i].ID != "" {
			// __name__ is typically "projects/{project}/databases/(default)/documents/{collection}/{docId}"
			// Extract just the last segment as the ID.
			parts := strings.Split(docs[i].ID, "/")
			if len(parts) > 0 {
				docs[i].ID = parts[len(parts)-1]
			}
		}
	}

	return docs, nil
}

// CreateCollectionTableSQL generates CREATE TABLE DDL for a Firestore collection.
// Each collection becomes a table with (id TEXT PK, data JSONB, created_at, updated_at).
func CreateCollectionTableSQL(name string) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %q (
  "id" text NOT NULL,
  "data" jsonb NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id")
);`, name)
}

// CreateCollectionIndexSQL generates a GIN index on the data column.
func CreateCollectionIndexSQL(name string) string {
	return fmt.Sprintf(`CREATE INDEX IF NOT EXISTS "idx_%s_data" ON %q USING GIN ("data");`, name, name)
}

// NormalizeCollectionName converts a subcollection path to a table name.
// "users/orders" → "users_orders"
func NormalizeCollectionName(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}

// FlattenFirestoreValue converts Firestore typed values to plain Go values.
// Firestore exports use typed wrappers like {"stringValue": "hello"}, {"integerValue": "42"}.
func FlattenFirestoreValue(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}

	if sv, ok := m["stringValue"]; ok {
		return sv
	}
	if iv, ok := m["integerValue"]; ok {
		return iv
	}
	if dv, ok := m["doubleValue"]; ok {
		return dv
	}
	if bv, ok := m["booleanValue"]; ok {
		return bv
	}
	if _, ok := m["nullValue"]; ok {
		return nil
	}
	if tv, ok := m["timestampValue"]; ok {
		return tv
	}
	if ref, ok := m["referenceValue"]; ok {
		return ref
	}
	if gv, ok := m["geoPointValue"]; ok {
		return gv
	}

	// Array value.
	if av, ok := m["arrayValue"]; ok {
		if avMap, ok := av.(map[string]any); ok {
			if values, ok := avMap["values"].([]any); ok {
				result := make([]any, len(values))
				for i, v := range values {
					result[i] = FlattenFirestoreValue(v)
				}
				return result
			}
		}
		return []any{}
	}

	// Map value.
	if mv, ok := m["mapValue"]; ok {
		if mvMap, ok := mv.(map[string]any); ok {
			if fields, ok := mvMap["fields"].(map[string]any); ok {
				return FlattenFirestoreFields(fields)
			}
		}
		return map[string]any{}
	}

	// Unknown type — return as-is.
	return m
}

// FlattenFirestoreFields converts a Firestore fields map to plain Go values.
func FlattenFirestoreFields(fields map[string]any) map[string]any {
	result := make(map[string]any, len(fields))
	for k, v := range fields {
		result[k] = FlattenFirestoreValue(v)
	}
	return result
}
