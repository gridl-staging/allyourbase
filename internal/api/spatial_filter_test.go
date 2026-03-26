package api

import (
	"net/url"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func spatialFixtureTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "places",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", TypeName: "text"},
			{Name: "loc", TypeName: "geometry(Point,4326)", IsGeometry: true, IsGeography: false, SRID: 4326},
			{Name: "loc_geog", TypeName: "geography(Point,4326)", IsGeometry: true, IsGeography: true, SRID: 4326},
		},
	}
}

func TestParseSpatialParamsNoSpatialParams(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}

	q := url.Values{}
	sql, args, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.NoError(t, err)
	testutil.Equal(t, "", sql)
	testutil.SliceLen(t, args, 0)
}

func TestParseSpatialParamsMissingPostGIS(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: false}
	q := url.Values{"near": []string{"loc,0,0,10"}}

	_, _, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "spatial filters require PostGIS extension")
}

func TestParseSpatialParamsNearGeometry(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}

	q := url.Values{"near": []string{"loc,12.34,56.78,100"}}
	sql, args, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_DWithin("loc", ST_SetSRID(ST_MakePoint($1, $2), 4326), $3)`, sql)
	testutil.SliceLen(t, args, 3)
}

func TestParseSpatialParamsNearGeographyAndOffset(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}

	q := url.Values{"near": []string{"loc_geog,12.34,56.78,100"}}
	sql, args, err := parseSpatialParams(tbl, q, cache, 4)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_DWithin("loc_geog"::geography, ST_SetSRID(ST_MakePoint($4, $5), 4326)::geography, $6)`, sql)
	testutil.SliceLen(t, args, 3)
	testutil.Equal(t, 12.34, args[0].(float64))
}

func TestParseSpatialParamsWithinWithInvalidType(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{"within": []string{`loc,{"type":"Point","coordinates":[0,0]}`}}

	_, _, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "within only supports Polygon and MultiPolygon")
}

func TestParseSpatialParamsWithinGeometry(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{"within": []string{`loc,{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`}}

	sql, args, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Within("loc", ST_GeomFromGeoJSON($1))`, sql)
	testutil.SliceLen(t, args, 1)
}

func TestParseSpatialParamsIntersectsAllowsCollection(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{
		"intersects": []string{`loc,{"type":"GeometryCollection","geometries":[{"type":"Point","coordinates":[1,1]}]}`},
	}

	sql, args, err := parseSpatialParams(tbl, q, cache, 7)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Intersects("loc", ST_GeomFromGeoJSON($7))`, sql)
	testutil.SliceLen(t, args, 1)
}

func TestParseSpatialParamsBBoxValid(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{"bbox": []string{"loc,-80,-20,80,20"}}

	sql, args, err := parseSpatialParams(tbl, q, cache, 2)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Intersects("loc", ST_MakeEnvelope($2, $3, $4, $5, 4326))`, sql)
	testutil.SliceLen(t, args, 4)
}

func TestParseSpatialParamsBBoxValidation(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{"bbox": []string{"loc,10,5,1,6"}}

	_, _, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "minLng must be less than maxLng")
}

func TestParseSpatialParamsBBoxBadValue(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{"bbox": []string{"loc,west,-20,80,20"}}

	_, _, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "bbox minLng must be a number")
}

func TestParseSpatialParamsNearMalformed(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{"near": []string{"loc,12.34,abc,100"}}

	_, _, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "near latitude must be a number")
}

func TestParseSpatialParamsNearRejectsOutOfRangeCoordinates(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{"near": []string{"loc,181,0,100"}}

	_, _, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "valid WGS84 coordinates")
}

func TestParseSpatialParamsMissingSpatialColumn(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{"near": []string{"does_not_exist,0,0,10"}}

	_, _, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "column \"does_not_exist\" not found")
}

func TestParseSpatialParamsNonSpatialColumn(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{"near": []string{"name,0,0,10"}}

	_, _, err := parseSpatialParams(tbl, q, cache, 1)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "is not a spatial column")
}

func TestParseSpatialParamsBboxAndIntersectsOrderAndOffsets(t *testing.T) {
	t.Parallel()
	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	q := url.Values{
		"near":       []string{"loc,1,2,3"},
		"intersects": []string{`loc,{"type":"LineString","coordinates":[[1,2],[3,4]]}`},
	}

	sql, args, err := parseSpatialParams(tbl, q, cache, 5)
	testutil.NoError(t, err)
	testutil.Equal(t, 4, len(args))
	testutil.Contains(t, sql, `ST_DWithin("loc", ST_SetSRID(ST_MakePoint($5, $6), 4326), $7)`)
	testutil.Contains(t, sql, `ST_Intersects("loc", ST_GeomFromGeoJSON($8))`)
}
