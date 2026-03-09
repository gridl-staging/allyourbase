package pbmigrate

import "fmt"

// FieldTypeToPgType converts a PocketBase field type to PostgreSQL column type
func FieldTypeToPgType(field PBField) string {
	switch field.Type {
	case "text", "email", "url", "editor":
		return "TEXT"

	case "number":
		return "DOUBLE PRECISION"

	case "bool":
		return "BOOLEAN"

	case "date":
		return "TIMESTAMP WITH TIME ZONE"

	case "select":
		// Check if maxSelect > 1 (multiple selection)
		if maxSelect := fieldMaxSelect(field); maxSelect > 1 {
			return "TEXT[]" // array for multiple select
		}
		return "TEXT" // single select

	case "json":
		return "JSONB"

	case "file":
		// Check if maxSelect > 1 (multiple files)
		if maxSelect := fieldMaxSelect(field); maxSelect > 1 {
			return "TEXT[]" // array of filenames
		}
		return "TEXT" // single filename

	case "relation":
		// Check if maxSelect > 1 (multiple relations)
		if maxSelect := fieldMaxSelect(field); maxSelect > 1 {
			return "TEXT[]" // array of IDs
		}
		return "TEXT" // single ID

	default:
		return "TEXT" // fallback to text for unknown types
	}
}

func fieldMaxSelect(field PBField) float64 {
	if field.MaxSelect > 0 {
		return field.MaxSelect
	}
	if field.Options == nil {
		return 0
	}
	if maxSelect, ok := field.Options["maxSelect"].(float64); ok {
		return maxSelect
	}
	return 0
}

// BuildCreateTableSQL generates CREATE TABLE statement for a collection
func BuildCreateTableSQL(coll PBCollection) string {
	tableName := SanitizeIdentifier(coll.Name)

	sql := fmt.Sprintf("CREATE TABLE %s (\n", tableName)

	// System fields (always present)
	sql += "  id TEXT PRIMARY KEY,\n"
	sql += "  created TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),\n"
	sql += "  updated TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()"

	// Add custom fields
	for _, field := range coll.Schema {
		if field.System {
			continue // skip system fields
		}

		pgType := FieldTypeToPgType(field)
		fieldName := SanitizeIdentifier(field.Name)

		sql += fmt.Sprintf(",\n  %s %s", fieldName, pgType)

		if field.Required {
			sql += " NOT NULL"
		}

		if field.Unique {
			sql += " UNIQUE"
		}
	}

	sql += "\n);"

	return sql
}

// BuildCreateViewSQL generates CREATE VIEW statement for a view collection
func BuildCreateViewSQL(coll PBCollection) string {
	viewName := SanitizeIdentifier(coll.Name)
	query := coll.ViewQuery

	// PocketBase view queries are already valid SQL
	return fmt.Sprintf("CREATE VIEW %s AS %s;", viewName, query)
}

// SanitizeIdentifier ensures a PostgreSQL identifier is safe
func SanitizeIdentifier(name string) string {
	// For now, just quote the identifier to handle special characters
	// In production, we might also check for reserved words
	return `"` + name + `"`
}

// IsReservedWord checks if a name is a PostgreSQL reserved word
func IsReservedWord(name string) bool {
	// Simplified list of common PostgreSQL reserved words
	reserved := map[string]bool{
		"user": true, "table": true, "select": true, "where": true,
		"from": true, "order": true, "group": true, "limit": true,
		"offset": true, "join": true, "union": true, "all": true,
	}
	return reserved[name]
}
