// Package openapi Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/openapi/generator_geojson.go.
package openapi

import "github.com/allyourbase/ayb/internal/schema"

// TODO: Document shouldEmitGeoJSONComponents.
func shouldEmitGeoJSONComponents(cache *schema.SchemaCache, tables []*schema.Table) bool {
	if cache == nil || !cache.HasPostGIS {
		return false
	}
	for _, tbl := range tables {
		if tbl == nil || isSystemTable(tbl.Name) {
			continue
		}
		if tbl.Kind != "table" && tbl.Kind != "view" && tbl.Kind != "materialized_view" {
			continue
		}
		if tbl.HasGeometry() {
			return true
		}
	}
	return false
}

// TODO: Document addGeoJSONComponentSchemas.
func addGeoJSONComponentSchemas(schemas map[string]*schemaProperty) {
	if schemas == nil {
		return
	}
	if _, exists := schemas["GeoJSONGeometry"]; exists {
		return
	}

	numberArray := &schemaProperty{Type: "array", Items: &schemaProperty{Type: "number"}}
	lineCoords := &schemaProperty{Type: "array", Items: numberArray}
	polygonCoords := &schemaProperty{Type: "array", Items: lineCoords}
	multiLineCoords := &schemaProperty{Type: "array", Items: lineCoords}
	multiPolygonCoords := &schemaProperty{Type: "array", Items: polygonCoords}

	schemas["GeoJSONPoint"] = geoJSONGeometryObjectSchema("Point", numberArray, nil)
	schemas["GeoJSONLineString"] = geoJSONGeometryObjectSchema("LineString", lineCoords, nil)
	schemas["GeoJSONPolygon"] = geoJSONGeometryObjectSchema("Polygon", polygonCoords, nil)
	schemas["GeoJSONMultiPoint"] = geoJSONGeometryObjectSchema("MultiPoint", lineCoords, nil)
	schemas["GeoJSONMultiLineString"] = geoJSONGeometryObjectSchema("MultiLineString", multiLineCoords, nil)
	schemas["GeoJSONMultiPolygon"] = geoJSONGeometryObjectSchema("MultiPolygon", multiPolygonCoords, nil)
	schemas["GeoJSONGeometryCollection"] = geoJSONGeometryObjectSchema("GeometryCollection", nil, &schemaProperty{
		Type:  "array",
		Items: &schemaProperty{Ref: "#/components/schemas/GeoJSONGeometry"},
	})
	schemas["GeoJSONGeometry"] = &schemaProperty{
		OneOf: []*schemaProperty{
			{Ref: "#/components/schemas/GeoJSONPoint"},
			{Ref: "#/components/schemas/GeoJSONLineString"},
			{Ref: "#/components/schemas/GeoJSONPolygon"},
			{Ref: "#/components/schemas/GeoJSONMultiPoint"},
			{Ref: "#/components/schemas/GeoJSONMultiLineString"},
			{Ref: "#/components/schemas/GeoJSONMultiPolygon"},
			{Ref: "#/components/schemas/GeoJSONGeometryCollection"},
		},
	}
}

// TODO: Document geoJSONGeometryObjectSchema.
func geoJSONGeometryObjectSchema(typeValue string, coordinates *schemaProperty, geometries *schemaProperty) *schemaProperty {
	properties := map[string]*schemaProperty{
		"type": {Type: "string", Enum: []string{typeValue}},
	}
	required := []string{"type"}
	if coordinates != nil {
		properties["coordinates"] = coordinates
		required = append(required, "coordinates")
	}
	if geometries != nil {
		properties["geometries"] = geometries
		required = append(required, "geometries")
	}
	return &schemaProperty{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}
