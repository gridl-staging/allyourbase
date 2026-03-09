package schema

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestPgTypeToJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		typ     string
		isArray bool
		isEnum  bool
		isJSON  bool
		want    string
	}{
		// Arrays always return "array" regardless of base type.
		{name: "integer array", typ: "integer[]", isArray: true, want: "array"},
		{name: "text array", typ: "text[]", isArray: true, want: "array"},
		{name: "json array", typ: "json[]", isArray: true, isJSON: true, want: "array"},

		// JSON types return "object".
		{name: "json flag", typ: "json", isJSON: true, want: "object"},
		{name: "jsonb flag", typ: "jsonb", isJSON: true, want: "object"},
		{name: "json by name", typ: "json", want: "object"},
		{name: "jsonb by name", typ: "jsonb", want: "object"},

		// Enum types return "string".
		{name: "enum", typ: "mood", isEnum: true, want: "string"},
		{name: "custom enum", typ: "status_type", isEnum: true, want: "string"},

		// Boolean types.
		{name: "boolean", typ: "boolean", want: "boolean"},
		{name: "bool", typ: "bool", want: "boolean"},

		// Integer types.
		{name: "smallint", typ: "smallint", want: "integer"},
		{name: "integer", typ: "integer", want: "integer"},
		{name: "bigint", typ: "bigint", want: "integer"},
		{name: "int2", typ: "int2", want: "integer"},
		{name: "int4", typ: "int4", want: "integer"},
		{name: "int8", typ: "int8", want: "integer"},
		{name: "serial", typ: "serial", want: "integer"},
		{name: "bigserial", typ: "bigserial", want: "integer"},
		{name: "smallserial", typ: "smallserial", want: "integer"},
		{name: "serial2", typ: "serial2", want: "integer"},
		{name: "serial4", typ: "serial4", want: "integer"},
		{name: "serial8", typ: "serial8", want: "integer"},
		{name: "oid", typ: "oid", want: "integer"},

		// Float / decimal types.
		{name: "real", typ: "real", want: "number"},
		{name: "double precision", typ: "double precision", want: "number"},
		{name: "float4", typ: "float4", want: "number"},
		{name: "float8", typ: "float8", want: "number"},
		{name: "numeric", typ: "numeric", want: "number"},
		{name: "decimal", typ: "decimal", want: "number"},
		{name: "money", typ: "money", want: "number"},
		{name: "numeric with precision", typ: "numeric(10,2)", want: "number"},
		{name: "decimal with precision", typ: "decimal(18,4)", want: "number"},

		// String types (default case).
		{name: "text", typ: "text", want: "string"},
		{name: "varchar", typ: "character varying", want: "string"},
		{name: "varchar with length", typ: "character varying(255)", want: "string"},
		{name: "char", typ: "character", want: "string"},
		{name: "char with length", typ: "character(10)", want: "string"},
		{name: "uuid", typ: "uuid", want: "string"},
		{name: "date", typ: "date", want: "string"},
		{name: "timestamp", typ: "timestamp without time zone", want: "string"},
		{name: "timestamptz", typ: "timestamp with time zone", want: "string"},
		{name: "time", typ: "time without time zone", want: "string"},
		{name: "timetz", typ: "time with time zone", want: "string"},
		{name: "interval", typ: "interval", want: "string"},
		{name: "bytea", typ: "bytea", want: "string"},
		{name: "inet", typ: "inet", want: "string"},
		{name: "cidr", typ: "cidr", want: "string"},
		{name: "macaddr", typ: "macaddr", want: "string"},
		{name: "point", typ: "point", want: "string"},
		{name: "xml", typ: "xml", want: "string"},

		// Case insensitivity.
		{name: "BOOLEAN uppercase", typ: "BOOLEAN", want: "boolean"},
		{name: "INTEGER uppercase", typ: "INTEGER", want: "integer"},
		{name: "NUMERIC uppercase", typ: "NUMERIC(10,2)", want: "number"},

		// Unknown types default to string.
		{name: "unknown type", typ: "somecustomtype", want: "string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pgTypeToJSON(tt.typ, tt.isArray, tt.isEnum, tt.isJSON)
			testutil.Equal(t, tt.want, got)
		})
	}
}
