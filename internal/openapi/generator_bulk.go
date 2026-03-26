// Package openapi Builds OpenAPI operations for bulk data operations including aggregation, imports, exports, and batch modifications.
package openapi

import "github.com/allyourbase/ayb/internal/schema"

// buildAggregatePath creates the GET /{table}/aggregate operation.
func buildAggregatePath(tbl *schema.Table, cache *schema.SchemaCache) *pathItem {
	return &pathItem{
		Get: buildAggregateOp(tbl, cache),
	}
}

// buildAggregateOp returns the operation for aggregating table rows.
func buildAggregateOp(tbl *schema.Table, cache *schema.SchemaCache) *operation {
	return &operation{
		Summary:     "Aggregate " + tbl.Name + " rows",
		Tags:        []string{tbl.Name},
		OperationID: "aggregate_" + tbl.Name,
		Parameters: []*parameter{
			{
				Name:        "aggregate",
				In:          "query",
				Description: aggregateParamDescription(tbl, cache),
				Required:    true,
				Schema:      &schemaProperty{Type: "string"},
			},
			{
				Name:        "group",
				In:          "query",
				Description: "Group by column(s) (comma-separated)",
				Schema:      &schemaProperty{Type: "string"},
			},
			{
				Name:        "filter",
				In:          "query",
				Description: "Filter expression applied before aggregation",
				Schema:      &schemaProperty{Type: "string"},
			},
			{
				Name:        "search",
				In:          "query",
				Description: "Full-text search term across text columns",
				Schema:      &schemaProperty{Type: "string"},
			},
		},
		Responses: map[string]*response{
			"200": {
				Description: "Successful aggregation response",
				Content: map[string]*mediaContent{
					"application/json": {
						Schema: &schemaProperty{
							Type: "object",
							Properties: map[string]*schemaProperty{
								"results": {
									Type:        "array",
									Items:       &schemaProperty{Type: "object"},
									Description: "Aggregation result rows with group columns and computed values",
								},
							},
						},
					},
				},
			},
		},
	}
}

// buildImportPath creates the POST /{table}/import operation.
func buildImportPath(tbl *schema.Table, writeSchema *schemaProperty) *pathItem {
	return &pathItem{
		Post: buildImportOp(tbl, writeSchema),
	}
}

// buildImportOp returns the operation for importing data into a table.
func buildImportOp(tbl *schema.Table, writeSchema *schemaProperty) *operation {
	return &operation{
		Summary:     "Import data into " + tbl.Name,
		Tags:        []string{tbl.Name},
		OperationID: "import_" + tbl.Name,
		Parameters: []*parameter{
			{
				Name:        "mode",
				In:          "query",
				Description: "Import mode: 'full' (default, all-or-nothing) or 'partial' (skip bad rows)",
				Schema:      &schemaProperty{Type: "string"},
			},
			{
				Name:        "on_conflict",
				In:          "query",
				Description: "Conflict resolution: 'skip' (DO NOTHING) or 'update' (DO UPDATE). Omit for error on conflict.",
				Schema:      &schemaProperty{Type: "string"},
			},
		},
		RequestBody: &requestBody{
			Required: true,
			Content: map[string]*mediaContent{
				"text/csv": {
					Schema: &schemaProperty{Type: "string", Description: "CSV data with header row"},
				},
				"application/json": {
					Schema: &schemaProperty{
						Type:  "array",
						Items: writeSchema,
					},
				},
			},
		},
		Responses: map[string]*response{
			"200": {
				Description: "Import completed",
				Content: map[string]*mediaContent{
					"application/json": {
						Schema: importResponseSchema(),
					},
				},
			},
		},
	}
}

// importResponseSchema returns the response schema matching api.ImportResponse.
func importResponseSchema() *schemaProperty {
	return &schemaProperty{
		Type: "object",
		Properties: map[string]*schemaProperty{
			"processed": {Type: "integer", Description: "Total rows processed (including failures)"},
			"inserted":  {Type: "integer", Description: "Number of rows inserted"},
			"updated":   {Type: "integer", Description: "Number of rows updated (on_conflict=update)"},
			"skipped":   {Type: "integer", Description: "Number of rows skipped (on_conflict=skip)"},
			"failed":    {Type: "integer", Description: "Number of rows that failed"},
			"errors": {
				Type: "array",
				Items: &schemaProperty{
					Type: "object",
					Properties: map[string]*schemaProperty{
						"row":     {Type: "integer", Description: "Row number (1-indexed)"},
						"message": {Type: "string", Description: "Error description"},
					},
				},
			},
		},
	}
}

// exportQueryParams returns the shared query parameters for export endpoints.
func exportQueryParams() []*parameter {
	return []*parameter{
		{
			Name:        "filter",
			In:          "query",
			Description: "Filter expression to select rows for export",
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "sort",
			In:          "query",
			Description: "Sort expression for export order",
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "search",
			In:          "query",
			Description: "Full-text search term across text columns",
			Schema:      &schemaProperty{Type: "string"},
		},
		{
			Name:        "fields",
			In:          "query",
			Description: "Comma-separated list of columns to export",
			Schema:      &schemaProperty{Type: "string"},
		},
	}
}

// buildExportCSVPath creates the GET /{table}/export.csv operation.
func buildExportCSVPath(tbl *schema.Table) *pathItem {
	return &pathItem{
		Get: &operation{
			Summary:     "Export " + tbl.Name + " as CSV",
			Tags:        []string{tbl.Name},
			OperationID: "export_csv_" + tbl.Name,
			Parameters:  exportQueryParams(),
			Responses: map[string]*response{
				"200": {
					Description: "CSV export response",
					Content: map[string]*mediaContent{
						"text/csv": {
							Schema: &schemaProperty{Type: "string", Description: "CSV data"},
						},
					},
				},
			},
		},
	}
}

// buildExportJSONPath creates the GET /{table}/export.json operation.
func buildExportJSONPath(tbl *schema.Table, rowSchema *schemaProperty) *pathItem {
	return &pathItem{
		Get: &operation{
			Summary:     "Export " + tbl.Name + " as JSON",
			Tags:        []string{tbl.Name},
			OperationID: "export_json_" + tbl.Name,
			Parameters:  exportQueryParams(),
			Responses: map[string]*response{
				"200": {
					Description: "JSON export response",
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
		},
	}
}

// buildBatchPath creates the POST /{table}/batch operation.
func buildBatchPath(tbl *schema.Table, writeSchema *schemaProperty) *pathItem {
	return &pathItem{
		Post: buildBatchOp(tbl, writeSchema),
	}
}

// buildBatchOp returns the operation for batch operations on a table.
func buildBatchOp(tbl *schema.Table, writeSchema *schemaProperty) *operation {
	return &operation{
		Summary:     "Batch operations on " + tbl.Name,
		Tags:        []string{tbl.Name},
		OperationID: "batch_" + tbl.Name,
		RequestBody: &requestBody{
			Required: true,
			Content: map[string]*mediaContent{
				"application/json": {
					Schema: &schemaProperty{
						Type: "object",
						Properties: map[string]*schemaProperty{
							"operations": {
								Type: "array",
								Items: &schemaProperty{
									Type: "object",
									Properties: map[string]*schemaProperty{
										"method": {Type: "string", Description: "Operation: create, update, or delete"},
										"id":     {Type: "string", Description: "Record ID (required for update/delete)"},
										"body":   writeSchema,
									},
								},
							},
						},
					},
				},
			},
		},
		Responses: map[string]*response{
			"200": {
				Description: "Batch operation results",
				Content: map[string]*mediaContent{
					"application/json": {
						Schema: &schemaProperty{
							Type: "array",
							Items: &schemaProperty{
								Type: "object",
								Properties: map[string]*schemaProperty{
									"index":  {Type: "integer", Description: "Operation index in the request array"},
									"status": {Type: "integer", Description: "HTTP status code for this operation"},
									"body":   {Type: "object", Description: "Result body (present on success)"},
								},
							},
						},
					},
				},
			},
		},
	}
}
