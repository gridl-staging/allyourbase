// Package mcp Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/mcp/server_tools_spatial.go.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/spatial"
)

const spatialQueryMaxPerPage = 500

type spatialPaginationPlan struct {
	requestPage     int
	requestPerPage  int
	responsePage    int
	responsePerPage int
	sliceStart      int
}

func registerSpatialTools(s *mcp.Server, c *apiClient) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "spatial_info",
		Description: "Get PostGIS status, spatial tables/columns, SRIDs, geometry types, and missing spatial indexes",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in SpatialInfoInput) (*mcp.CallToolResult, SpatialInfoOutput, error) {
		return handleSpatialInfo(ctx, c)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "spatial_query",
		Description: "Run a spatial query against a table using near/within/intersects/bbox filters through AYB REST",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in SpatialQueryInput) (*mcp.CallToolResult, SpatialQueryOutput, error) {
		return handleSpatialQuery(ctx, c, in)
	})
}

func handleSpatialInfo(ctx context.Context, c *apiClient) (*mcp.CallToolResult, SpatialInfoOutput, error) {
	result, _, err := c.doJSON(ctx, "GET", "/api/schema", nil, false)
	if err != nil {
		return nil, SpatialInfoOutput{}, err
	}

	cache, err := decodeSchemaCache(result)
	if err != nil {
		return nil, SpatialInfoOutput{}, err
	}

	return nil, schema.BuildSpatialInfoSummary(cache), nil
}

// TODO: Document handleSpatialQuery.
func handleSpatialQuery(ctx context.Context, c *apiClient, in SpatialQueryInput) (*mcp.CallToolResult, SpatialQueryOutput, error) {
	if strings.TrimSpace(in.Table) == "" {
		return nil, SpatialQueryOutput{}, fmt.Errorf("table is required")
	}
	if strings.TrimSpace(in.Column) == "" {
		return nil, SpatialQueryOutput{}, fmt.Errorf("column is required")
	}

	params := url.Values{}
	paginationPlan, err := buildSpatialPaginationPlan(in.Limit, in.Offset)
	if err != nil {
		return nil, SpatialQueryOutput{}, err
	}
	filterType := strings.ToLower(strings.TrimSpace(in.FilterType))
	switch filterType {
	case "near":
		if in.Longitude == nil || in.Latitude == nil || in.Distance == nil {
			return nil, SpatialQueryOutput{}, fmt.Errorf("near requires longitude, latitude, and distance")
		}
		if err := spatial.ValidateWGS84Point(*in.Longitude, *in.Latitude); err != nil {
			return nil, SpatialQueryOutput{}, fmt.Errorf("near requires valid WGS84 coordinates: %w", err)
		}
		if *in.Distance <= 0 {
			return nil, SpatialQueryOutput{}, fmt.Errorf("distance must be greater than 0")
		}
		params.Set("near", strings.Join([]string{
			in.Column,
			formatSpatialNumber(*in.Longitude),
			formatSpatialNumber(*in.Latitude),
			formatSpatialNumber(*in.Distance),
		}, ","))

	case "within":
		if strings.TrimSpace(in.GeoJSON) == "" {
			return nil, SpatialQueryOutput{}, fmt.Errorf("within requires geojson")
		}
		params.Set("within", in.Column+","+in.GeoJSON)

	case "intersects":
		if strings.TrimSpace(in.GeoJSON) == "" {
			return nil, SpatialQueryOutput{}, fmt.Errorf("intersects requires geojson")
		}
		params.Set("intersects", in.Column+","+in.GeoJSON)

	case "bbox":
		if in.MinLng == nil || in.MinLat == nil || in.MaxLng == nil || in.MaxLat == nil {
			return nil, SpatialQueryOutput{}, fmt.Errorf("bbox requires min_lng, min_lat, max_lng, and max_lat")
		}
		if *in.MinLng >= *in.MaxLng {
			return nil, SpatialQueryOutput{}, fmt.Errorf("min_lng must be less than max_lng")
		}
		if *in.MinLat >= *in.MaxLat {
			return nil, SpatialQueryOutput{}, fmt.Errorf("min_lat must be less than max_lat")
		}
		params.Set("bbox", strings.Join([]string{
			in.Column,
			formatSpatialNumber(*in.MinLng),
			formatSpatialNumber(*in.MinLat),
			formatSpatialNumber(*in.MaxLng),
			formatSpatialNumber(*in.MaxLat),
		}, ","))

	default:
		return nil, SpatialQueryOutput{}, fmt.Errorf("filter_type must be one of near, within, intersects, bbox")
	}

	if in.Filter != "" {
		params.Set("filter", in.Filter)
	}
	if in.Sort != "" {
		params.Set("sort", in.Sort)
	}
	if paginationPlan.requestPerPage > 0 {
		params.Set("page", strconv.Itoa(paginationPlan.requestPage))
		params.Set("perPage", strconv.Itoa(paginationPlan.requestPerPage))
	}

	path := "/api/collections/" + url.PathEscape(in.Table)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	result, _, err := c.doJSON(ctx, "GET", path, nil, false)
	if err != nil {
		return nil, SpatialQueryOutput{}, err
	}

	out := decodeSpatialQueryOutput(result)
	if paginationPlan.responsePerPage > 0 {
		out.Items = sliceSpatialItems(out.Items, paginationPlan.sliceStart, paginationPlan.responsePerPage)
		out.Page = paginationPlan.responsePage
		out.PerPage = paginationPlan.responsePerPage
		out.TotalPages = calculateSpatialTotalPages(out.TotalItems, paginationPlan.responsePerPage)
	}
	return nil, out, nil
}

func formatSpatialNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// TODO: Document buildSpatialPaginationPlan.
func buildSpatialPaginationPlan(limit, offset *int) (spatialPaginationPlan, error) {
	if offset != nil && *offset < 0 {
		return spatialPaginationPlan{}, fmt.Errorf("offset must be greater than or equal to 0")
	}
	if limit == nil {
		if offset != nil && *offset > 0 {
			return spatialPaginationPlan{}, fmt.Errorf("offset requires limit because AYB REST pagination is page-based")
		}
		return spatialPaginationPlan{}, nil
	}
	if *limit <= 0 {
		return spatialPaginationPlan{}, fmt.Errorf("limit must be greater than 0")
	}
	if *limit > spatialQueryMaxPerPage {
		return spatialPaginationPlan{}, fmt.Errorf("limit must be less than or equal to %d", spatialQueryMaxPerPage)
	}

	requestPage, requestPerPage, sliceStart, err := translateSpatialOffsetWindow(*limit, intValue(offset), spatialQueryMaxPerPage)
	if err != nil {
		return spatialPaginationPlan{}, err
	}
	return spatialPaginationPlan{
		requestPage:     requestPage,
		requestPerPage:  requestPerPage,
		responsePage:    intValue(offset)/(*limit) + 1,
		responsePerPage: *limit,
		sliceStart:      sliceStart,
	}, nil
}

func translateSpatialOffsetWindow(limit, offset, maxPerPage int) (page, perPage, sliceStart int, err error) {
	for candidatePerPage := limit; candidatePerPage <= maxPerPage; candidatePerPage++ {
		candidateSliceStart := offset % candidatePerPage
		if candidateSliceStart+limit > candidatePerPage {
			continue
		}
		return offset/candidatePerPage + 1, candidatePerPage, candidateSliceStart, nil
	}
	return 0, 0, 0, fmt.Errorf(
		"offset %d with limit %d cannot be represented through AYB REST page/perPage pagination",
		offset,
		limit,
	)
}

// TODO: Document decodeSpatialQueryOutput.
func decodeSpatialQueryOutput(result map[string]any) SpatialQueryOutput {
	out := SpatialQueryOutput{Items: []map[string]any{}}
	if items, ok := result["items"].([]any); ok {
		for _, item := range items {
			if itemMap, ok := item.(map[string]any); ok {
				out.Items = append(out.Items, itemMap)
			}
		}
	}
	if page, ok := result["page"].(float64); ok {
		out.Page = int(page)
	}
	if perPage, ok := result["perPage"].(float64); ok {
		out.PerPage = int(perPage)
	}
	if totalItems, ok := result["totalItems"].(float64); ok {
		out.TotalItems = int(totalItems)
	}
	if totalPages, ok := result["totalPages"].(float64); ok {
		out.TotalPages = int(totalPages)
	}
	return out
}

func sliceSpatialItems(items []map[string]any, start, count int) []map[string]any {
	if start >= len(items) {
		return []map[string]any{}
	}
	end := start + count
	if end > len(items) {
		end = len(items)
	}
	return append([]map[string]any(nil), items[start:end]...)
}

func calculateSpatialTotalPages(totalItems, perPage int) int {
	if totalItems <= 0 || perPage <= 0 {
		return 0
	}
	return int(math.Ceil(float64(totalItems) / float64(perPage)))
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

// TODO: Document decodeSchemaCache.
func decodeSchemaCache(payload map[string]any) (*schema.SchemaCache, error) {
	cache := &schema.SchemaCache{
		Tables: make(map[string]*schema.Table),
	}
	if hasPostGIS, ok := payload["hasPostGIS"].(bool); ok {
		cache.HasPostGIS = hasPostGIS
	}
	if postGISVersion, ok := payload["postGISVersion"].(string); ok {
		cache.PostGISVersion = postGISVersion
	}

	tablesMap, _ := payload["tables"].(map[string]any)
	for _, tableRaw := range tablesMap {
		tableMap, ok := tableRaw.(map[string]any)
		if !ok {
			continue
		}
		raw, err := json.Marshal(tableMap)
		if err != nil {
			return nil, fmt.Errorf("marshal table schema: %w", err)
		}
		var table schema.Table
		if err := json.Unmarshal(raw, &table); err != nil {
			return nil, fmt.Errorf("decode table schema: %w", err)
		}
		if table.Name == "" {
			continue
		}
		if table.Schema == "" {
			table.Schema = "public"
		}
		cache.Tables[table.Schema+"."+table.Name] = &table
	}
	return cache, nil
}
