package graphql

import (
	"encoding/json"
	"testing"

	gql "github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func resetEnumTypeCacheForTest() {
	buildSchemaMu.Lock()
	defer buildSchemaMu.Unlock()
	resetEnumTypeCacheUnsafe()
}

func TestPgToGraphQLScalarMappings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		col  *schema.Column
		want gql.Output
	}{
		{name: "integer", col: &schema.Column{Name: "id", TypeName: "integer"}, want: gql.Int},
		{name: "text", col: &schema.Column{Name: "title", TypeName: "text"}, want: gql.String},
		{name: "boolean", col: &schema.Column{Name: "enabled", TypeName: "boolean"}, want: gql.Boolean},
		{name: "float", col: &schema.Column{Name: "score", TypeName: "numeric"}, want: gql.Float},
		{name: "timestamptz", col: &schema.Column{Name: "created_at", TypeName: "timestamptz"}, want: gql.DateTime},
		{name: "uuid", col: &schema.Column{Name: "user_id", TypeName: "uuid"}, want: gql.String},
		{name: "unknown", col: &schema.Column{Name: "custom", TypeName: "my_custom_type"}, want: gql.String},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pgToGraphQL(tt.col)
			testutil.True(t, got == tt.want, "pgToGraphQL(%s) mismatch", tt.col.TypeName)
		})
	}
}

func TestPgToGraphQLJSONScalar(t *testing.T) {
	t.Parallel()
	jsonCol := &schema.Column{Name: "metadata", TypeName: "json", IsJSON: true}
	jsonbCol := &schema.Column{Name: "metadata", TypeName: "jsonb", IsJSON: true}

	testutil.True(t, pgToGraphQL(jsonCol) == JSONScalar, "json should map to JSONScalar")
	testutil.True(t, pgToGraphQL(jsonbCol) == JSONScalar, "jsonb should map to JSONScalar")
}

func TestPgToGraphQLSpatialUsesGeoJSONScalar(t *testing.T) {
	t.Parallel()
	geometryCol := &schema.Column{Name: "location", TypeName: "geometry", IsGeometry: true}
	geographyCol := &schema.Column{Name: "location", TypeName: "geography", IsGeometry: true, IsGeography: true}

	testutil.True(t, pgToGraphQL(geometryCol) == GeoJSONScalar, "geometry should map to GeoJSONScalar")
	testutil.True(t, pgToGraphQL(geographyCol) == GeoJSONScalar, "geography should map to GeoJSONScalar")
}

func TestPgToGraphQLEnum(t *testing.T) {
	t.Parallel()
	resetEnumTypeCacheForTest()

	col := &schema.Column{
		Name:       "status",
		TypeName:   "status_enum",
		IsEnum:     true,
		EnumValues: []string{"active", "inactive", "suspended"},
	}
	got := pgToGraphQL(col)
	enumType, ok := got.(*gql.Enum)
	testutil.True(t, ok, "expected enum type, got %T", got)

	values := enumType.Values()
	testutil.Equal(t, 3, len(values))
	found := map[string]bool{}
	for _, v := range values {
		found[v.Name] = true
	}
	testutil.True(t, found["active"], "missing enum value active")
	testutil.True(t, found["inactive"], "missing enum value inactive")
	testutil.True(t, found["suspended"], "missing enum value suspended")
}

func TestPgToGraphQLArray(t *testing.T) {
	t.Parallel()
	col := &schema.Column{Name: "tags", TypeName: "text[]", IsArray: true}
	got := pgToGraphQL(col)
	listType, ok := got.(*gql.List)
	testutil.True(t, ok, "expected list type for array, got %T", got)
	testutil.True(t, listType.OfType == gql.String, "expected list inner type string")
}

func TestPgToGraphQLIgnoresNullableWrapping(t *testing.T) {
	t.Parallel()
	col := &schema.Column{Name: "title", TypeName: "text", IsNullable: false}
	got := pgToGraphQL(col)
	_, isNonNull := got.(*gql.NonNull)
	testutil.False(t, isNonNull, "pgToGraphQL should not apply NonNull wrapping")
}

func TestGeoJSONScalarSerializeMapPoint(t *testing.T) {
	t.Parallel()
	point := map[string]any{
		"type":        "Point",
		"coordinates": []any{10.0, 20.0},
	}

	serialized := GeoJSONScalar.Serialize(point)
	got, ok := serialized.(map[string]any)
	testutil.True(t, ok, "serialize should return map input unchanged")
	testutil.Equal(t, "Point", got["type"])
}

func TestGeoJSONScalarParseValueValidPolygon(t *testing.T) {
	t.Parallel()
	polygon := map[string]any{
		"type": "Polygon",
		"coordinates": []any{
			[]any{
				[]any{-1.0, -1.0},
				[]any{1.0, -1.0},
				[]any{1.0, 1.0},
				[]any{-1.0, -1.0},
			},
		},
	}

	parsed := GeoJSONScalar.ParseValue(polygon)
	got, ok := parsed.(map[string]any)
	testutil.True(t, ok, "parse value should return a map for valid GeoJSON polygon")
	testutil.Equal(t, "Polygon", got["type"])
}

func TestGeoJSONScalarRejectsFeatureWrapper(t *testing.T) {
	t.Parallel()
	feature := map[string]any{
		"type": "Feature",
		"geometry": map[string]any{
			"type":        "Point",
			"coordinates": []any{0.0, 0.0},
		},
	}

	parsed := GeoJSONScalar.ParseValue(feature)
	testutil.Nil(t, parsed)
}

func TestGeoJSONScalarRejectsNonObjectInput(t *testing.T) {
	t.Parallel()
	parsed := GeoJSONScalar.ParseValue("not-a-geojson-object")
	testutil.Nil(t, parsed)
}

func TestGeoJSONScalarParseValueRejectsJSONStringInput(t *testing.T) {
	t.Parallel()
	parsed := GeoJSONScalar.ParseValue(`{"type":"Point","coordinates":[0,0]}`)
	testutil.Nil(t, parsed)
}

func TestGeoJSONScalarParseLiteralPreservesNumericCoordinates(t *testing.T) {
	t.Parallel()

	parsed := GeoJSONScalar.ParseLiteral(&ast.ObjectValue{
		Fields: []*ast.ObjectField{
			{
				Name:  &ast.Name{Value: "type"},
				Value: &ast.StringValue{Value: "Point"},
			},
			{
				Name: &ast.Name{Value: "coordinates"},
				Value: &ast.ListValue{
					Values: []ast.Value{
						&ast.FloatValue{Value: "-73.98"},
						&ast.IntValue{Value: "40"},
					},
				},
			},
		},
	})

	geometry, ok := parsed.(map[string]any)
	testutil.True(t, ok, "parse literal should return a GeoJSON object")

	coordinates, ok := geometry["coordinates"].([]any)
	testutil.True(t, ok, "coordinates should be a slice")
	testutil.Equal(t, 2, len(coordinates))
	testutil.Equal(t, -73.98, coordinates[0])
	latitude, ok := coordinates[1].(int64)
	testutil.True(t, ok, "integer coordinates should stay numeric")
	testutil.Equal(t, int64(40), latitude)
}

func TestGeoJSONScalarRoundTripFidelity(t *testing.T) {
	t.Parallel()
	input := map[string]any{
		"type":        "Point",
		"coordinates": []any{-73.98, 40.76},
	}

	encoded, err := json.Marshal(input)
	testutil.NoError(t, err)
	serialized := GeoJSONScalar.Serialize(string(encoded))
	parsed := GeoJSONScalar.ParseValue(serialized)

	got, ok := parsed.(map[string]any)
	testutil.True(t, ok, "round-trip should produce map")
	testutil.Equal(t, "Point", got["type"])

	coords, ok := got["coordinates"].([]any)
	testutil.True(t, ok, "coordinates should be array")
	testutil.Equal(t, 2, len(coords))
	testutil.Equal(t, -73.98, coords[0])
	testutil.Equal(t, 40.76, coords[1])
}
