package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/jackc/pgx/v5"
)

// countKnownColumns returns the number of keys in data that match a column in the table schema.
func countKnownColumns(tbl *schema.Table, data map[string]any) int {
	n := 0
	for col := range data {
		if tbl.ColumnByName(col) != nil {
			n++
		}
	}
	return n
}

// parseFields extracts the fields query parameter.
func parseFields(r *http.Request) []string {
	f := r.URL.Query().Get("fields")
	if f == "" {
		return nil
	}
	parts := strings.Split(f, ",")
	fields := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			fields = append(fields, p)
		}
	}
	return fields
}

// parseSortSQL converts the sort parameter to a SQL ORDER BY clause.
// Format: "-created,+name" → "created" DESC, "name" ASC
func parseSortSQL(tbl *schema.Table, sortParam string) string {
	parsed, err := parseStructuredSort(tbl, sortParam, true)
	if err != nil {
		return ""
	}
	return sortFieldsToSQL(plainSortFieldsFromStructuredSort(parsed))
}

// scanRow scans a single row from a pgx.Rows result using field descriptions.
// Returns nil if no rows are present.
func scanRow(rows pgx.Rows) (map[string]any, error) {
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	return scanCurrentRow(rows)
}

// scanRows scans all rows from a pgx.Rows result.
func scanRows(rows pgx.Rows) ([]map[string]any, error) {
	var result []map[string]any

	for rows.Next() {
		record, err := scanCurrentRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if result == nil {
		result = []map[string]any{}
	}
	return result, nil
}

// scanCurrentRow scans the current row into a map.
func scanCurrentRow(rows pgx.Rows) (map[string]any, error) {
	descs := rows.FieldDescriptions()
	values := make([]any, len(descs))
	ptrs := make([]any, len(descs))
	for i := range values {
		ptrs[i] = &values[i]
	}

	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}

	record := make(map[string]any, len(descs))
	for i, desc := range descs {
		record[desc.Name] = normalizeValue(values[i])
	}
	return record, nil
}

// normalizeValue converts pgx binary-protocol types into JSON-friendly forms.
// In particular, UUID columns scanned into `any` arrive as [16]byte; we convert
// them to the standard "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" string.
func normalizeValue(v any) any {
	switch val := v.(type) {
	case [16]byte:
		return fmt.Sprintf("%x-%x-%x-%x-%x", val[0:4], val[4:6], val[6:8], val[8:10], val[10:16])
	default:
		return v
	}
}

// pkMap extracts primary key field values from a scanned record map.
func pkMap(tbl *schema.Table, record map[string]any) map[string]any {
	m := make(map[string]any, len(tbl.PrimaryKey))
	for _, pk := range tbl.PrimaryKey {
		m[pk] = record[pk]
	}
	return m
}

// extractOldRecord removes and returns the _audit_old_values sentinel column
// (injected by buildUpdateWithAudit) as a map[string]any. The column is deleted
// from record so it won't appear in the API response. Handles both map[string]any
// (pgx auto-unmarshal) and []byte/string (raw JSON) forms.
func extractOldRecord(record map[string]any) map[string]any {
	if record == nil {
		return nil
	}
	v, ok := record["_audit_old_values"]
	if !ok {
		return nil
	}
	delete(record, "_audit_old_values")
	switch val := v.(type) {
	case map[string]any:
		return val
	case []byte:
		var m map[string]any
		if json.Unmarshal(val, &m) == nil {
			return m
		}
	case string:
		var m map[string]any
		if json.Unmarshal([]byte(val), &m) == nil {
			return m
		}
	}
	return nil
}
