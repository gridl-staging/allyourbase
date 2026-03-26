package spatial

import (
	"encoding/json"
	"fmt"
	"strings"
)

var validGeoJSONGeometryTypes = map[string]struct{}{
	"Point":              {},
	"MultiPoint":         {},
	"LineString":         {},
	"MultiLineString":    {},
	"Polygon":            {},
	"MultiPolygon":       {},
	"GeometryCollection": {},
	"Feature":            {},
	"FeatureCollection":  {},
}

// ParseGeoJSONGeometry validates GeoJSON and returns the top-level geometry type.
func ParseGeoJSONGeometry(input string) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return "", fmt.Errorf("invalid GeoJSON: %w", err)
	}

	rawType, ok := payload["type"]
	if !ok {
		return "", fmt.Errorf("GeoJSON geometry must include a type field")
	}

	geomType, ok := rawType.(string)
	if !ok {
		return "", fmt.Errorf("GeoJSON geometry type must be a string")
	}
	geomType = strings.TrimSpace(geomType)
	if geomType == "" {
		return "", fmt.Errorf("GeoJSON geometry type must be a non-empty string")
	}

	if _, ok := validGeoJSONGeometryTypes[geomType]; !ok {
		return "", fmt.Errorf("unsupported GeoJSON geometry type %q", geomType)
	}

	switch geomType {
	case "Feature", "FeatureCollection":
		return "", fmt.Errorf("GeoJSON Feature and FeatureCollection are not supported; pass geometry directly")
	case "GeometryCollection":
		if rawGeoms, ok := payload["geometries"]; !ok || rawGeoms == nil {
			return "", fmt.Errorf("GeometryCollection requires a geometries field")
		}
	default:
		if rawCoords, ok := payload["coordinates"]; !ok || rawCoords == nil {
			return "", fmt.Errorf("%s geometry requires a coordinates field", geomType)
		}
	}

	return geomType, nil
}
