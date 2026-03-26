package spatial

import (
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBBoxAggregateExprGeometry(t *testing.T) {
	t.Parallel()

	col := &schema.Column{Name: "location", IsGeometry: true, IsGeography: false}
	testutil.Equal(t, `ST_AsGeoJSON(ST_Extent("location"))::jsonb`, BBoxAggregateExpr(col))
}

func TestBBoxAggregateExprGeography(t *testing.T) {
	t.Parallel()

	col := &schema.Column{Name: "location", IsGeometry: true, IsGeography: true}
	testutil.Equal(t, `ST_AsGeoJSON(ST_Extent("location"::geometry))::jsonb`, BBoxAggregateExpr(col))
}

func TestCentroidAggregateExprGeometry(t *testing.T) {
	t.Parallel()

	col := &schema.Column{Name: "location", IsGeometry: true, IsGeography: false}
	testutil.Equal(t, `ST_AsGeoJSON(ST_Centroid(ST_Collect("location")))::jsonb`, CentroidAggregateExpr(col))
}

func TestCentroidAggregateExprGeography(t *testing.T) {
	t.Parallel()

	col := &schema.Column{Name: "location", IsGeometry: true, IsGeography: true}
	testutil.Equal(t, `ST_AsGeoJSON(ST_Centroid(ST_Collect("location"::geometry)))::jsonb`, CentroidAggregateExpr(col))
}

func TestSpatialAggregateExprQuotesIdentifier(t *testing.T) {
	t.Parallel()

	col := &schema.Column{Name: `co"l`, IsGeometry: true}
	testutil.Contains(t, BBoxAggregateExpr(col), `"co""l"`)
	testutil.Contains(t, CentroidAggregateExpr(col), `"co""l"`)
}
