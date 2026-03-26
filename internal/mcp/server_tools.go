// Package mcp Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/mcp/server_tools.go.
package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools registers all MCP tools on the server including schema introspection (list_tables, describe_table, list_functions), record operations (query, get, create, update, delete), SQL execution, RPC function calls, and server status checks.
func registerTools(s *mcp.Server, c *apiClient) {
	// Schema tools
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tables",
		Description: "List all database tables with their columns, types, and row counts",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ListTablesInput) (*mcp.CallToolResult, ListTablesOutput, error) {
		return handleListTables(ctx, c)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "describe_table",
		Description: "Get detailed structure of a table: columns, types, primary keys, foreign keys, and indexes",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in DescribeTableInput) (*mcp.CallToolResult, DescribeTableOutput, error) {
		return handleDescribeTable(ctx, c, in)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_functions",
		Description: "List all callable PostgreSQL functions available via the RPC API",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ListFunctionsInput) (*mcp.CallToolResult, ListFunctionsOutput, error) {
		return handleListFunctions(ctx, c)
	})

	registerSpatialTools(s, c)

	// Data tools
	mcp.AddTool(s, &mcp.Tool{
		Name:        "query_records",
		Description: "List records from a table with optional filter, sort, pagination, search, and FK expansion",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in QueryRecordsInput) (*mcp.CallToolResult, QueryRecordsOutput, error) {
		return handleQueryRecords(ctx, c, in)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_record",
		Description: "Get a single record by its ID",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GetRecordInput) (*mcp.CallToolResult, RecordOutput, error) {
		return handleGetRecord(ctx, c, in)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_record",
		Description: "Insert a new record into a table",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CreateRecordInput) (*mcp.CallToolResult, RecordOutput, error) {
		return handleCreateRecord(ctx, c, in)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "update_record",
		Description: "Partially update a record by ID",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in UpdateRecordInput) (*mcp.CallToolResult, RecordOutput, error) {
		return handleUpdateRecord(ctx, c, in)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_record",
		Description: "Delete a record by ID",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in DeleteRecordInput) (*mcp.CallToolResult, DeleteRecordOutput, error) {
		return handleDeleteRecord(ctx, c, in)
	})

	// SQL tool
	mcp.AddTool(s, &mcp.Tool{
		Name:        "run_sql",
		Description: "Execute arbitrary SQL against the database (requires admin token)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in RunSQLInput) (*mcp.CallToolResult, RunSQLOutput, error) {
		return handleRunSQL(ctx, c, in)
	})

	// RPC tool
	mcp.AddTool(s, &mcp.Tool{
		Name:        "call_function",
		Description: "Call a PostgreSQL function via the RPC API with named arguments",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CallFunctionInput) (*mcp.CallToolResult, FunctionOutput, error) {
		return handleCallFunction(ctx, c, in)
	})

	// Admin tool
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_status",
		Description: "Get the AYB server health status and configuration",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GetStatusInput) (*mcp.CallToolResult, GetStatusOutput, error) {
		return handleGetStatus(ctx, c)
	})
}

// --- Tool handlers ---

// TODO: Document handleListTables.
func handleListTables(ctx context.Context, c *apiClient) (*mcp.CallToolResult, ListTablesOutput, error) {
	result, _, err := c.doJSON(ctx, "GET", "/api/schema", nil, false)
	if err != nil {
		return nil, ListTablesOutput{}, err
	}

	tablesMap, _ := result["tables"].(map[string]any)
	out := ListTablesOutput{Tables: make([]map[string]any, 0, len(tablesMap))}
	if hasPostGIS, ok := result["hasPostGIS"].(bool); ok {
		out.HasPostGIS = hasPostGIS
	}
	if postGISVersion, ok := result["postGISVersion"].(string); ok {
		out.PostGISVersion = postGISVersion
	}
	for _, t := range tablesMap {
		if tMap, ok := t.(map[string]any); ok {
			out.Tables = append(out.Tables, tMap)
		}
	}
	return nil, out, nil
}

// handleDescribeTable fetches the schema and extracts the structure of a named table, returning its columns, types, primary keys, and foreign key relationships.
func handleDescribeTable(ctx context.Context, c *apiClient, in DescribeTableInput) (*mcp.CallToolResult, DescribeTableOutput, error) {
	result, _, err := c.doJSON(ctx, "GET", "/api/schema", nil, false)
	if err != nil {
		return nil, DescribeTableOutput{}, err
	}

	tablesMap, _ := result["tables"].(map[string]any)
	for _, t := range tablesMap {
		tMap, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tMap["name"].(string)
		if name != in.Table {
			continue
		}

		out := DescribeTableOutput{Name: name}
		if cols, ok := tMap["columns"].([]any); ok {
			for _, col := range cols {
				if colMap, ok := col.(map[string]any); ok {
					out.Columns = append(out.Columns, colMap)
				}
			}
		}
		if pks, ok := tMap["primaryKey"].([]any); ok {
			for _, pk := range pks {
				if s, ok := pk.(string); ok {
					out.PKs = append(out.PKs, s)
				}
			}
		}
		if fks, ok := tMap["foreignKeys"].([]any); ok {
			for _, fk := range fks {
				if fkMap, ok := fk.(map[string]any); ok {
					out.FKs = append(out.FKs, fkMap)
				}
			}
		}
		return nil, out, nil
	}

	return nil, DescribeTableOutput{}, fmt.Errorf("table %q not found", in.Table)
}

func handleListFunctions(ctx context.Context, c *apiClient) (*mcp.CallToolResult, ListFunctionsOutput, error) {
	result, _, err := c.doJSON(ctx, "GET", "/api/schema", nil, false)
	if err != nil {
		return nil, ListFunctionsOutput{}, err
	}

	functionsMap, _ := result["functions"].(map[string]any)
	out := ListFunctionsOutput{Functions: make([]map[string]any, 0, len(functionsMap))}
	for _, f := range functionsMap {
		if fMap, ok := f.(map[string]any); ok {
			out.Functions = append(out.Functions, fMap)
		}
	}
	return nil, out, nil
}

// handleQueryRecords queries records from a table with optional filtering, sorting, pagination, full-text search, and foreign key expansion, returning paginated results with total counts.
func handleQueryRecords(ctx context.Context, c *apiClient, in QueryRecordsInput) (*mcp.CallToolResult, QueryRecordsOutput, error) {
	params := url.Values{}
	if in.Filter != "" {
		params.Set("filter", in.Filter)
	}
	if in.Sort != "" {
		params.Set("sort", in.Sort)
	}
	if in.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", in.Page))
	}
	if in.PerPage > 0 {
		params.Set("perPage", fmt.Sprintf("%d", in.PerPage))
	}
	if in.Expand != "" {
		params.Set("expand", in.Expand)
	}
	if in.Search != "" {
		params.Set("search", in.Search)
	}

	path := "/api/collections/" + url.PathEscape(in.Table)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	result, _, err := c.doJSON(ctx, "GET", path, nil, false)
	if err != nil {
		return nil, QueryRecordsOutput{}, err
	}

	out := QueryRecordsOutput{}
	if items, ok := result["items"].([]any); ok {
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				out.Items = append(out.Items, m)
			}
		}
	}
	if v, ok := result["page"].(float64); ok {
		out.Page = int(v)
	}
	if v, ok := result["perPage"].(float64); ok {
		out.PerPage = int(v)
	}
	if v, ok := result["totalItems"].(float64); ok {
		out.TotalItems = int(v)
	}
	if v, ok := result["totalPages"].(float64); ok {
		out.TotalPages = int(v)
	}
	return nil, out, nil
}

func handleGetRecord(ctx context.Context, c *apiClient, in GetRecordInput) (*mcp.CallToolResult, RecordOutput, error) {
	path := "/api/collections/" + url.PathEscape(in.Table) + "/" + url.PathEscape(in.ID)
	if in.Expand != "" {
		path += "?expand=" + url.QueryEscape(in.Expand)
	}

	result, _, err := c.doJSON(ctx, "GET", path, nil, false)
	if err != nil {
		return nil, RecordOutput{}, err
	}
	return nil, RecordOutput{Record: result}, nil
}

func handleCreateRecord(ctx context.Context, c *apiClient, in CreateRecordInput) (*mcp.CallToolResult, RecordOutput, error) {
	path := "/api/collections/" + url.PathEscape(in.Table)
	result, _, err := c.doJSON(ctx, "POST", path, in.Data, false)
	if err != nil {
		return nil, RecordOutput{}, err
	}
	return nil, RecordOutput{Record: result}, nil
}

func handleUpdateRecord(ctx context.Context, c *apiClient, in UpdateRecordInput) (*mcp.CallToolResult, RecordOutput, error) {
	path := "/api/collections/" + url.PathEscape(in.Table) + "/" + url.PathEscape(in.ID)
	result, _, err := c.doJSON(ctx, "PATCH", path, in.Data, false)
	if err != nil {
		return nil, RecordOutput{}, err
	}
	return nil, RecordOutput{Record: result}, nil
}

func handleDeleteRecord(ctx context.Context, c *apiClient, in DeleteRecordInput) (*mcp.CallToolResult, DeleteRecordOutput, error) {
	path := "/api/collections/" + url.PathEscape(in.Table) + "/" + url.PathEscape(in.ID)
	_, status, err := c.doJSON(ctx, "DELETE", path, nil, false)
	if err != nil {
		return nil, DeleteRecordOutput{}, err
	}
	return nil, DeleteRecordOutput{Deleted: status == http.StatusNoContent}, nil
}

// handleRunSQL executes an arbitrary SQL query using the admin API, returning column names, result rows, row count, and execution time in milliseconds.
func handleRunSQL(ctx context.Context, c *apiClient, in RunSQLInput) (*mcp.CallToolResult, RunSQLOutput, error) {
	result, _, err := c.doJSON(ctx, "POST", "/api/admin/sql", map[string]string{"query": in.Query}, true)
	if err != nil {
		return nil, RunSQLOutput{}, err
	}

	out := RunSQLOutput{}
	if cols, ok := result["columns"].([]any); ok {
		for _, col := range cols {
			if s, ok := col.(string); ok {
				out.Columns = append(out.Columns, s)
			}
		}
	}
	if rows, ok := result["rows"].([]any); ok {
		for _, row := range rows {
			if rowSlice, ok := row.([]any); ok {
				out.Rows = append(out.Rows, rowSlice)
			}
		}
		out.RowCount = len(out.Rows)
	}
	if v, ok := result["durationMs"].(float64); ok {
		out.DurationMs = v
	}
	if v, ok := result["rowCount"].(float64); ok {
		out.RowCount = int(v)
	}
	return nil, out, nil
}

func handleCallFunction(ctx context.Context, c *apiClient, in CallFunctionInput) (*mcp.CallToolResult, FunctionOutput, error) {
	path := "/api/rpc/" + url.PathEscape(in.Function)
	result, status, err := c.doJSON(ctx, "POST", path, in.Args, false)
	if err != nil {
		return nil, FunctionOutput{}, err
	}
	if status == http.StatusNoContent {
		return nil, FunctionOutput{Result: nil}, nil
	}
	return nil, FunctionOutput{Result: result}, nil
}

func handleGetStatus(ctx context.Context, c *apiClient) (*mcp.CallToolResult, GetStatusOutput, error) {
	health, _, err := c.doJSON(ctx, "GET", "/health", nil, false)
	if err != nil {
		return nil, GetStatusOutput{Status: "unreachable"}, nil
	}

	out := GetStatusOutput{}
	if s, ok := health["status"].(string); ok {
		out.Status = s
	}

	admin, _, _ := c.doJSON(ctx, "GET", "/api/admin/status", nil, false)
	out.Admin = admin
	return nil, out, nil
}
