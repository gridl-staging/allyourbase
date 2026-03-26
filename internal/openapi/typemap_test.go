package openapi

import (
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
)

func TestColumnToProperty_BasicTypes(t *testing.T) {
	tests := []struct {
		name     string
		col      *schema.Column
		wantType string
		wantFmt  string
	}{
		{"integer", &schema.Column{TypeName: "integer"}, "integer", ""},
		{"bigint", &schema.Column{TypeName: "bigint"}, "integer", ""},
		{"smallint", &schema.Column{TypeName: "smallint"}, "integer", ""},
		{"text", &schema.Column{TypeName: "text"}, "string", ""},
		{"varchar", &schema.Column{TypeName: "varchar"}, "string", ""},
		{"character varying", &schema.Column{TypeName: "character varying"}, "string", ""},
		{"boolean", &schema.Column{TypeName: "boolean"}, "boolean", ""},
		{"timestamptz", &schema.Column{TypeName: "timestamptz"}, "string", "date-time"},
		{"timestamp", &schema.Column{TypeName: "timestamp"}, "string", "date-time"},
		{"date", &schema.Column{TypeName: "date"}, "string", "date"},
		{"time", &schema.Column{TypeName: "time"}, "string", "time"},
		{"uuid", &schema.Column{TypeName: "uuid"}, "string", "uuid"},
		{"json", &schema.Column{TypeName: "json"}, "object", ""},
		{"jsonb", &schema.Column{TypeName: "jsonb"}, "object", ""},
		{"numeric", &schema.Column{TypeName: "numeric"}, "string", "decimal"},
		{"decimal", &schema.Column{TypeName: "decimal"}, "string", "decimal"},
		{"numeric(10,2)", &schema.Column{TypeName: "numeric(10,2)"}, "string", "decimal"},
		{"real", &schema.Column{TypeName: "real"}, "number", ""},
		{"double precision", &schema.Column{TypeName: "double precision"}, "number", ""},
		{"bytea", &schema.Column{TypeName: "bytea"}, "string", "binary"},
		{"serial", &schema.Column{TypeName: "serial"}, "integer", ""},
		{"bigserial", &schema.Column{TypeName: "bigserial"}, "integer", ""},
		{"inet", &schema.Column{TypeName: "inet"}, "string", ""},
		{"interval", &schema.Column{TypeName: "interval"}, "string", ""},
		{"xml", &schema.Column{TypeName: "xml"}, "string", ""},
		{"tsvector", &schema.Column{TypeName: "tsvector"}, "string", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := columnToProperty(tt.col)
			if p.Type != tt.wantType {
				t.Errorf("type = %q, want %q", p.Type, tt.wantType)
			}
			if p.Format != tt.wantFmt {
				t.Errorf("format = %q, want %q", p.Format, tt.wantFmt)
			}
		})
	}
}

func TestColumnToProperty_Nullable(t *testing.T) {
	col := &schema.Column{TypeName: "integer", IsNullable: true}
	p := columnToProperty(col)
	if p.OneOf == nil || len(p.OneOf) != 2 {
		t.Fatalf("expected oneOf with 2 entries, got %v", p.OneOf)
	}
	if p.OneOf[0].Type != "integer" {
		t.Errorf("oneOf[0].type = %q, want integer", p.OneOf[0].Type)
	}
	if p.OneOf[1].Type != "null" {
		t.Errorf("oneOf[1].type = %q, want null", p.OneOf[1].Type)
	}
}

func TestColumnToProperty_PrimaryKey(t *testing.T) {
	col := &schema.Column{TypeName: "integer", IsPrimaryKey: true}
	p := columnToProperty(col)
	if !p.ReadOnly {
		t.Error("expected PK column to be readOnly")
	}
}

func TestColumnToProperty_Enum(t *testing.T) {
	col := &schema.Column{TypeName: "mood", IsEnum: true, EnumValues: []string{"happy", "sad", "neutral"}}
	p := columnToProperty(col)
	if p.Type != "string" {
		t.Errorf("type = %q, want string", p.Type)
	}
	if len(p.Enum) != 3 || p.Enum[0] != "happy" {
		t.Errorf("enum = %v, want [happy sad neutral]", p.Enum)
	}
}

func TestColumnToProperty_Vector(t *testing.T) {
	col := &schema.Column{TypeName: "vector", IsVector: true, VectorDim: 384}
	p := columnToProperty(col)
	if p.Type != "array" {
		t.Errorf("type = %q, want array", p.Type)
	}
	if p.Items == nil || p.Items.Type != "number" {
		t.Error("expected items.type = number for vector column")
	}
}

func TestColumnToProperty_Geometry(t *testing.T) {
	col := &schema.Column{TypeName: "geometry", IsGeometry: true, GeometryType: "Point", SRID: 4326}
	p := columnToProperty(col)
	if p.Ref != "#/components/schemas/GeoJSONPoint" {
		t.Errorf("ref = %q, want #/components/schemas/GeoJSONPoint", p.Ref)
	}
	if p.Description == "" {
		t.Fatal("expected spatial description")
	}
	if !containsAll(p.Description, []string{"GeoJSON Point", "SRID: 4326"}) {
		t.Errorf("description = %q, want GeoJSON Point + SRID", p.Description)
	}
}

func TestColumnToPropertyWithGeoJSONRefsDisabledFallsBackToObject(t *testing.T) {
	col := &schema.Column{TypeName: "geometry", IsGeometry: true, GeometryType: "Point", SRID: 4326}
	p := columnToPropertyWithGeoJSONRefs(col, false)
	if p.Ref != "" {
		t.Errorf("ref = %q, want empty ref when GeoJSON components are disabled", p.Ref)
	}
	if p.Type != "object" {
		t.Errorf("type = %q, want object when GeoJSON components are disabled", p.Type)
	}
	if !containsAll(p.Description, []string{"GeoJSON Point", "SRID: 4326"}) {
		t.Errorf("description = %q, want GeoJSON Point + SRID", p.Description)
	}
}

func TestColumnToProperty_Array(t *testing.T) {
	col := &schema.Column{TypeName: "integer[]"}
	p := columnToProperty(col)
	if p.Type != "array" {
		t.Errorf("type = %q, want array", p.Type)
	}
	if p.Items == nil || p.Items.Type != "integer" {
		t.Error("expected items.type = integer for integer[]")
	}
}

func TestFuncParamTypeToSchema(t *testing.T) {
	p := funcParamTypeToSchema("text")
	if p.Type != "string" {
		t.Errorf("type = %q, want string", p.Type)
	}
}

func TestReturnTypeToSchema(t *testing.T) {
	tests := []struct {
		name       string
		returnType string
		returnsSet bool
		wantType   string
	}{
		{"void", "void", false, "object"},
		{"integer", "integer", false, "integer"},
		{"setof record", "SETOF record", true, "array"},
		{"record set", "record", true, "array"},
		{"integer set", "integer", true, "array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := returnTypeToSchema(tt.returnType, tt.returnsSet)
			if p.Type != tt.wantType {
				t.Errorf("type = %q, want %q", p.Type, tt.wantType)
			}
		})
	}
}

func TestPgTypeNameToSchema_UnknownFallback(t *testing.T) {
	p := pgTypeNameToSchema("custom_unknown_type")
	if p.Type != "string" {
		t.Errorf("type = %q, want string for unknown type", p.Type)
	}
}
