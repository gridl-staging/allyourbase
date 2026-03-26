package schema

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildSpatialInfoSummaryNilCache(t *testing.T) {
	t.Parallel()

	summary := BuildSpatialInfoSummary(nil)
	testutil.False(t, summary.HasPostGIS)
	testutil.Equal(t, "", summary.PostGISVersion)
	testutil.SliceLen(t, summary.Tables, 0)
}

func TestBuildSpatialInfoSummaryIncludesSpatialMetadata(t *testing.T) {
	t.Parallel()

	cache := &SchemaCache{
		HasPostGIS:     true,
		PostGISVersion: "3.4",
		Tables: map[string]*Table{
			"public.audit_logs": {
				Schema: "public",
				Name:   "audit_logs",
				Columns: []*Column{
					{Name: "id", IsGeometry: false},
				},
			},
			"public.places": {
				Schema: "public",
				Name:   "places",
				Columns: []*Column{
					{Name: "id", IsGeometry: false},
					{Name: "location", IsGeometry: true, GeometryType: "Point", SRID: 4326},
					{Name: "service_area", IsGeometry: true, IsGeography: true, GeometryType: "Polygon", SRID: 3857},
				},
				Indexes: []*Index{
					{Name: "idx_places_location_gist", Method: "gist", Columns: []string{"location"}},
				},
			},
		},
	}

	summary := BuildSpatialInfoSummary(cache)
	testutil.True(t, summary.HasPostGIS)
	testutil.Equal(t, "3.4", summary.PostGISVersion)
	testutil.SliceLen(t, summary.Tables, 1)

	tbl := summary.Tables[0]
	testutil.Equal(t, "public", tbl.Schema)
	testutil.Equal(t, "places", tbl.Table)
	testutil.SliceLen(t, tbl.SpatialColumns, 2)
	testutil.Equal(t, "location", tbl.SpatialColumns[0].Column)
	testutil.Equal(t, "Point", tbl.SpatialColumns[0].GeometryType)
	testutil.Equal(t, 4326, tbl.SpatialColumns[0].SRID)
	testutil.False(t, tbl.SpatialColumns[0].IsGeography)
	testutil.Equal(t, "service_area", tbl.SpatialColumns[1].Column)
	testutil.True(t, tbl.SpatialColumns[1].IsGeography)
	testutil.Equal(t, "Polygon", tbl.SpatialColumns[1].GeometryType)
	testutil.Equal(t, 3857, tbl.SpatialColumns[1].SRID)
	testutil.SliceLen(t, tbl.ColumnsMissingSpatialIndex, 1)
	testutil.Equal(t, "service_area", tbl.ColumnsMissingSpatialIndex[0])
}

func TestSpatialInfoSummaryToMap(t *testing.T) {
	t.Parallel()

	summary := SpatialInfoSummary{
		HasPostGIS:     true,
		PostGISVersion: "3.4",
		Tables: []SpatialInfoTable{
			{
				Schema: "public",
				Table:  "places",
				SpatialColumns: []SpatialInfoColumn{
					{Column: "location", GeometryType: "Point", SRID: 4326},
				},
				ColumnsMissingSpatialIndex: []string{"location"},
			},
		},
	}

	payload := summary.ToMap()
	testutil.Equal(t, true, payload["hasPostGIS"])
	testutil.Equal(t, "3.4", payload["postGISVersion"])
	tables, ok := payload["tables"].([]map[string]any)
	testutil.True(t, ok)
	testutil.SliceLen(t, tables, 1)
	testutil.Equal(t, "public", tables[0]["schema"])
	testutil.Equal(t, "places", tables[0]["table"])
	columns, ok := tables[0]["spatialColumns"].([]map[string]any)
	testutil.True(t, ok)
	testutil.SliceLen(t, columns, 1)
	testutil.Equal(t, "location", columns[0]["column"])
	testutil.Equal(t, "Point", columns[0]["geometryType"])
	testutil.Equal(t, 4326, columns[0]["srid"])
}
