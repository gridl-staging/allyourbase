package pbmigrate

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // SQLite driver
)

// Reader reads data from a PocketBase SQLite database
type Reader struct {
	db *sql.DB
}

// NewReader creates a new PocketBase reader
func NewReader(sourcePath string) (*Reader, error) {
	// Validate source path
	if _, err := os.Stat(sourcePath); err != nil {
		return nil, fmt.Errorf("source path does not exist: %w", err)
	}

	// Find data.db
	dataPath := filepath.Join(sourcePath, "data.db")
	if _, err := os.Stat(dataPath); err != nil {
		return nil, fmt.Errorf("data.db not found in source path: %w", err)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite", dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &Reader{db: db}, nil
}

// Close closes the database connection
func (r *Reader) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// ReadCollections reads all collections from _collections table
func (r *Reader) ReadCollections() ([]PBCollection, error) {
	schemaColumn, err := r.getCollectionsSchemaColumn()
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
		SELECT id, name, type, system, %s, indexes,
		       listRule, viewRule, createRule, updateRule, deleteRule,
		       options
		FROM _collections
		ORDER BY created
	`, schemaColumn)

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query collections: %w", err)
	}
	defer rows.Close()

	var collections []PBCollection

	for rows.Next() {
		var coll PBCollection
		var schemaJSON, optionsJSON string
		var indexesJSON sql.NullString
		var listRule, viewRule, createRule, updateRule, deleteRule sql.NullString

		err := rows.Scan(
			&coll.ID,
			&coll.Name,
			&coll.Type,
			&coll.System,
			&schemaJSON,
			&indexesJSON,
			&listRule,
			&viewRule,
			&createRule,
			&updateRule,
			&deleteRule,
			&optionsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan collection: %w", err)
		}

		// Parse JSON fields
		if err := json.Unmarshal([]byte(schemaJSON), &coll.Schema); err != nil {
			return nil, fmt.Errorf("failed to parse schema for %s: %w", coll.Name, err)
		}

		if indexesJSON.Valid && indexesJSON.String != "" && indexesJSON.String != "null" {
			if err := json.Unmarshal([]byte(indexesJSON.String), &coll.Indexes); err != nil {
				return nil, fmt.Errorf("failed to parse indexes for %s: %w", coll.Name, err)
			}
		}

		if err := json.Unmarshal([]byte(optionsJSON), &coll.Options); err != nil {
			return nil, fmt.Errorf("failed to parse options for %s: %w", coll.Name, err)
		}

		// Handle nullable rules
		if listRule.Valid {
			coll.ListRule = &listRule.String
		}
		if viewRule.Valid {
			coll.ViewRule = &viewRule.String
		}
		if createRule.Valid {
			coll.CreateRule = &createRule.String
		}
		if updateRule.Valid {
			coll.UpdateRule = &updateRule.String
		}
		if deleteRule.Valid {
			coll.DeleteRule = &deleteRule.String
		}

		// Extract viewQuery for view collections
		if coll.Type == "view" {
			if query, ok := coll.Options["query"].(string); ok {
				coll.ViewQuery = query
			}
		}

		collections = append(collections, coll)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating collections: %w", err)
	}

	return collections, nil
}

func (r *Reader) getCollectionsSchemaColumn() (string, error) {
	rows, err := r.db.Query(`PRAGMA table_info('_collections')`)
	if err != nil {
		return "", fmt.Errorf("failed to inspect _collections table: %w", err)
	}
	defer rows.Close()

	var hasSchema bool
	var hasFields bool

	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return "", fmt.Errorf("failed to scan _collections table info: %w", err)
		}
		switch strings.ToLower(name) {
		case "schema":
			hasSchema = true
		case "fields":
			hasFields = true
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("failed to iterate _collections table info: %w", err)
	}

	if hasSchema {
		return "schema", nil
	}
	if hasFields {
		return "fields", nil
	}
	return "", fmt.Errorf("_collections is missing both schema and fields columns")
}

// ReadRecords reads all records from a collection table
func (r *Reader) ReadRecords(tableName string, schema []PBField) ([]PBRecord, error) {
	query := fmt.Sprintf("SELECT * FROM %s", SanitizeIdentifier(tableName))

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query table %s: %w", tableName, err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var records []PBRecord

	for rows.Next() {
		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Build record
		record := PBRecord{
			Data: make(map[string]interface{}),
		}

		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				val = string(b)
			}

			// Handle special columns
			switch col {
			case "id":
				if s, ok := val.(string); ok {
					record.ID = s
				} else if val != nil {
					record.ID = fmt.Sprint(val)
				}
			case "created":
				record.Data[col] = val
			case "updated":
				record.Data[col] = val
			default:
				record.Data[col] = val
			}
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating records: %w", err)
	}

	return records, nil
}

// CountRecords returns the number of records in a table
func (r *Reader) CountRecords(tableName string) (int, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", SanitizeIdentifier(tableName))

	var count int
	err := r.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count records in %s: %w", tableName, err)
	}

	return count, nil
}
