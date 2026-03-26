// Package api Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/api/spatial_filter.go.
package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/spatial"
)

// parseSpatialParams parses near/within/intersects/bbox filters and returns SQL + args.
func parseSpatialParams(tbl *schema.Table, q url.Values, cache *schema.SchemaCache, argOffset int) (string, []any, error) {
	if tbl == nil {
		return "", nil, fmt.Errorf("table is required for spatial filters")
	}

	nearRaw := strings.TrimSpace(q.Get("near"))
	withinRaw := strings.TrimSpace(q.Get("within"))
	intersectsRaw := strings.TrimSpace(q.Get("intersects"))
	bboxRaw := strings.TrimSpace(q.Get("bbox"))

	if nearRaw == "" && withinRaw == "" && intersectsRaw == "" && bboxRaw == "" {
		return "", nil, nil
	}

	if cache == nil || !cache.HasPostGIS {
		return "", nil, fmt.Errorf("spatial filters require PostGIS extension")
	}

	whereClauses := make([]string, 0, 4)
	args := make([]any, 0, 10)

	if nearRaw != "" {
		clause, filterArgs, err := parseNearSpatial(tbl, nearRaw, argOffset)
		if err != nil {
			return "", nil, err
		}
		whereClauses = append(whereClauses, clause)
		args = append(args, filterArgs...)
		argOffset += len(filterArgs)
	}

	if withinRaw != "" {
		clause, filterArgs, err := parseWithinSpatial(tbl, withinRaw, argOffset)
		if err != nil {
			return "", nil, err
		}
		whereClauses = append(whereClauses, clause)
		args = append(args, filterArgs...)
		argOffset += len(filterArgs)
	}

	if intersectsRaw != "" {
		clause, filterArgs, err := parseIntersectsSpatial(tbl, intersectsRaw, argOffset)
		if err != nil {
			return "", nil, err
		}
		whereClauses = append(whereClauses, clause)
		args = append(args, filterArgs...)
		argOffset += len(filterArgs)
	}

	if bboxRaw != "" {
		clause, filterArgs, err := parseBBoxSpatial(tbl, bboxRaw, argOffset)
		if err != nil {
			return "", nil, err
		}
		whereClauses = append(whereClauses, clause)
		args = append(args, filterArgs...)
	}

	return strings.Join(whereClauses, " AND "), args, nil
}

// TODO: Document parseNearSpatial.
func parseNearSpatial(tbl *schema.Table, raw string, argOffset int) (string, []any, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return "", nil, fmt.Errorf("near must use format: near=column,lng,lat,distance")
	}

	col, err := findSpatialColumn(tbl, strings.TrimSpace(parts[0]))
	if err != nil {
		return "", nil, err
	}

	lng, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return "", nil, fmt.Errorf("near longitude must be a number: %w", err)
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err != nil {
		return "", nil, fmt.Errorf("near latitude must be a number: %w", err)
	}
	if err := spatial.ValidateWGS84Point(lng, lat); err != nil {
		return "", nil, fmt.Errorf("near requires valid WGS84 coordinates: %w", err)
	}

	distance, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	if err != nil {
		return "", nil, fmt.Errorf("near distance must be a number: %w", err)
	}
	if distance <= 0 {
		return "", nil, fmt.Errorf("near distance must be greater than 0")
	}

	filter := spatial.NearFilter{
		Column:    col,
		Longitude: lng,
		Latitude:  lat,
		Distance:  distance,
	}

	return filter.WhereClause(argOffset)
}

// parseGeoJSONSpatialParam splits a "column,{geojson}" parameter, resolves the
// spatial column, and validates the GeoJSON structure. Shared by within and intersects.
func parseGeoJSONSpatialParam(tbl *schema.Table, raw, paramName string) (*schema.Column, string, string, error) {
	parts := strings.SplitN(raw, ",", 2)
	if len(parts) != 2 {
		return nil, "", "", fmt.Errorf("%s must use format: %s=column,{geojson}", paramName, paramName)
	}

	col, err := findSpatialColumn(tbl, strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, "", "", err
	}

	geojson := strings.TrimSpace(parts[1])
	if geojson == "" {
		return nil, "", "", fmt.Errorf("%s requires a GeoJSON value", paramName)
	}

	geomType, err := spatial.ParseGeoJSONGeometry(geojson)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid %s geometry: %w", paramName, err)
	}

	return col, geojson, geomType, nil
}

func parseWithinSpatial(tbl *schema.Table, raw string, argOffset int) (string, []any, error) {
	col, geojson, geomType, err := parseGeoJSONSpatialParam(tbl, raw, "within")
	if err != nil {
		return "", nil, err
	}
	if geomType != "Polygon" && geomType != "MultiPolygon" {
		return "", nil, fmt.Errorf("within only supports Polygon and MultiPolygon")
	}

	filter := spatial.WithinFilter{Column: col, GeoJSON: geojson}
	return filter.WhereClause(argOffset)
}

func parseIntersectsSpatial(tbl *schema.Table, raw string, argOffset int) (string, []any, error) {
	col, geojson, _, err := parseGeoJSONSpatialParam(tbl, raw, "intersects")
	if err != nil {
		return "", nil, err
	}

	filter := spatial.IntersectsFilter{Column: col, GeoJSON: geojson}
	return filter.WhereClause(argOffset)
}

// TODO: Document parseBBoxSpatial.
func parseBBoxSpatial(tbl *schema.Table, raw string, argOffset int) (string, []any, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 5 {
		return "", nil, fmt.Errorf("bbox must use format: bbox=column,minLng,minLat,maxLng,maxLat")
	}

	col, err := findSpatialColumn(tbl, strings.TrimSpace(parts[0]))
	if err != nil {
		return "", nil, err
	}

	minLng, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return "", nil, fmt.Errorf("bbox minLng must be a number: %w", err)
	}
	minLat, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err != nil {
		return "", nil, fmt.Errorf("bbox minLat must be a number: %w", err)
	}
	maxLng, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	if err != nil {
		return "", nil, fmt.Errorf("bbox maxLng must be a number: %w", err)
	}
	maxLat, err := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
	if err != nil {
		return "", nil, fmt.Errorf("bbox maxLat must be a number: %w", err)
	}

	if minLng >= maxLng {
		return "", nil, fmt.Errorf("bbox minLng must be less than maxLng")
	}
	if minLat >= maxLat {
		return "", nil, fmt.Errorf("bbox minLat must be less than maxLat")
	}

	filter := spatial.BBoxFilter{
		Column: col,
		MinLng: minLng,
		MinLat: minLat,
		MaxLng: maxLng,
		MaxLat: maxLat,
	}
	return filter.WhereClause(argOffset)
}

func findSpatialColumn(tbl *schema.Table, name string) (*schema.Column, error) {
	if name == "" {
		return nil, fmt.Errorf("spatial filter column name is required")
	}
	col := tbl.ColumnByName(name)
	if col == nil {
		return nil, fmt.Errorf("column %q not found in table %q", name, tbl.Name)
	}
	if !col.IsGeometry && !col.IsGeography {
		return nil, fmt.Errorf("column %q is not a spatial column", name)
	}
	return col, nil
}
