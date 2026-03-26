package api

import (
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func sortFixtureTable() *schema.Table {
	return &schema.Table{
		Schema: "public",
		Name:   "places",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", TypeName: "text"},
			{Name: "created_at", TypeName: "timestamp"},
			{Name: "location", TypeName: "geometry(Point,4326)", IsGeometry: true, SRID: 4326},
		},
		PrimaryKey: []string{"id"},
	}
}

func TestParseStructuredSortValidDistanceWithSecondarySorts(t *testing.T) {
	t.Parallel()

	tbl := sortFixtureTable()
	sort, err := parseStructuredSort(tbl, "location.distance(-73.99,40.73),-created_at,+name", true)
	testutil.NoError(t, err)
	testutil.SliceLen(t, sort.Terms, 3)
	testutil.True(t, sort.Terms[0].Distance != nil, "expected first sort term to be distance")
	testutil.Equal(t, "created_at", sort.Terms[1].Column.Name)
	testutil.Equal(t, true, sort.Terms[1].Desc)
	testutil.Equal(t, "name", sort.Terms[2].Column.Name)
	testutil.Equal(t, false, sort.Terms[2].Desc)
}

func TestParseStructuredSortRejectsDistanceOnMissingColumn(t *testing.T) {
	t.Parallel()

	_, err := parseStructuredSort(sortFixtureTable(), "missing.distance(1,2)", true)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "not found")
}

func TestParseStructuredSortRejectsDistanceOnNonSpatialColumn(t *testing.T) {
	t.Parallel()

	_, err := parseStructuredSort(sortFixtureTable(), "name.distance(1,2)", true)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "not a spatial column")
}

func TestParseStructuredSortRejectsMalformedCoordinates(t *testing.T) {
	t.Parallel()

	_, err := parseStructuredSort(sortFixtureTable(), "location.distance(abc,2)", true)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "longitude")
}

func TestParseStructuredSortRejectsOutOfRangeCoordinates(t *testing.T) {
	t.Parallel()

	_, err := parseStructuredSort(sortFixtureTable(), "location.distance(181,2)", true)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "valid WGS84 coordinates")
}

func TestParseStructuredSortRejectsDuplicateDistanceTerms(t *testing.T) {
	t.Parallel()

	_, err := parseStructuredSort(sortFixtureTable(), "location.distance(1,2),location.distance(2,3)", true)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "at most one")
}

func TestParseStructuredSortRejectsDistanceTermInNonPrimaryPosition(t *testing.T) {
	t.Parallel()

	_, err := parseStructuredSort(sortFixtureTable(), "-name,location.distance(1,2)", true)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "first")
}

func TestParseStructuredSortHonorsMaxSortFields(t *testing.T) {
	t.Parallel()

	tbl := sortFixtureTable()
	sort, err := parseStructuredSort(tbl, "name,created_at,id,name,created_at,id,name,created_at,id,name,created_at,id", true)
	testutil.NoError(t, err)
	testutil.Equal(t, maxSortFields, len(sort.Terms))
}

func TestParseStructuredSortRequiresPostGISForDistance(t *testing.T) {
	t.Parallel()

	_, err := parseStructuredSort(sortFixtureTable(), "location.distance(1,2)", false)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "PostGIS")
}

func TestParseStructuredSortRejectsDistanceWhenDistanceOutputColumnExists(t *testing.T) {
	t.Parallel()

	tbl := sortFixtureTable()
	tbl.Columns = append(tbl.Columns, &schema.Column{Name: distanceSortOutputColumn, TypeName: "text"})

	_, err := parseStructuredSort(tbl, "location.distance(1,2)", true)
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), distanceSortOutputColumn)
}
