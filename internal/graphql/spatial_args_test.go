package graphql

import (
	"context"
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
			{Name: "location", TypeName: "geometry", IsGeometry: true, SRID: 4326, GeometryType: "Point"},
			{Name: "name_only", TypeName: "text"},
		},
		PrimaryKey: []string{"id"},
	}
}

func TestParseSpatialArgsNearBuildsFilterSQL(t *testing.T) {
	t.Parallel()

	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	args := map[string]interface{}{
		"near": map[string]interface{}{
			"column":    "location",
			"longitude": -73.98,
			"latitude":  40.76,
			"distance":  1000.0,
		},
	}

	filters, err := parseSpatialArgs(tbl, cache, args)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(filters))

	sql, sqlArgs, err := buildSelectQueryWithSpatial(tbl, nil, filters, nil, 0, 0)
	testutil.NoError(t, err)
	testutil.Contains(t, sql, `ST_DWithin("location", ST_SetSRID(ST_MakePoint($1, $2), 4326), $3)`)
	testutil.Contains(t, sql, `LIMIT $4`)
	testutil.SliceLen(t, sqlArgs, 4)
	testutil.Equal(t, -73.98, sqlArgs[0])
	testutil.Equal(t, 40.76, sqlArgs[1])
	testutil.Equal(t, 1000.0, sqlArgs[2])
	testutil.Equal(t, DefaultMaxLimit, sqlArgs[3])
}

func TestParseSpatialArgsMissingColumnReturnsError(t *testing.T) {
	t.Parallel()

	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	args := map[string]interface{}{
		"near": map[string]interface{}{
			"column":    "does_not_exist",
			"longitude": 0.0,
			"latitude":  0.0,
			"distance":  1.0,
		},
	}

	_, err := parseSpatialArgs(tbl, cache, args)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), `column "does_not_exist" not found`)
}

func TestParseSpatialArgsNonSpatialColumnReturnsError(t *testing.T) {
	t.Parallel()

	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	args := map[string]interface{}{
		"near": map[string]interface{}{
			"column":    "name",
			"longitude": 0.0,
			"latitude":  0.0,
			"distance":  1.0,
		},
	}

	_, err := parseSpatialArgs(tbl, cache, args)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), `is not a spatial column`)
}

func TestParseSpatialArgsInvalidNearInputsReturnError(t *testing.T) {
	t.Parallel()

	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}

	invalidCases := []map[string]interface{}{
		{
			"column":    "location",
			"longitude": 181.0,
			"latitude":  0.0,
			"distance":  1.0,
		},
		{
			"column":    "location",
			"longitude": 0.0,
			"latitude":  91.0,
			"distance":  1.0,
		},
		{
			"column":    "location",
			"longitude": 0.0,
			"latitude":  0.0,
			"distance":  0.0,
		},
	}

	for _, near := range invalidCases {
		_, err := parseSpatialArgs(tbl, cache, map[string]interface{}{"near": near})
		testutil.Error(t, err)
	}
}

func TestParseSpatialArgsWithinRejectsNonPolygon(t *testing.T) {
	t.Parallel()

	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	args := map[string]interface{}{
		"within": map[string]interface{}{
			"column": "location",
			"geojson": map[string]interface{}{
				"type":        "Point",
				"coordinates": []interface{}{0.0, 0.0},
			},
		},
	}

	_, err := parseSpatialArgs(tbl, cache, args)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "within only supports Polygon and MultiPolygon")
}

func TestParseSpatialArgsBBoxRejectsInvertedBounds(t *testing.T) {
	t.Parallel()

	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	args := map[string]interface{}{
		"bbox": map[string]interface{}{
			"column": "location",
			"minLng": 10.0,
			"minLat": 0.0,
			"maxLng": 1.0,
			"maxLat": 5.0,
		},
	}

	_, err := parseSpatialArgs(tbl, cache, args)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "minLng must be less than maxLng")
}

func TestParseSpatialArgsAndWhereUseCorrectPlaceholderOffsets(t *testing.T) {
	t.Parallel()

	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: true}
	filters, err := parseSpatialArgs(tbl, cache, map[string]interface{}{
		"near": map[string]interface{}{
			"column":    "location",
			"longitude": -73.98,
			"latitude":  40.76,
			"distance":  1000.0,
		},
	})
	testutil.NoError(t, err)

	where := map[string]interface{}{
		"name": map[string]interface{}{"_eq": "central"},
	}
	sql, sqlArgs, err := buildSelectQueryWithSpatial(tbl, where, filters, nil, 0, 0)
	testutil.NoError(t, err)
	testutil.Contains(t, sql, `"name" = $1`)
	testutil.Contains(t, sql, `ST_DWithin("location", ST_SetSRID(ST_MakePoint($2, $3), 4326), $4)`)
	testutil.Contains(t, sql, `LIMIT $5`)
	testutil.SliceLen(t, sqlArgs, 5)
	testutil.Equal(t, "central", sqlArgs[0])
}

func TestResolveTableRejectsSpatialArgsWhenPostGISDisabled(t *testing.T) {
	t.Parallel()

	tbl := spatialFixtureTable()
	cache := &schema.SchemaCache{HasPostGIS: false}
	_, err := resolveTable(context.Background(), tbl, nil, cache, map[string]interface{}{
		"near": map[string]interface{}{
			"column":    "location",
			"longitude": 0.0,
			"latitude":  0.0,
			"distance":  10.0,
		},
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "PostGIS")
}
