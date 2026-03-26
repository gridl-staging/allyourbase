//go:build integration

package graphql

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSpatialResolverGuardReturnsErrorWhenPostGISDisabled(t *testing.T) {
	t.Parallel()

	tbl := &schema.Table{
		Schema: "public",
		Name:   "places",
		Kind:   "table",
		Columns: []*schema.Column{
			{Name: "id", TypeName: "integer"},
			{Name: "location", TypeName: "geometry", IsGeometry: true, GeometryType: "Point"},
		},
	}

	_, err := resolveTable(context.Background(), tbl, nil, &schema.SchemaCache{HasPostGIS: false}, map[string]interface{}{
		"near": map[string]interface{}{
			"column":    "location",
			"longitude": 0.0,
			"latitude":  0.0,
			"distance":  100.0,
		},
	})
	testutil.Error(t, err)
	testutil.Contains(t, err.Error(), "PostGIS")
}
