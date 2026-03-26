package openapi

import (
	"encoding/json"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
)

func TestGenerate_PostGISSpatialComponentsAndParams(t *testing.T) {
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "places",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer", IsPrimaryKey: true},
				{Name: "name", TypeName: "text"},
				{Name: "location", TypeName: "geometry", IsGeometry: true, GeometryType: "Point", SRID: 4326},
			},
			PrimaryKey: []string{"id"},
		},
	}, nil)
	cache.HasPostGIS = true

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	if _, ok := schemas["GeoJSONPoint"]; !ok {
		t.Fatal("expected GeoJSONPoint component schema")
	}

	placesSchema := schemas["Places"].(map[string]any)
	locationProp := placesSchema["properties"].(map[string]any)["location"].(map[string]any)
	if ref := locationProp["$ref"]; ref != "#/components/schemas/GeoJSONPoint" {
		t.Fatalf("location $ref = %v, want #/components/schemas/GeoJSONPoint", ref)
	}
	description, _ := locationProp["description"].(string)
	if description == "" || !containsAll(description, []string{"GeoJSON Point", "SRID: 4326"}) {
		t.Fatalf("expected SRID-aware location description, got %q", description)
	}

	params := doc["paths"].(map[string]any)["/places"].(map[string]any)["get"].(map[string]any)["parameters"].([]any)
	paramNames := make(map[string]bool, len(params))
	for _, p := range params {
		name := p.(map[string]any)["name"].(string)
		paramNames[name] = true
	}
	for _, name := range []string{"near", "within", "intersects", "bbox"} {
		if !paramNames[name] {
			t.Fatalf("expected spatial query parameter %q", name)
		}
	}
}

func TestGenerate_PostGISGeographyDescriptionIncludesMeters(t *testing.T) {
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "places_geog",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer", IsPrimaryKey: true},
				{Name: "location", TypeName: "geography", IsGeometry: true, IsGeography: true, GeometryType: "Point", SRID: 4326},
			},
			PrimaryKey: []string{"id"},
		},
	}, nil)
	cache.HasPostGIS = true

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	locationProp := schemas["Places_geog"].(map[string]any)["properties"].(map[string]any)["location"].(map[string]any)
	if ref := locationProp["$ref"]; ref != "#/components/schemas/GeoJSONPoint" {
		t.Fatalf("location $ref = %v, want #/components/schemas/GeoJSONPoint", ref)
	}
	description, _ := locationProp["description"].(string)
	if !containsAll(description, []string{"geography", "meters"}) {
		t.Fatalf("expected geography description, got %q", description)
	}
}

func TestGenerate_PostGISDisabledSpatialTableFallsBackWithoutGeoJSONRefs(t *testing.T) {
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "places",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer", IsPrimaryKey: true},
				{Name: "location", TypeName: "geometry", IsGeometry: true, GeometryType: "Point", SRID: 4326},
			},
			PrimaryKey: []string{"id"},
		},
	}, nil)
	cache.HasPostGIS = false

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	if _, ok := schemas["GeoJSONPoint"]; ok {
		t.Fatal("GeoJSONPoint should not be emitted without PostGIS support")
	}

	locationProp := schemas["Places"].(map[string]any)["properties"].(map[string]any)["location"].(map[string]any)
	if ref, hasRef := locationProp["$ref"]; hasRef {
		t.Fatalf("location should not use a dangling GeoJSON ref, got %v", ref)
	}
	if locationProp["type"] != "object" {
		t.Fatalf("location type = %v, want object fallback without GeoJSON refs", locationProp["type"])
	}

	params := doc["paths"].(map[string]any)["/places"].(map[string]any)["get"].(map[string]any)["parameters"].([]any)
	for _, p := range params {
		name := p.(map[string]any)["name"].(string)
		if name == "near" || name == "within" || name == "intersects" || name == "bbox" {
			t.Fatalf("unexpected spatial query param %q when PostGIS is disabled", name)
		}
	}
}

func TestGenerate_UnconstrainedGeometryUsesGeoJSONUnionRef(t *testing.T) {
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "shapes",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer", IsPrimaryKey: true},
				{Name: "geom", TypeName: "geometry", IsGeometry: true, GeometryType: ""},
			},
			PrimaryKey: []string{"id"},
		},
	}, nil)
	cache.HasPostGIS = true

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	geomProp := schemas["Shapes"].(map[string]any)["properties"].(map[string]any)["geom"].(map[string]any)
	if ref := geomProp["$ref"]; ref != "#/components/schemas/GeoJSONGeometry" {
		t.Fatalf("geom $ref = %v, want #/components/schemas/GeoJSONGeometry", ref)
	}
}

func TestGenerate_GeometryCollectionUsesSpecificRef(t *testing.T) {
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "collections",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer", IsPrimaryKey: true},
				{Name: "geom", TypeName: "geometry", IsGeometry: true, GeometryType: "GeometryCollection"},
			},
			PrimaryKey: []string{"id"},
		},
	}, nil)
	cache.HasPostGIS = true

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	geomProp := schemas["Collections"].(map[string]any)["properties"].(map[string]any)["geom"].(map[string]any)
	if ref := geomProp["$ref"]; ref != "#/components/schemas/GeoJSONGeometryCollection" {
		t.Fatalf("geom $ref = %v, want #/components/schemas/GeoJSONGeometryCollection", ref)
	}
}
