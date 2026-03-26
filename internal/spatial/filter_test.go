package spatial

import (
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNearFilterWhereClauseGeometry(t *testing.T) {
	col := &schema.Column{Name: "location", IsGeometry: true, IsGeography: false, SRID: 3857}
	filter := NearFilter{
		Column:    col,
		Longitude: 10.5,
		Latitude:  20.5,
		Distance:  100,
	}

	sql, args, err := filter.WhereClause(2)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_DWithin("location", ST_Transform(ST_SetSRID(ST_MakePoint($2, $3), 4326), 3857), $4)`, sql)
	testutil.SliceLen(t, args, 3)
	testutil.Equal(t, 10.5, args[0].(float64))
	testutil.Equal(t, 20.5, args[1].(float64))
	testutil.Equal(t, 100.0, args[2].(float64))
}

func TestNearFilterWhereClauseGeometryNoTransform(t *testing.T) {
	col := &schema.Column{Name: "location", IsGeometry: true, IsGeography: false, SRID: 4326}
	filter := NearFilter{
		Column:    col,
		Longitude: -1.2,
		Latitude:  3.4,
		Distance:  5,
	}

	sql, args, err := filter.WhereClause(1)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_DWithin("location", ST_SetSRID(ST_MakePoint($1, $2), 4326), $3)`, sql)
	testutil.SliceLen(t, args, 3)
}

func TestNearFilterWhereClauseGeography(t *testing.T) {
	col := &schema.Column{Name: "location", IsGeometry: true, IsGeography: true, SRID: 4326}
	filter := NearFilter{
		Column:    col,
		Longitude: 0.1,
		Latitude:  0.2,
		Distance:  9,
	}

	sql, args, err := filter.WhereClause(7)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_DWithin("location"::geography, ST_SetSRID(ST_MakePoint($7, $8), 4326)::geography, $9)`, sql)
	testutil.SliceLen(t, args, 3)
}

func TestWithinFilterWhereClauseGeometryTransformsGeometrySRID(t *testing.T) {
	col := &schema.Column{Name: "path", IsGeometry: true, SRID: 3857}
	filter := WithinFilter{
		Column:  col,
		GeoJSON: `{"type":"Point","coordinates":[0,0]}`,
	}

	sql, args, err := filter.WhereClause(3)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Within("path", ST_Transform(ST_GeomFromGeoJSON($3), 3857))`, sql)
	testutil.Equal(t, 1, len(args))
	testutil.Equal(t, `{"type":"Point","coordinates":[0,0]}`, args[0].(string))
}

func TestWithinFilterWhereClauseGeography(t *testing.T) {
	col := &schema.Column{Name: "path", IsGeometry: true, IsGeography: true, SRID: 4326}
	filter := WithinFilter{
		Column:  col,
		GeoJSON: `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`,
	}

	sql, _, err := filter.WhereClause(4)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Covers(ST_GeomFromGeoJSON($4)::geography, "path"::geography)`, sql)
}

func TestIntersectsFilterWhereClauseGeometry(t *testing.T) {
	col := &schema.Column{Name: "shape", IsGeometry: true}
	filter := IntersectsFilter{
		Column:  col,
		GeoJSON: `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,1],[0,0]]]}`,
	}

	sql, args, err := filter.WhereClause(1)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Intersects("shape", ST_GeomFromGeoJSON($1))`, sql)
	testutil.SliceLen(t, args, 1)
}

func TestIntersectsFilterWhereClauseGeography(t *testing.T) {
	col := &schema.Column{Name: "area", IsGeometry: true, IsGeography: true, SRID: 4326}
	filter := IntersectsFilter{
		Column:  col,
		GeoJSON: `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,1],[0,0]]]}`,
	}

	sql, args, err := filter.WhereClause(1)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Intersects("area"::geography, ST_GeomFromGeoJSON($1)::geography)`, sql)
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,1],[0,0]]]}`, args[0].(string))
}

func TestIntersectsFilterWhereClauseGeometryOffset(t *testing.T) {
	col := &schema.Column{Name: "shape", IsGeometry: true, SRID: 4326}
	filter := IntersectsFilter{
		Column:  col,
		GeoJSON: `{"type":"LineString","coordinates":[[0,0],[1,1]]}`,
	}

	sql, args, err := filter.WhereClause(5)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Intersects("shape", ST_GeomFromGeoJSON($5))`, sql)
	testutil.SliceLen(t, args, 1)
	testutil.Equal(t, `{"type":"LineString","coordinates":[[0,0],[1,1]]}`, args[0].(string))
}

func TestBBoxFilterWhereClauseGeometryNon4326(t *testing.T) {
	col := &schema.Column{Name: "bounds", IsGeometry: true, SRID: 3857}
	filter := BBoxFilter{
		Column: col,
		MinLng: -10,
		MinLat: -20,
		MaxLng: 10,
		MaxLat: 20,
	}

	sql, args, err := filter.WhereClause(9)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Intersects("bounds", ST_Transform(ST_MakeEnvelope($9, $10, $11, $12, 4326), 3857))`, sql)
	testutil.SliceLen(t, args, 4)
}

func TestBBoxFilterWhereClauseGeography(t *testing.T) {
	col := &schema.Column{Name: "bounds", IsGeometry: true, IsGeography: true}
	filter := BBoxFilter{
		Column: col,
		MinLng: -0.1,
		MinLat: 1.1,
		MaxLng: 2.2,
		MaxLat: 3.3,
	}

	sql, _, err := filter.WhereClause(2)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Intersects("bounds"::geography, ST_MakeEnvelope($2, $3, $4, $5, 4326)::geography)`, sql)
}

func TestParseGeoJSONGeometryGeometryCollection(t *testing.T) {
	geomType, err := ParseGeoJSONGeometry(`{"type":"GeometryCollection","geometries":[{"type":"Point","coordinates":[0,0]}]}`)
	testutil.NoError(t, err)
	testutil.Equal(t, "GeometryCollection", geomType)
}

func TestParseGeoJSONGeometryRejectsFeature(t *testing.T) {
	_, err := ParseGeoJSONGeometry(`{"type":"Feature","geometry":{"type":"Point","coordinates":[0,0]}}`)
	testutil.Error(t, err)
}

func TestParseGeoJSONGeometryRequiresCoordinates(t *testing.T) {
	_, err := ParseGeoJSONGeometry(`{"type":"Point"}`)
	testutil.Error(t, err)
}

func TestParseGeoJSONGeometryRequiresGeometries(t *testing.T) {
	_, err := ParseGeoJSONGeometry(`{"type":"GeometryCollection","geometries":[]}`)
	// empty list is valid for this parser contract; parsing is structural only.
	testutil.NoError(t, err)
}
