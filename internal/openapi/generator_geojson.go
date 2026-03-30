package openapi

import "github.com/allyourbase/ayb/internal/schema"

// shouldEmitGeoJSONComponents returns true if any user table has a PostGIS
// geometry/geography column, indicating the spec needs GeoJSON component schemas.
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

// addGeoJSONComponentSchemas registers the full set of GeoJSON geometry type
// schemas (Point, LineString, Polygon, Multi*, GeometryCollection, and the
// union type GeoJSONGeometry) into the OpenAPI components map. No-ops if
// already registered or if schemas is nil.
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

// geoJSONGeometryObjectSchema builds a single GeoJSON geometry object schema
// with the given type discriminator and either coordinates or geometries array.
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
