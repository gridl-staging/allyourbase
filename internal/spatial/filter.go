package spatial

import (
	"fmt"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
)

const wgs84SRID = 4326

const (
	minLongitude = -180.0
	maxLongitude = 180.0
	minLatitude  = -90.0
	maxLatitude  = 90.0
)

// Filter defines a spatial WHERE clause builder with parameterized SQL support.
type Filter interface {
	WhereClause(paramOffset int) (string, []any, error)
}

// NearFilter filters rows within a distance of a WGS84 point.
type NearFilter struct {
	Column    *schema.Column
	Longitude float64
	Latitude  float64
	Distance  float64
}

// WithinFilter filters rows that are within the given GeoJSON geometry.
type WithinFilter struct {
	Column  *schema.Column
	GeoJSON string
}

// IntersectsFilter filters rows that intersect the given GeoJSON geometry.
type IntersectsFilter struct {
	Column  *schema.Column
	GeoJSON string
}

// BBoxFilter filters rows inside an axis-aligned envelope (minLng,minLat,maxLng,maxLat).
type BBoxFilter struct {
	Column *schema.Column
	MinLng float64
	MinLat float64
	MaxLng float64
	MaxLat float64
}

// ValidateWGS84Point ensures a longitude/latitude pair is valid in WGS84.
func ValidateWGS84Point(longitude, latitude float64) error {
	if longitude < minLongitude || longitude > maxLongitude {
		return fmt.Errorf("longitude must be between %v and %v", minLongitude, maxLongitude)
	}
	if latitude < minLatitude || latitude > maxLatitude {
		return fmt.Errorf("latitude must be between %v and %v", minLatitude, maxLatitude)
	}
	return nil
}

// WhereClause builds a geography-aware or geometry-aware ST_DWithin expression.
func (f NearFilter) WhereClause(paramOffset int) (string, []any, error) {
	if f.Column == nil {
		return "", nil, fmt.Errorf("near filter requires a spatial column")
	}
	if err := ValidateWGS84Point(f.Longitude, f.Latitude); err != nil {
		return "", nil, fmt.Errorf("near filter requires valid WGS84 coordinates: %w", err)
	}

	pointExpr := wgs84PointExpression(paramOffset, paramOffset+1)
	if f.Column.IsGeography {
		return fmt.Sprintf(`ST_DWithin(%s::geography, %s::geography, $%d)`,
			quotedColumn(f.Column.Name), pointExpr, paramOffset+2), []any{f.Longitude, f.Latitude, f.Distance}, nil
	}

	if f.Column.IsGeometry {
		pointExpr = transformToColumnSRID(pointExpr, f.Column.SRID)
		return fmt.Sprintf(`ST_DWithin(%s, %s, $%d)`, quotedColumn(f.Column.Name), pointExpr, paramOffset+2),
			[]any{f.Longitude, f.Latitude, f.Distance}, nil
	}

	return "", nil, fmt.Errorf("near filter requires geometry or geography column")
}

// WhereClause builds a topology-aware ST_Within/ST_Covers expression.
func (f WithinFilter) WhereClause(paramOffset int) (string, []any, error) {
	if f.Column == nil {
		return "", nil, fmt.Errorf("within filter requires a spatial column")
	}

	geomExpr := geomJSONExpression(f.Column, paramOffset)
	if f.Column.IsGeography {
		return fmt.Sprintf(`ST_Covers(%s, %s::geography)`, geomExpr, quotedColumn(f.Column.Name)),
			[]any{f.GeoJSON}, nil
	}
	if f.Column.IsGeometry {
		return fmt.Sprintf(`ST_Within(%s, %s)`, quotedColumn(f.Column.Name), geomExpr),
			[]any{f.GeoJSON}, nil
	}

	return "", nil, fmt.Errorf("within filter requires geometry or geography column")
}

// WhereClause builds an ST_Intersects expression.
func (f IntersectsFilter) WhereClause(paramOffset int) (string, []any, error) {
	if f.Column == nil {
		return "", nil, fmt.Errorf("intersects filter requires a spatial column")
	}

	geomExpr := geomJSONExpression(f.Column, paramOffset)
	if f.Column.IsGeography {
		return fmt.Sprintf(`ST_Intersects(%s::geography, %s)`, quotedColumn(f.Column.Name), geomExpr),
			[]any{f.GeoJSON}, nil
	}
	if f.Column.IsGeometry {
		return fmt.Sprintf(`ST_Intersects(%s, %s)`, quotedColumn(f.Column.Name), geomExpr),
			[]any{f.GeoJSON}, nil
	}

	return "", nil, fmt.Errorf("intersects filter requires geometry or geography column")
}

// WhereClause builds an ST_Intersects expression using ST_MakeEnvelope.
func (f BBoxFilter) WhereClause(paramOffset int) (string, []any, error) {
	if f.Column == nil {
		return "", nil, fmt.Errorf("bbox filter requires a spatial column")
	}

	envelopeExpr := envelopeExpression(f.Column, paramOffset)
	if f.Column.IsGeography {
		return fmt.Sprintf(`ST_Intersects(%s::geography, %s::geography)`, quotedColumn(f.Column.Name), envelopeExpr),
			[]any{f.MinLng, f.MinLat, f.MaxLng, f.MaxLat}, nil
	}
	if f.Column.IsGeometry {
		return fmt.Sprintf(`ST_Intersects(%s, %s)`, quotedColumn(f.Column.Name), envelopeExpr),
			[]any{f.MinLng, f.MinLat, f.MaxLng, f.MaxLat}, nil
	}

	return "", nil, fmt.Errorf("bbox filter requires geometry or geography column")
}

func quotedColumn(name string) string {
	return sqlutil.QuoteIdent(name)
}

func wgs84PointExpression(lngParamIdx, latParamIdx int) string {
	return fmt.Sprintf("ST_SetSRID(ST_MakePoint($%d, $%d), %d)", lngParamIdx, latParamIdx, wgs84SRID)
}

func transformToColumnSRID(expr string, columnSRID int) string {
	if columnSRID <= 0 || columnSRID == wgs84SRID {
		return expr
	}
	return fmt.Sprintf("ST_Transform(%s, %d)", expr, columnSRID)
}

func geomJSONExpression(col *schema.Column, jsonParamIdx int) string {
	base := fmt.Sprintf("ST_GeomFromGeoJSON($%d)", jsonParamIdx)
	if col == nil || col.IsGeography {
		return base + "::geography"
	}
	if !col.IsGeometry {
		return base
	}
	return transformToColumnSRID(base, col.SRID)
}

func envelopeExpression(col *schema.Column, baseParamIdx int) string {
	base := fmt.Sprintf("ST_MakeEnvelope($%d, $%d, $%d, $%d, %d)", baseParamIdx, baseParamIdx+1, baseParamIdx+2, baseParamIdx+3, wgs84SRID)
	if col != nil && !col.IsGeography {
		return transformToColumnSRID(base, col.SRID)
	}
	return base
}
