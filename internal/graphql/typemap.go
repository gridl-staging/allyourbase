// Package graphql typemap.go provides functions for converting PostgreSQL column types to their corresponding GraphQL types, with support for enums, arrays, and type name sanitization.
package graphql

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	gql "github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/spatial"
)

var JSONScalar = gql.NewScalar(gql.ScalarConfig{
	Name:        "JSON",
	Description: "Arbitrary JSON value",
	Serialize: func(value interface{}) interface{} {
		return value
	},
	ParseValue: func(value interface{}) interface{} {
		return value
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		return parseJSONLiteral(valueAST)
	},
})

var GeoJSONScalar = gql.NewScalar(gql.ScalarConfig{
	Name:        "GeoJSON",
	Description: "GeoJSON geometry object",
	Serialize: func(value interface{}) interface{} {
		geometry, ok := serializeGeoJSONGeometryValue(value)
		if !ok {
			return nil
		}
		return geometry
	},
	ParseValue: func(value interface{}) interface{} {
		geometry, ok := parseGeoJSONInputObject(value)
		if !ok {
			return nil
		}
		return geometry
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		parsed := parseJSONLiteral(valueAST)
		geometry, ok := parseGeoJSONInputObject(parsed)
		if !ok {
			return nil
		}
		return geometry
	},
})

func serializeGeoJSONGeometryValue(value interface{}) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if !isValidGeoJSONGeometryMap(typed) {
			return nil, false
		}
		return typed, true
	case string:
		return decodeAndValidateGeoJSONGeometry(typed)
	case []byte:
		return decodeAndValidateGeoJSONGeometry(string(typed))
	default:
		return nil, false
	}
}

func parseGeoJSONInputObject(value interface{}) (map[string]any, bool) {
	geometry, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	if !isValidGeoJSONGeometryMap(geometry) {
		return nil, false
	}
	return geometry, true
}

func decodeAndValidateGeoJSONGeometry(raw string) (map[string]any, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}

	var geometry map[string]any
	if err := json.Unmarshal([]byte(raw), &geometry); err != nil {
		return nil, false
	}
	if !isValidGeoJSONGeometryMap(geometry) {
		return nil, false
	}
	return geometry, true
}

func isValidGeoJSONGeometryMap(geometry map[string]any) bool {
	if len(geometry) == 0 {
		return false
	}
	payload, err := json.Marshal(geometry)
	if err != nil {
		return false
	}
	_, err = spatial.ParseGeoJSONGeometry(string(payload))
	return err == nil
}

// parseJSONLiteral recursively converts a GraphQL AST value node to a Go value, handling objects as maps, lists as slices, and scalar values as their corresponding Go types.
func parseJSONLiteral(v ast.Value) interface{} {
	switch node := v.(type) {
	case *ast.ObjectValue:
		out := map[string]interface{}{}
		for _, field := range node.Fields {
			out[field.Name.Value] = parseJSONLiteral(field.Value)
		}
		return out
	case *ast.ListValue:
		out := make([]interface{}, 0, len(node.Values))
		for _, value := range node.Values {
			out = append(out, parseJSONLiteral(value))
		}
		return out
	case *ast.StringValue:
		return node.Value
	case *ast.BooleanValue:
		return node.Value
	case *ast.IntValue:
		parsedValue, err := strconv.ParseInt(node.Value, 10, 64)
		if err != nil {
			return nil
		}
		return parsedValue
	case *ast.FloatValue:
		parsedValue, err := strconv.ParseFloat(node.Value, 64)
		if err != nil {
			return nil
		}
		return parsedValue
	case *ast.EnumValue:
		return node.Value
	default:
		return nil
	}
}

var (
	enumTypeCache   = map[string]*gql.Enum{}
	enumTypeCacheMu sync.Mutex
)

func resetEnumTypeCacheUnsafe() {
	enumTypeCacheMu.Lock()
	defer enumTypeCacheMu.Unlock()
	enumTypeCache = map[string]*gql.Enum{}
}

// pgToGraphQL maps a PostgreSQL column type to its corresponding GraphQL output type, handling enums, arrays, JSON, geometry, vector types, and standard SQL types.
func pgToGraphQL(col *schema.Column) gql.Output {
	if col == nil {
		return gql.String
	}
	if col.IsEnum && len(col.EnumValues) > 0 {
		return enumTypeForColumn(col)
	}
	if col.IsArray {
		inner := &schema.Column{
			Name:       col.Name,
			TypeName:   strings.TrimSuffix(strings.ToLower(strings.TrimSpace(col.TypeName)), "[]"),
			IsJSON:     false,
			IsEnum:     false,
			IsArray:    false,
			IsGeometry: false,
			IsVector:   false,
		}
		return gql.NewList(pgToGraphQL(inner))
	}
	if col.IsJSON {
		return JSONScalar
	}
	if col.IsGeometry || col.IsGeography {
		return GeoJSONScalar
	}
	if col.IsVector {
		return gql.String
	}
	return pgTypeNameToGraphQL(col.TypeName)
}

// pgTypeNameToGraphQL converts a PostgreSQL type name string to the corresponding GraphQL output type, supporting integer, float, boolean, timestamp, and JSON types, defaulting to String.
func pgTypeNameToGraphQL(typeName string) gql.Output {
	t := strings.ToLower(strings.TrimSpace(typeName))

	switch {
	case t == "boolean" || t == "bool":
		return gql.Boolean
	case t == "smallint" || t == "int2" ||
		t == "integer" || t == "int" || t == "int4" ||
		t == "bigint" || t == "int8" ||
		t == "serial" || t == "serial4" ||
		t == "smallserial" || t == "serial2" ||
		t == "bigserial" || t == "serial8" ||
		t == "oid":
		return gql.Int
	case t == "real" || t == "float4" ||
		t == "double precision" || t == "float8" || t == "float" ||
		t == "numeric" || t == "decimal" || t == "money" ||
		strings.HasPrefix(t, "numeric(") || strings.HasPrefix(t, "decimal("):
		return gql.Float
	case t == "timestamp" || t == "timestamp without time zone" ||
		t == "timestamptz" || t == "timestamp with time zone":
		return gql.DateTime
	case t == "json" || t == "jsonb":
		return JSONScalar
	default:
		return gql.String
	}
}

// enumTypeForColumn creates or retrieves a cached GraphQL enum type for the given column, deriving the type name from the column's type or name and sanitizing it to be GraphQL-compliant.
func enumTypeForColumn(col *schema.Column) *gql.Enum {
	nameBase := col.TypeName
	if nameBase == "" {
		nameBase = col.Name
	}
	typeName := sanitizeTypeName(nameBase) + "Enum"

	enumTypeCacheMu.Lock()
	defer enumTypeCacheMu.Unlock()
	if cached, ok := enumTypeCache[typeName]; ok {
		return cached
	}

	values := gql.EnumValueConfigMap{}
	for _, value := range col.EnumValues {
		values[value] = &gql.EnumValueConfig{Value: value}
	}
	enumType := gql.NewEnum(gql.EnumConfig{Name: typeName, Values: values})
	enumTypeCache[typeName] = enumType
	return enumType
}

// sanitizeTypeName transforms a string into a valid GraphQL type name by keeping letters, underscores, and digits (except as the first character), replacing invalid characters with underscores, and prefixing with T_ if the result would start with a digit.
func sanitizeTypeName(s string) string {
	if s == "" {
		return "Type"
	}
	var b strings.Builder
	for i, r := range s {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if isLetter || r == '_' || (i > 0 && isDigit) {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('_')
	}
	out := b.String()
	if out[0] >= '0' && out[0] <= '9' {
		return "T_" + out
	}
	return out
}
