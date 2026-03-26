package spatial

import (
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestDistanceSortExprGeography(t *testing.T) {
	t.Parallel()

	sort := DistanceSort{
		Column:    &schema.Column{Name: "loc_geog", IsGeometry: true, IsGeography: true, SRID: 4326},
		Longitude: -73.99,
		Latitude:  40.73,
	}

	sql, args, err := sort.Expr(3)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Distance("loc_geog"::geography, ST_SetSRID(ST_MakePoint($3, $4), 4326)::geography)`, sql)
	testutil.SliceLen(t, args, 2)
	testutil.Equal(t, -73.99, args[0])
	testutil.Equal(t, 40.73, args[1])
}

func TestDistanceSortExprGeometry4326(t *testing.T) {
	t.Parallel()

	sort := DistanceSort{
		Column:    &schema.Column{Name: "location", IsGeometry: true, SRID: 4326},
		Longitude: 1.5,
		Latitude:  2.5,
	}

	sql, args, err := sort.Expr(1)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Distance("location", ST_SetSRID(ST_MakePoint($1, $2), 4326))`, sql)
	testutil.SliceLen(t, args, 2)
	testutil.Equal(t, 1.5, args[0])
	testutil.Equal(t, 2.5, args[1])
}

func TestDistanceSortExprGeometryNon4326(t *testing.T) {
	t.Parallel()

	sort := DistanceSort{
		Column:    &schema.Column{Name: "location", IsGeometry: true, SRID: 3857},
		Longitude: 10,
		Latitude:  20,
	}

	sql, args, err := sort.Expr(8)
	testutil.NoError(t, err)
	testutil.Equal(t, `ST_Distance("location", ST_Transform(ST_SetSRID(ST_MakePoint($8, $9), 4326), 3857))`, sql)
	testutil.SliceLen(t, args, 2)
	testutil.Equal(t, 10.0, args[0])
	testutil.Equal(t, 20.0, args[1])
}

func TestDistanceSortExprQuotesIdentifier(t *testing.T) {
	t.Parallel()

	sort := DistanceSort{
		Column:    &schema.Column{Name: `col"name`, IsGeometry: true, SRID: 4326},
		Longitude: 0,
		Latitude:  0,
	}

	sql, _, err := sort.Expr(1)
	testutil.NoError(t, err)
	testutil.Contains(t, sql, `"col""name"`)
}

func TestDistanceSortExprRejectsNonSpatialColumn(t *testing.T) {
	t.Parallel()

	sort := DistanceSort{Column: &schema.Column{Name: "name", IsGeometry: false, IsGeography: false}}
	_, _, err := sort.Expr(1)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "geometry or geography")
}

func TestDistanceSortExprRejectsOutOfRangeCoordinates(t *testing.T) {
	t.Parallel()

	sort := DistanceSort{
		Column:    &schema.Column{Name: "location", IsGeometry: true, SRID: 4326},
		Longitude: 181,
		Latitude:  0,
	}
	_, _, err := sort.Expr(1)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "valid WGS84 coordinates")
}
