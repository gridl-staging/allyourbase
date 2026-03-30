package graphql

import (
	"encoding/json"
	"fmt"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/spatial"
)

// parseSpatialArgs extracts near, within, and bbox spatial filter arguments from a GraphQL query, requiring PostGIS to be available in the schema cache.
func parseSpatialArgs(tbl *schema.Table, cache *schema.SchemaCache, args map[string]interface{}) ([]spatial.Filter, error) {
	if tbl == nil {
		return nil, fmt.Errorf("table is required for spatial arguments")
	}

	nearRaw := args["near"]
	withinRaw := args["within"]
	bboxRaw := args["bbox"]
	if nearRaw == nil && withinRaw == nil && bboxRaw == nil {
		return nil, nil
	}

	if cache == nil || !cache.HasPostGIS {
		return nil, fmt.Errorf("spatial filters require PostGIS extension")
	}

	filters := make([]spatial.Filter, 0, 3)
	if nearRaw != nil {
		nearFilter, err := parseNearArg(tbl, nearRaw)
		if err != nil {
			return nil, err
		}
		filters = append(filters, nearFilter)
	}
	if withinRaw != nil {
		withinFilter, err := parseWithinArg(tbl, withinRaw)
		if err != nil {
			return nil, err
		}
		filters = append(filters, withinFilter)
	}
	if bboxRaw != nil {
		bboxFilter, err := parseBBoxArg(tbl, bboxRaw)
		if err != nil {
			return nil, err
		}
		filters = append(filters, bboxFilter)
	}

	return filters, nil
}

// parseNearArg parses a "near" spatial argument into a NearFilter, validating the column is spatial and coordinates are valid WGS84 with a positive distance.
func parseNearArg(tbl *schema.Table, raw interface{}) (spatial.Filter, error) {
	obj, err := requireObjectArg(raw, "near")
	if err != nil {
		return nil, err
	}

	columnName, err := requireStringField(obj, "column", "near")
	if err != nil {
		return nil, err
	}
	col, err := findSpatialColumn(tbl, columnName)
	if err != nil {
		return nil, err
	}

	longitude, err := requireFloatField(obj, "longitude", "near")
	if err != nil {
		return nil, err
	}
	latitude, err := requireFloatField(obj, "latitude", "near")
	if err != nil {
		return nil, err
	}
	distance, err := requireFloatField(obj, "distance", "near")
	if err != nil {
		return nil, err
	}

	if err := spatial.ValidateWGS84Point(longitude, latitude); err != nil {
		return nil, fmt.Errorf("near requires valid WGS84 coordinates: %w", err)
	}
	if distance <= 0 {
		return nil, fmt.Errorf("near distance must be greater than 0")
	}

	return spatial.NearFilter{
		Column:    col,
		Longitude: longitude,
		Latitude:  latitude,
		Distance:  distance,
	}, nil
}

// parseWithinArg parses a "within" spatial argument into a WithinFilter, requiring a Polygon or MultiPolygon GeoJSON geometry on a spatial column.
func parseWithinArg(tbl *schema.Table, raw interface{}) (spatial.Filter, error) {
	obj, err := requireObjectArg(raw, "within")
	if err != nil {
		return nil, err
	}

	columnName, err := requireStringField(obj, "column", "within")
	if err != nil {
		return nil, err
	}
	col, err := findSpatialColumn(tbl, columnName)
	if err != nil {
		return nil, err
	}

	geoJSONValue, ok := obj["geojson"]
	if !ok {
		return nil, fmt.Errorf("within.geojson is required")
	}
	geoJSONString, err := encodeGeoJSONArgument(geoJSONValue)
	if err != nil {
		return nil, fmt.Errorf("invalid within.geojson: %w", err)
	}
	geometryType, err := spatial.ParseGeoJSONGeometry(geoJSONString)
	if err != nil {
		return nil, fmt.Errorf("invalid within geometry: %w", err)
	}
	if geometryType != "Polygon" && geometryType != "MultiPolygon" {
		return nil, fmt.Errorf("within only supports Polygon and MultiPolygon")
	}

	return spatial.WithinFilter{Column: col, GeoJSON: geoJSONString}, nil
}

// parseBBoxArg parses a "bbox" spatial argument into a BBoxFilter, validating that min bounds are strictly less than max bounds on a spatial column.
func parseBBoxArg(tbl *schema.Table, raw interface{}) (spatial.Filter, error) {
	obj, err := requireObjectArg(raw, "bbox")
	if err != nil {
		return nil, err
	}

	columnName, err := requireStringField(obj, "column", "bbox")
	if err != nil {
		return nil, err
	}
	col, err := findSpatialColumn(tbl, columnName)
	if err != nil {
		return nil, err
	}

	minLng, err := requireFloatField(obj, "minLng", "bbox")
	if err != nil {
		return nil, err
	}
	minLat, err := requireFloatField(obj, "minLat", "bbox")
	if err != nil {
		return nil, err
	}
	maxLng, err := requireFloatField(obj, "maxLng", "bbox")
	if err != nil {
		return nil, err
	}
	maxLat, err := requireFloatField(obj, "maxLat", "bbox")
	if err != nil {
		return nil, err
	}

	if minLng >= maxLng {
		return nil, fmt.Errorf("bbox minLng must be less than maxLng")
	}
	if minLat >= maxLat {
		return nil, fmt.Errorf("bbox minLat must be less than maxLat")
	}

	return spatial.BBoxFilter{
		Column: col,
		MinLng: minLng,
		MinLat: minLat,
		MaxLng: maxLng,
		MaxLat: maxLat,
	}, nil
}

func findSpatialColumn(tbl *schema.Table, columnName string) (*schema.Column, error) {
	if columnName == "" {
		return nil, fmt.Errorf("spatial filter column name is required")
	}
	col := tbl.ColumnByName(columnName)
	if col == nil {
		return nil, fmt.Errorf("column %q not found in table %q", columnName, tbl.Name)
	}
	if !col.IsGeometry && !col.IsGeography {
		return nil, fmt.Errorf("column %q is not a spatial column", columnName)
	}
	return col, nil
}

func requireObjectArg(raw interface{}, argumentName string) (map[string]interface{}, error) {
	objectValue, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be an input object", argumentName)
	}
	return objectValue, nil
}

func requireStringField(obj map[string]interface{}, fieldName, argumentName string) (string, error) {
	raw, ok := obj[fieldName]
	if !ok {
		return "", fmt.Errorf("%s.%s is required", argumentName, fieldName)
	}
	value, ok := raw.(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s.%s must be a non-empty string", argumentName, fieldName)
	}
	return value, nil
}

func requireFloatField(obj map[string]interface{}, fieldName, argumentName string) (float64, error) {
	raw, ok := obj[fieldName]
	if !ok {
		return 0, fmt.Errorf("%s.%s is required", argumentName, fieldName)
	}
	value, ok := toSpatialFloat64(raw)
	if !ok {
		return 0, fmt.Errorf("%s.%s must be a number", argumentName, fieldName)
	}
	return value, nil
}

// toSpatialFloat64 converts numeric types (float32, int, int32, int64) to float64 for spatial argument parsing.
func toSpatialFloat64(raw interface{}) (float64, bool) {
	switch typed := raw.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

// encodeGeoJSONArgument converts a GeoJSON value (map, string, or bytes) into a JSON string suitable for PostGIS ST_GeomFromGeoJSON.
func encodeGeoJSONArgument(raw interface{}) (string, error) {
	switch typed := raw.(type) {
	case map[string]interface{}:
		payload, err := json.Marshal(typed)
		if err != nil {
			return "", err
		}
		return string(payload), nil
	case string:
		if typed == "" {
			return "", fmt.Errorf("geojson string must not be empty")
		}
		return typed, nil
	case []byte:
		if len(typed) == 0 {
			return "", fmt.Errorf("geojson bytes must not be empty")
		}
		return string(typed), nil
	default:
		payload, err := json.Marshal(raw)
		if err != nil {
			return "", err
		}
		return string(payload), nil
	}
}
