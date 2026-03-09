package schema

import "strings"

// pgTypeToJSON maps a PostgreSQL type name (from format_type()) to a JSON type string.
// Returns one of: "string", "integer", "number", "boolean", "object", "array".
func pgTypeToJSON(typeName string, isArray bool, isEnum bool, isJSON bool) string {
	if isArray {
		return "array"
	}
	if isJSON {
		return "object"
	}
	if isEnum {
		return "string"
	}

	// Normalize: strip modifiers like (255), (10,2) for matching.
	base := strings.ToLower(typeName)
	if idx := strings.Index(base, "("); idx > 0 {
		base = strings.TrimSpace(base[:idx])
	}
	// Strip trailing [] if present (shouldn't happen since isArray is checked above).
	base = strings.TrimSuffix(base, "[]")

	switch base {
	// Boolean
	case "boolean", "bool":
		return "boolean"

	// Integer types
	case "smallint", "integer", "bigint",
		"int2", "int4", "int8",
		"serial", "bigserial", "smallserial",
		"serial2", "serial4", "serial8",
		"oid":
		return "integer"

	// Float / decimal types
	case "real", "double precision", "float4", "float8",
		"numeric", "decimal", "money":
		return "number"

	// JSON/JSONB pass-through
	case "json", "jsonb":
		return "object"

	// All remaining types serialize as JSON strings:
	// text, varchar, char, uuid, date, timestamp, timestamptz,
	// time, timetz, interval, bytea, inet, cidr, macaddr,
	// point, line, polygon, circle, box, path, xml, etc.
	default:
		return "string"
	}
}
