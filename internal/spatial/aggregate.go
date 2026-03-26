package spatial

import (
	"fmt"

	"github.com/allyourbase/ayb/internal/schema"
)

// BBoxAggregateExpr builds a GeoJSON bbox aggregate expression.
func BBoxAggregateExpr(col *schema.Column) string {
	return fmt.Sprintf(`ST_AsGeoJSON(ST_Extent(%s))::jsonb`, aggregateInputExpr(col))
}

// CentroidAggregateExpr builds a GeoJSON centroid aggregate expression.
func CentroidAggregateExpr(col *schema.Column) string {
	return fmt.Sprintf(`ST_AsGeoJSON(ST_Centroid(ST_Collect(%s)))::jsonb`, aggregateInputExpr(col))
}

func aggregateInputExpr(col *schema.Column) string {
	if col != nil && col.IsGeography {
		return fmt.Sprintf(`%s::geometry`, quotedColumn(col.Name))
	}
	if col == nil {
		return `""`
	}
	return quotedColumn(col.Name)
}
