// Package graphql This file defines GraphQL input types for filtering and ordering database tables, including comparison operators for different column types.
package graphql

import (
	"strings"
	"sync"

	gql "github.com/graphql-go/graphql"

	"github.com/allyourbase/ayb/internal/schema"
)

var comparisonInputsInit sync.Once

func initComparisonInputs() {
	comparisonInputsInit.Do(func() {
		_ = intComparisonInput.Fields()
		_ = floatComparisonInput.Fields()
		_ = stringComparisonInput.Fields()
		_ = boolComparisonInput.Fields()
		_ = jsonComparisonInput.Fields()
	})
}

var OrderByEnum = gql.NewEnum(gql.EnumConfig{
	Name: "OrderByEnum",
	Values: gql.EnumValueConfigMap{
		"ASC":  &gql.EnumValueConfig{Value: "ASC"},
		"DESC": &gql.EnumValueConfig{Value: "DESC"},
	},
})

var (
	nearInput = gql.NewInputObject(gql.InputObjectConfig{
		Name: "NearInput",
		Fields: gql.InputObjectConfigFieldMap{
			"column":    &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.String)},
			"longitude": &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.Float)},
			"latitude":  &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.Float)},
			"distance":  &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.Float)},
		},
	})
	withinInput = gql.NewInputObject(gql.InputObjectConfig{
		Name: "WithinInput",
		Fields: gql.InputObjectConfigFieldMap{
			"column":  &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.String)},
			"geojson": &gql.InputObjectFieldConfig{Type: gql.NewNonNull(GeoJSONScalar)},
		},
	})
	bboxInput = gql.NewInputObject(gql.InputObjectConfig{
		Name: "BBoxInput",
		Fields: gql.InputObjectConfigFieldMap{
			"column": &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.String)},
			"minLng": &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.Float)},
			"minLat": &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.Float)},
			"maxLng": &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.Float)},
			"maxLat": &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.Float)},
		},
	})

	intComparisonInput = gql.NewInputObject(gql.InputObjectConfig{
		Name: "IntComparisonInput",
		Fields: gql.InputObjectConfigFieldMap{
			"_eq":      &gql.InputObjectFieldConfig{Type: gql.Int},
			"_neq":     &gql.InputObjectFieldConfig{Type: gql.Int},
			"_gt":      &gql.InputObjectFieldConfig{Type: gql.Int},
			"_gte":     &gql.InputObjectFieldConfig{Type: gql.Int},
			"_lt":      &gql.InputObjectFieldConfig{Type: gql.Int},
			"_lte":     &gql.InputObjectFieldConfig{Type: gql.Int},
			"_in":      &gql.InputObjectFieldConfig{Type: gql.NewList(gql.NewNonNull(gql.Int))},
			"_is_null": &gql.InputObjectFieldConfig{Type: gql.Boolean},
		},
	})
	floatComparisonInput = gql.NewInputObject(gql.InputObjectConfig{
		Name: "FloatComparisonInput",
		Fields: gql.InputObjectConfigFieldMap{
			"_eq":      &gql.InputObjectFieldConfig{Type: gql.Float},
			"_neq":     &gql.InputObjectFieldConfig{Type: gql.Float},
			"_gt":      &gql.InputObjectFieldConfig{Type: gql.Float},
			"_gte":     &gql.InputObjectFieldConfig{Type: gql.Float},
			"_lt":      &gql.InputObjectFieldConfig{Type: gql.Float},
			"_lte":     &gql.InputObjectFieldConfig{Type: gql.Float},
			"_in":      &gql.InputObjectFieldConfig{Type: gql.NewList(gql.NewNonNull(gql.Float))},
			"_is_null": &gql.InputObjectFieldConfig{Type: gql.Boolean},
		},
	})
	stringComparisonInput = gql.NewInputObject(gql.InputObjectConfig{
		Name: "StringComparisonInput",
		Fields: gql.InputObjectConfigFieldMap{
			"_eq":      &gql.InputObjectFieldConfig{Type: gql.String},
			"_neq":     &gql.InputObjectFieldConfig{Type: gql.String},
			"_gt":      &gql.InputObjectFieldConfig{Type: gql.String},
			"_gte":     &gql.InputObjectFieldConfig{Type: gql.String},
			"_lt":      &gql.InputObjectFieldConfig{Type: gql.String},
			"_lte":     &gql.InputObjectFieldConfig{Type: gql.String},
			"_in":      &gql.InputObjectFieldConfig{Type: gql.NewList(gql.NewNonNull(gql.String))},
			"_is_null": &gql.InputObjectFieldConfig{Type: gql.Boolean},
			"_like":    &gql.InputObjectFieldConfig{Type: gql.String},
			"_ilike":   &gql.InputObjectFieldConfig{Type: gql.String},
		},
	})
	boolComparisonInput = gql.NewInputObject(gql.InputObjectConfig{
		Name: "BooleanComparisonInput",
		Fields: gql.InputObjectConfigFieldMap{
			"_eq":      &gql.InputObjectFieldConfig{Type: gql.Boolean},
			"_neq":     &gql.InputObjectFieldConfig{Type: gql.Boolean},
			"_is_null": &gql.InputObjectFieldConfig{Type: gql.Boolean},
		},
	})
	jsonComparisonInput = gql.NewInputObject(gql.InputObjectConfig{
		Name: "JSONComparisonInput",
		Fields: gql.InputObjectConfigFieldMap{
			"_eq":      &gql.InputObjectFieldConfig{Type: JSONScalar},
			"_neq":     &gql.InputObjectFieldConfig{Type: JSONScalar},
			"_is_null": &gql.InputObjectFieldConfig{Type: gql.Boolean},
		},
	})
)

// buildWhereInput constructs a GraphQL InputObject type for filtering a table. It creates comparison fields for each column and includes _and, _or, and _not fields to support logical operations.
func buildWhereInput(tbl *schema.Table) *gql.InputObject {
	initComparisonInputs()

	typeName := toPascal(tbl.Name) + "WhereInput"
	var whereInput *gql.InputObject
	whereInput = gql.NewInputObject(gql.InputObjectConfig{
		Name: typeName,
		Fields: gql.InputObjectConfigFieldMapThunk(func() gql.InputObjectConfigFieldMap {
			fields := gql.InputObjectConfigFieldMap{}
			for _, col := range tbl.Columns {
				fields[col.Name] = &gql.InputObjectFieldConfig{Type: comparisonInputFor(col)}
			}
			fields["_and"] = &gql.InputObjectFieldConfig{Type: gql.NewList(whereInput)}
			fields["_or"] = &gql.InputObjectFieldConfig{Type: gql.NewList(whereInput)}
			fields["_not"] = &gql.InputObjectFieldConfig{Type: whereInput}
			return fields
		}),
	})
	return whereInput
}

func buildOrderByInput(tbl *schema.Table) *gql.InputObject {
	initComparisonInputs()

	fields := gql.InputObjectConfigFieldMap{}
	for _, col := range tbl.Columns {
		fields[col.Name] = &gql.InputObjectFieldConfig{Type: OrderByEnum}
	}
	return gql.NewInputObject(gql.InputObjectConfig{
		Name:   toPascal(tbl.Name) + "OrderByInput",
		Fields: fields,
	})
}

// comparisonInputFor returns the appropriate GraphQL comparison input type for a column based on its PostgreSQL type. It returns stringComparisonInput for nil columns, jsonComparisonInput for JSON columns, and handles enums, arrays, geometry, and vector types as strings.
func comparisonInputFor(col *schema.Column) *gql.InputObject {
	initComparisonInputs()

	if col == nil {
		return stringComparisonInput
	}

	if col.IsJSON {
		return jsonComparisonInput
	}
	if col.IsEnum || col.IsArray || col.IsGeometry || col.IsVector {
		return stringComparisonInput
	}

	mapped := pgTypeNameToGraphQL(col.TypeName)
	switch mapped {
	case gql.Int:
		return intComparisonInput
	case gql.Float:
		return floatComparisonInput
	case gql.Boolean:
		return boolComparisonInput
	case gql.DateTime:
		return stringComparisonInput
	default:
		return stringComparisonInput
	}
}

// toPascal converts a name to PascalCase, treating underscores, hyphens, and spaces as word boundaries. Returns "Type" if the input is empty or sanitization produces an empty result.
func toPascal(name string) string {
	if name == "" {
		return "Type"
	}
	var out strings.Builder
	upperNext := true
	for _, r := range name {
		if r == '_' || r == '-' || r == ' ' {
			upperNext = true
			continue
		}
		if upperNext && r >= 'a' && r <= 'z' {
			out.WriteRune(r - ('a' - 'A'))
		} else {
			out.WriteRune(r)
		}
		upperNext = false
	}
	result := sanitizeTypeName(out.String())
	if result == "" {
		return "Type"
	}
	return result
}
