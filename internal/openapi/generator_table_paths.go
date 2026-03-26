// Package openapi Generates OpenAPI paths and operations for REST API table collection and record endpoints.
package openapi

import (
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

func collectionPathForTable(basePath, tableName string) string {
	base := normalizeBasePath(basePath)
	if base == "" {
		// Backward-compatible default: table paths at root.
		return "/" + tableName
	}
	return base + "/collections/" + tableName
}

func tableComponentName(tableName string, suffix string) string {
	if len(tableName) == 0 {
		return suffix
	}
	name := strings.ToUpper(tableName[0:1]) + tableName[1:]
	if suffix != "" {
		return name + suffix
	}
	return name
}

// buildTableCollectionPath creates GET (list) and POST (create) operations for a table.
func buildTableCollectionPath(tbl *schema.Table, rowSchema, createSchema *schemaProperty, cache *schema.SchemaCache) *pathItem {
	return &pathItem{
		Get:  buildListOp(tbl, rowSchema, cache),
		Post: buildInsertOp(tbl, createSchema, rowSchema),
	}
}

// buildTableRecordPath creates GET (read), PATCH (update), and DELETE operations
// for a single record addressed by primary key.
func buildTableRecordPath(tbl *schema.Table, rowSchema, createSchema *schemaProperty) *pathItem {
	return &pathItem{
		Get:    buildReadOp(tbl, rowSchema),
		Patch:  buildUpdateOp(tbl, createSchema, rowSchema),
		Delete: buildDeleteOp(tbl),
	}
}

func tableRowSchema(tbl *schema.Table, emitGeoJSONComponents bool) *schemaProperty {
	props := make(map[string]*schemaProperty, len(tbl.Columns))
	for _, col := range tbl.Columns {
		props[col.Name] = columnToPropertyWithGeoJSONRefs(col, emitGeoJSONComponents)
	}
	return &schemaProperty{Type: "object", Properties: props}
}

func tableCreateSchema(tbl *schema.Table, emitGeoJSONComponents bool) *schemaProperty {
	props := make(map[string]*schemaProperty, len(tbl.Columns))
	for _, col := range tbl.Columns {
		// Skip PK and columns with defaults for create schema.
		if col.IsPrimaryKey {
			continue
		}
		prop := columnToPropertyWithGeoJSONRefs(col, emitGeoJSONComponents)
		prop.ReadOnly = false // writable in create
		props[col.Name] = prop
	}
	return &schemaProperty{Type: "object", Properties: props}
}

func tableWriteSchema(tbl *schema.Table, emitGeoJSONComponents bool) *schemaProperty {
	props := make(map[string]*schemaProperty, len(tbl.Columns))
	for _, col := range tbl.Columns {
		// Import and batch bodies can include any recognized table column.
		prop := columnToPropertyWithGeoJSONRefs(col, emitGeoJSONComponents)
		prop.ReadOnly = false
		props[col.Name] = prop
	}
	return &schemaProperty{Type: "object", Properties: props}
}

// buildListOp creates an OpenAPI GET operation for listing rows in a table.
func buildListOp(tbl *schema.Table, rowSchema *schemaProperty, cache *schema.SchemaCache) *operation {
	return &operation{
		Summary:     "List " + tbl.Name + " rows",
		Tags:        []string{tbl.Name},
		OperationID: "list_" + tbl.Name,
		Parameters:  listQueryParams(tbl, cache),
		Responses: map[string]*response{
			"200": {
				Description: "Successful response",
				Content: map[string]*mediaContent{
					"application/json": {
						Schema: &schemaProperty{
							Type:  "array",
							Items: rowSchema,
						},
					},
				},
			},
		},
	}
}

// buildReadOp creates an OpenAPI GET operation for reading a single record from a table.
func buildReadOp(tbl *schema.Table, rowSchema *schemaProperty) *operation {
	return &operation{
		Summary:     "Read a single " + tbl.Name + " record",
		Tags:        []string{tbl.Name},
		OperationID: "read_" + tbl.Name,
		Parameters:  []*parameter{idPathParam()},
		Responses: map[string]*response{
			"200": {
				Description: "Successful response",
				Content: map[string]*mediaContent{
					"application/json": {Schema: rowSchema},
				},
			},
		},
	}
}

// buildInsertOp creates an OpenAPI POST operation for inserting rows into a table.
func buildInsertOp(tbl *schema.Table, createSchema, rowSchema *schemaProperty) *operation {
	return &operation{
		Summary:     "Insert into " + tbl.Name,
		Tags:        []string{tbl.Name},
		OperationID: "insert_" + tbl.Name,
		RequestBody: &requestBody{
			Required: true,
			Content: map[string]*mediaContent{
				"application/json": {Schema: createSchema},
			},
		},
		Responses: map[string]*response{
			"201": {
				Description: "Created",
				Content: map[string]*mediaContent{
					"application/json": {Schema: rowSchema},
				},
			},
		},
	}
}

// buildUpdateOp creates an OpenAPI PATCH operation for updating a record in a table.
func buildUpdateOp(tbl *schema.Table, createSchema, rowSchema *schemaProperty) *operation {
	return &operation{
		Summary:     "Update a " + tbl.Name + " record",
		Tags:        []string{tbl.Name},
		OperationID: "update_" + tbl.Name,
		Parameters:  []*parameter{idPathParam()},
		RequestBody: &requestBody{
			Required: true,
			Content: map[string]*mediaContent{
				"application/json": {Schema: createSchema},
			},
		},
		Responses: map[string]*response{
			"200": {
				Description: "Updated",
				Content: map[string]*mediaContent{
					"application/json": {Schema: rowSchema},
				},
			},
		},
	}
}

func buildDeleteOp(tbl *schema.Table) *operation {
	return &operation{
		Summary:     "Delete a " + tbl.Name + " record",
		Tags:        []string{tbl.Name},
		OperationID: "delete_" + tbl.Name,
		Parameters:  []*parameter{idPathParam()},
		Responses: map[string]*response{
			"204": {
				Description: "Deleted",
			},
		},
	}
}

// idPathParam returns the {id} path parameter for record-level operations.
func idPathParam() *parameter {
	return &parameter{
		Name:        "id",
		In:          "path",
		Required:    true,
		Description: "Record primary key",
		Schema:      &schemaProperty{Type: "string"},
	}
}

// listQueryParams produces the query parameters for GET list endpoints.
// These match the actual AYB REST query engine in internal/api/handler.go.
func listQueryParams(tbl *schema.Table, cache *schema.SchemaCache) []*parameter {
	params := []*parameter{
		{
			Name:        "fields",
			In:          "query",
			Description: "Comma-separated list of columns to return",
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "filter",
			In:          "query",
			Description: "Filter expression (e.g. status='active' && age>25). Operators: =, !=, >, >=, <, <=, ~ (LIKE), !~ (NOT LIKE), IN, IS NULL. Combine with && (AND) or || (OR). Parentheses supported.",
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "sort",
			In:          "query",
			Description: "Sort expression: column name with optional - prefix for descending (comma-separated for multiple)",
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "page",
			In:          "query",
			Description: "Page number (1-indexed, default 1)",
			Schema:      &schemaProperty{Type: "integer"},
		},
		{
			Name:        "perPage",
			In:          "query",
			Description: "Number of rows per page (default 20, max 500)",
			Schema:      &schemaProperty{Type: "integer"},
		},
		{
			Name:        "search",
			In:          "query",
			Description: "Full-text search term across text columns",
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "skipTotal",
			In:          "query",
			Description: "Set to 'true' to skip total count for faster pagination",
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "cursor",
			In:          "query",
			Description: "Opaque cursor for pagination (use nextCursor from previous response)",
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "direction",
			In:          "query",
			Description: "Pagination direction: 'forward' or 'backward' (default forward)",
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "aggregate",
			In:          "query",
			Description: aggregateParamDescription(tbl, cache),
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "group",
			In:          "query",
			Description: "Group by column(s) (comma-separated)",
			Schema:      &schemaProperty{Type: "string"},
		},
	}
	if cache != nil && cache.HasPostGIS && tbl != nil && tbl.HasGeometry() {
		params = append(params,
			&parameter{
				Name:        "near",
				In:          "query",
				Description: "Spatial near filter format: column,lng,lat,distance (example: location,-73.9857,40.7484,1000)",
				Schema:      &schemaProperty{Type: "string"},
			},
			&parameter{
				Name:        "within",
				In:          "query",
				Description: "Spatial within filter format: column,{geojson} (example: location,{\"type\":\"Polygon\",\"coordinates\":[[[-74,40],[-73,40],[-73,41],[-74,40]]]})",
				Schema:      &schemaProperty{Type: "string"},
			},
			&parameter{
				Name:        "intersects",
				In:          "query",
				Description: "Spatial intersects filter format: column,{geojson} (example: location,{\"type\":\"LineString\",\"coordinates\":[[-74,40],[-73,41]]})",
				Schema:      &schemaProperty{Type: "string"},
			},
			&parameter{
				Name:        "bbox",
				In:          "query",
				Description: "Spatial bounding box filter format: column,minLng,minLat,maxLng,maxLat (example: location,-74,40,-73,41)",
				Schema:      &schemaProperty{Type: "string"},
			},
		)
	}
	return params
}

func aggregateParamDescription(tbl *schema.Table, cache *schema.SchemaCache) string {
	description := "Aggregation expression (e.g. count(*), sum(amount), avg(price))"
	if cache != nil && cache.HasPostGIS && tbl != nil && tbl.HasGeometry() {
		return description + "; spatial examples: bbox(column), centroid(column)"
	}
	return description
}
