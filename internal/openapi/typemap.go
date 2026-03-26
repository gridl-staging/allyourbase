// Package openapi Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/openapi/typemap.go.
package openapi

import (
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

// schemaProperty represents a JSON Schema property inside an OpenAPI document.
type schemaProperty struct {
	Ref         string                     `json:"$ref,omitempty"`
	Type        string                     `json:"type,omitempty"`
	Format      string                     `json:"format,omitempty"`
	Enum        []string                   `json:"enum,omitempty"`
	Items       *schemaProperty            `json:"items,omitempty"`
	Properties  map[string]*schemaProperty `json:"properties,omitempty"`
	Required    []string                   `json:"required,omitempty"`
	Description string                     `json:"description,omitempty"`
	ReadOnly    bool                       `json:"readOnly,omitempty"`
	OneOf       []*schemaProperty          `json:"oneOf,omitempty"`
}

// columnToProperty converts a schema.Column to a JSON Schema property.
func columnToProperty(col *schema.Column) *schemaProperty {
	return columnToPropertyWithGeoJSONRefs(col, true)
}

func columnToPropertyWithGeoJSONRefs(col *schema.Column, emitGeoJSONComponentRefs bool) *schemaProperty {
	base := pgTypeToSchemaWithGeoJSONRefs(col, emitGeoJSONComponentRefs)
	if col.IsPrimaryKey {
		base.ReadOnly = true
	}
	if col.IsNullable {
		return nullable(base)
	}
	return base
}

// nullable wraps a property in a oneOf [type, null] construct.
func nullable(p *schemaProperty) *schemaProperty {
	nullProp := &schemaProperty{Type: "null"}
	return &schemaProperty{OneOf: []*schemaProperty{p, nullProp}}
}

// pgTypeToSchema maps a PostgreSQL column type to a JSON Schema property.
// It does not apply nullable or readOnly — callers handle those separately.
func pgTypeToSchema(col *schema.Column) *schemaProperty {
	return pgTypeToSchemaWithGeoJSONRefs(col, true)
}

// TODO: Document pgTypeToSchemaWithGeoJSONRefs.
func pgTypeToSchemaWithGeoJSONRefs(col *schema.Column, emitGeoJSONComponentRefs bool) *schemaProperty {
	// Enum columns: resolve to string with enum values.
	if col.IsEnum && len(col.EnumValues) > 0 {
		return &schemaProperty{Type: "string", Enum: col.EnumValues}
	}

	// Vector column (pgvector).
	if col.IsVector {
		return &schemaProperty{
			Type:  "array",
			Items: &schemaProperty{Type: "number"},
		}
	}

	// Geometry / geography (PostGIS).
	if col.IsGeometry || col.IsGeography {
		return spatialColumnProperty(col, emitGeoJSONComponentRefs)
	}

	return pgTypeNameToSchema(col.TypeName)
}

func spatialColumnProperty(col *schema.Column, emitGeoJSONComponentRefs bool) *schemaProperty {
	componentName := geoJSONComponentForColumn(col)
	description := spatialColumnDescription(col, componentName)
	if !emitGeoJSONComponentRefs {
		return &schemaProperty{
			Type:        "object",
			Description: description,
		}
	}
	return &schemaProperty{
		Ref:         "#/components/schemas/" + componentName,
		Description: description,
	}
}

// TODO: Document geoJSONComponentForColumn.
func geoJSONComponentForColumn(col *schema.Column) string {
	if col == nil {
		return "GeoJSONGeometry"
	}

	switch strings.ToLower(strings.TrimSpace(col.GeometryType)) {
	case "point":
		return "GeoJSONPoint"
	case "linestring":
		return "GeoJSONLineString"
	case "polygon":
		return "GeoJSONPolygon"
	case "multipoint":
		return "GeoJSONMultiPoint"
	case "multilinestring":
		return "GeoJSONMultiLineString"
	case "multipolygon":
		return "GeoJSONMultiPolygon"
	case "geometrycollection":
		return "GeoJSONGeometryCollection"
	default:
		return "GeoJSONGeometry"
	}
}

// TODO: Document spatialColumnDescription.
func spatialColumnDescription(col *schema.Column, componentName string) string {
	label := strings.TrimPrefix(componentName, "GeoJSON")
	if label == "Geometry" || label == "" {
		label = "Geometry"
	}

	description := "GeoJSON " + label
	var qualifiers []string
	if col != nil && col.SRID != 0 {
		qualifiers = append(qualifiers, "SRID: "+strconv.Itoa(col.SRID))
	}
	if col != nil && col.IsGeography {
		qualifiers = append(qualifiers, "geography - distances in meters")
	}
	if len(qualifiers) == 0 {
		return description
	}
	return description + " (" + strings.Join(qualifiers, ", ") + ")"
}

// pgTypeNameToSchema maps a PostgreSQL type name string to a JSON Schema property.
func pgTypeNameToSchema(typeName string) *schemaProperty {
	t := strings.ToLower(strings.TrimSpace(typeName))

	// Strip array suffix [] to detect arrays.
	if strings.HasSuffix(t, "[]") {
		inner := pgTypeNameToSchema(strings.TrimSuffix(t, "[]"))
		return &schemaProperty{Type: "array", Items: inner}
	}

	switch {
	// Integer types.
	case t == "integer" || t == "int" || t == "int4" ||
		t == "bigint" || t == "int8" || t == "smallint" || t == "int2":
		return &schemaProperty{Type: "integer"}

	// Text / character types.
	case t == "text" || t == "varchar" || t == "character varying" ||
		t == "char" || t == "character" || t == "name" || t == "citext":
		return &schemaProperty{Type: "string"}

	// Boolean.
	case t == "boolean" || t == "bool":
		return &schemaProperty{Type: "boolean"}

	// Timestamp with timezone.
	case t == "timestamptz" || t == "timestamp with time zone" ||
		t == "timestamp":
		return &schemaProperty{Type: "string", Format: "date-time"}

	// Date.
	case t == "date":
		return &schemaProperty{Type: "string", Format: "date"}

	// Time.
	case t == "time" || t == "timetz" || t == "time with time zone" ||
		t == "time without time zone":
		return &schemaProperty{Type: "string", Format: "time"}

	// UUID.
	case t == "uuid":
		return &schemaProperty{Type: "string", Format: "uuid"}

	// JSON / JSONB.
	case t == "json" || t == "jsonb":
		return &schemaProperty{Type: "object"}

	// Numeric / decimal.
	case t == "numeric" || t == "decimal" || t == "money" ||
		strings.HasPrefix(t, "numeric(") || strings.HasPrefix(t, "decimal("):
		return &schemaProperty{Type: "string", Format: "decimal"}

	// Float.
	case t == "real" || t == "float4" || t == "float8" ||
		t == "double precision" || t == "float":
		return &schemaProperty{Type: "number"}

	// Byte array.
	case t == "bytea":
		return &schemaProperty{Type: "string", Format: "binary"}

	// Interval.
	case t == "interval":
		return &schemaProperty{Type: "string"}

	// Network address types.
	case t == "inet" || t == "cidr" || t == "macaddr":
		return &schemaProperty{Type: "string"}

	// Text search.
	case t == "tsvector" || t == "tsquery":
		return &schemaProperty{Type: "string"}

	// XML.
	case t == "xml":
		return &schemaProperty{Type: "string"}

	// Serial (treated as integer — it's a pseudo-type).
	case t == "serial" || t == "serial4" || t == "bigserial" || t == "serial8" || t == "smallserial":
		return &schemaProperty{Type: "integer"}

	default:
		// Fallback: treat unknown types as string.
		return &schemaProperty{Type: "string"}
	}
}

// funcParamTypeToSchema maps a PostgreSQL function parameter type to a JSON Schema property.
func funcParamTypeToSchema(pgType string) *schemaProperty {
	return pgTypeNameToSchema(pgType)
}

// returnTypeToSchema maps a function return type to a top-level response schema.
func returnTypeToSchema(returnType string, returnsSet bool) *schemaProperty {
	t := strings.ToLower(strings.TrimSpace(returnType))

	var item *schemaProperty
	switch {
	case t == "void":
		return &schemaProperty{Type: "object"}
	case strings.HasPrefix(t, "setof ") || strings.HasPrefix(t, "table("):
		item = &schemaProperty{Type: "object"}
	case t == "record":
		item = &schemaProperty{Type: "object"}
	default:
		item = pgTypeNameToSchema(returnType)
	}

	if returnsSet {
		return &schemaProperty{Type: "array", Items: item}
	}
	return item
}
