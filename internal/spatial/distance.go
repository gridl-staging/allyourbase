package spatial

import (
	"fmt"

	"github.com/allyourbase/ayb/internal/schema"
)

// DistanceSort builds a reusable ST_Distance expression for ORDER BY clauses.
type DistanceSort struct {
	Column    *schema.Column
	Longitude float64
	Latitude  float64
}

// Expr returns a parameterized ST_Distance expression and its args.
func (s DistanceSort) Expr(paramOffset int) (string, []any, error) {
	if s.Column == nil {
		return "", nil, fmt.Errorf("distance sort requires a spatial column")
	}
	if err := ValidateWGS84Point(s.Longitude, s.Latitude); err != nil {
		return "", nil, fmt.Errorf("distance sort requires valid WGS84 coordinates: %w", err)
	}

	pointExpr := wgs84PointExpression(paramOffset, paramOffset+1)
	args := []any{s.Longitude, s.Latitude}

	if s.Column.IsGeography {
		return fmt.Sprintf(`ST_Distance(%s::geography, %s::geography)`, quotedColumn(s.Column.Name), pointExpr), args, nil
	}

	if s.Column.IsGeometry {
		pointExpr = transformToColumnSRID(pointExpr, s.Column.SRID)
		return fmt.Sprintf(`ST_Distance(%s, %s)`, quotedColumn(s.Column.Name), pointExpr), args, nil
	}

	return "", nil, fmt.Errorf("distance sort requires a geometry or geography column")
}
