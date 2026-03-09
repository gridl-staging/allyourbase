// Package mcp implements a Model Context Protocol server for AYB.
// It exposes AYB's REST API as MCP tools, resources, and prompts,
// allowing AI coding tools (Claude Code, Cursor, Windsurf) to interact
// with the database through structured tool calls.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config holds the connection parameters for the MCP server.
type Config struct {
	// BaseURL is the AYB server URL (e.g., "http://localhost:8090").
	BaseURL string
	// AdminToken is the admin bearer token for privileged operations.
	AdminToken string
	// UserToken is a user JWT for RLS-filtered data access.
	UserToken string
}

// apiClient wraps HTTP calls to the AYB REST API.
type apiClient struct {
	baseURL    string
	adminToken string
	userToken  string
	http       *http.Client
}

func newClient(cfg Config) *apiClient {
	return &apiClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		adminToken: cfg.AdminToken,
		userToken:  cfg.UserToken,
		http:       &http.Client{},
	}
}

// doJSON makes an HTTP request and returns the parsed JSON response.
func (c *apiClient) doJSON(ctx context.Context, method, path string, body any, admin bool) (map[string]any, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	token := c.userToken
	if admin {
		token = c.adminToken
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}

	if len(respBody) == 0 {
		return nil, resp.StatusCode, nil
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Return raw text for non-JSON responses
		return map[string]any{"raw": string(respBody)}, resp.StatusCode, nil
	}

	if resp.StatusCode >= 400 {
		msg := "unknown error"
		if m, ok := result["message"].(string); ok {
			msg = m
		}
		return result, resp.StatusCode, fmt.Errorf("AYB error (%d): %s", resp.StatusCode, msg)
	}

	return result, resp.StatusCode, nil
}

// NewServer creates a new MCP server wired to an AYB instance.
func NewServer(cfg Config) *mcp.Server {
	client := newClient(cfg)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ayb-mcp",
		Title:   "Allyourbase MCP Server",
		Version: "v0.1.0",
	}, &mcp.ServerOptions{
		Instructions: "AYB MCP server — interact with your PostgreSQL database via Allyourbase. " +
			"Use tools to query data, manage schema, run SQL, and more.",
	})

	registerTools(server, client)
	registerResources(server, client)
	registerPrompts(server)

	return server
}

// --- Input/Output types for tools ---

type ListTablesInput struct{}
type ListTablesOutput struct {
	Tables []map[string]any `json:"tables"`
}

type DescribeTableInput struct {
	Table string `json:"table" jsonschema:"Table name to describe"`
}
type DescribeTableOutput struct {
	Name    string           `json:"name"`
	Columns []map[string]any `json:"columns"`
	PKs     []string         `json:"primary_keys"`
	FKs     []map[string]any `json:"foreign_keys"`
}

type ListFunctionsInput struct{}
type ListFunctionsOutput struct {
	Functions []map[string]any `json:"functions"`
}

type QueryRecordsInput struct {
	Table   string `json:"table" jsonschema:"Table name"`
	Filter  string `json:"filter,omitempty" jsonschema:"Filter expression (e.g. status='active' AND age>21)"`
	Sort    string `json:"sort,omitempty" jsonschema:"Sort fields (e.g. -created_at,+title)"`
	Page    int    `json:"page,omitempty" jsonschema:"Page number (default 1)"`
	PerPage int    `json:"perPage,omitempty" jsonschema:"Items per page (default 20, max 500)"`
	Expand  string `json:"expand,omitempty" jsonschema:"FK relationships to expand"`
	Search  string `json:"search,omitempty" jsonschema:"Full-text search query"`
}
type QueryRecordsOutput struct {
	Items      []map[string]any `json:"items"`
	Page       int              `json:"page"`
	PerPage    int              `json:"perPage"`
	TotalItems int              `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
}

type GetRecordInput struct {
	Table  string `json:"table" jsonschema:"Table name"`
	ID     string `json:"id" jsonschema:"Record ID"`
	Expand string `json:"expand,omitempty" jsonschema:"FK relationships to expand"`
}

type CreateRecordInput struct {
	Table string         `json:"table" jsonschema:"Table name"`
	Data  map[string]any `json:"data" jsonschema:"Record data as key-value pairs"`
}

type UpdateRecordInput struct {
	Table string         `json:"table" jsonschema:"Table name"`
	ID    string         `json:"id" jsonschema:"Record ID"`
	Data  map[string]any `json:"data" jsonschema:"Fields to update as key-value pairs"`
}

type DeleteRecordInput struct {
	Table string `json:"table" jsonschema:"Table name"`
	ID    string `json:"id" jsonschema:"Record ID"`
}
type DeleteRecordOutput struct {
	Deleted bool `json:"deleted"`
}

type RunSQLInput struct {
	Query string `json:"query" jsonschema:"SQL query to execute"`
}
type RunSQLOutput struct {
	Columns    []string `json:"columns"`
	Rows       [][]any  `json:"rows"`
	RowCount   int      `json:"rowCount"`
	DurationMs float64  `json:"durationMs"`
}

type CallFunctionInput struct {
	Function string         `json:"function" jsonschema:"PostgreSQL function name"`
	Args     map[string]any `json:"args,omitempty" jsonschema:"Named arguments"`
}

type GetStatusInput struct{}
type GetStatusOutput struct {
	Status string         `json:"status"`
	Admin  map[string]any `json:"admin,omitempty"`
}

type RecordOutput struct {
	Record map[string]any `json:"record"`
}

type FunctionOutput struct {
	Result any `json:"result"`
}

// --- Tool registration ---

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

func handleListTables(ctx context.Context, c *apiClient) (*mcp.CallToolResult, ListTablesOutput, error) {
	result, _, err := c.doJSON(ctx, "GET", "/api/schema", nil, false)
	if err != nil {
		return nil, ListTablesOutput{}, err
	}

	tables, _ := result["tables"].([]any)
	out := ListTablesOutput{Tables: make([]map[string]any, 0, len(tables))}
	for _, t := range tables {
		if tMap, ok := t.(map[string]any); ok {
			out.Tables = append(out.Tables, tMap)
		}
	}
	return nil, out, nil
}

func handleDescribeTable(ctx context.Context, c *apiClient, in DescribeTableInput) (*mcp.CallToolResult, DescribeTableOutput, error) {
	result, _, err := c.doJSON(ctx, "GET", "/api/schema", nil, false)
	if err != nil {
		return nil, DescribeTableOutput{}, err
	}

	tables, _ := result["tables"].([]any)
	for _, t := range tables {
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
		if pks, ok := tMap["primary_keys"].([]any); ok {
			for _, pk := range pks {
				if s, ok := pk.(string); ok {
					out.PKs = append(out.PKs, s)
				}
			}
		}
		if fks, ok := tMap["foreign_keys"].([]any); ok {
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

	functions, _ := result["functions"].([]any)
	out := ListFunctionsOutput{Functions: make([]map[string]any, 0, len(functions))}
	for _, f := range functions {
		if fMap, ok := f.(map[string]any); ok {
			out.Functions = append(out.Functions, fMap)
		}
	}
	return nil, out, nil
}

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

// --- Resource registration ---

func registerResources(s *mcp.Server, c *apiClient) {
	s.AddResource(&mcp.Resource{
		URI:         "ayb://schema",
		Name:        "Database Schema",
		Description: "Complete database schema including tables, columns, types, primary keys, foreign keys, and functions",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		result, _, err := c.doJSON(ctx, "GET", "/api/schema", nil, false)
		if err != nil {
			return nil, err
		}
		b, _ := json.MarshalIndent(result, "", "  ")
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      "ayb://schema",
				Text:     string(b),
				MIMEType: "application/json",
			}},
		}, nil
	})

	s.AddResource(&mcp.Resource{
		URI:         "ayb://health",
		Name:        "Server Health",
		Description: "AYB server health status",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		result, _, err := c.doJSON(ctx, "GET", "/health", nil, false)
		if err != nil {
			return nil, err
		}
		b, _ := json.MarshalIndent(result, "", "  ")
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      "ayb://health",
				Text:     string(b),
				MIMEType: "application/json",
			}},
		}, nil
	})
}

// --- Prompt registration ---

func registerPrompts(s *mcp.Server) {
	s.AddPrompt(&mcp.Prompt{
		Name:        "explore-table",
		Description: "Explore the structure and sample data of a database table",
		Arguments: []*mcp.PromptArgument{
			{Name: "table", Description: "Table name to explore", Required: true},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		table := req.Params.Arguments["table"]
		return &mcp.GetPromptResult{
			Description: "Explore table: " + table,
			Messages: []*mcp.PromptMessage{{
				Role: "user",
				Content: &mcp.TextContent{
					Text: fmt.Sprintf(
						"Describe the structure of the %q table. First use describe_table to get the schema, "+
							"then use query_records to show 5 sample rows. Summarize the table's purpose, "+
							"column types, relationships, and any notable patterns in the data.", table),
				},
			}},
		}, nil
	})

	s.AddPrompt(&mcp.Prompt{
		Name:        "write-migration",
		Description: "Generate a SQL migration for a schema change",
		Arguments: []*mcp.PromptArgument{
			{Name: "description", Description: "What the migration should do", Required: true},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		desc := req.Params.Arguments["description"]
		return &mcp.GetPromptResult{
			Description: "Write migration: " + desc,
			Messages: []*mcp.PromptMessage{{
				Role: "user",
				Content: &mcp.TextContent{
					Text: fmt.Sprintf(
						"I need a SQL migration to: %s\n\n"+
							"First use list_tables to understand the current schema. "+
							"Then write a safe, idempotent PostgreSQL migration with both UP and DOWN sections. "+
							"Use IF EXISTS/IF NOT EXISTS guards. Include comments explaining each change.", desc),
				},
			}},
		}, nil
	})

	s.AddPrompt(&mcp.Prompt{
		Name:        "generate-types",
		Description: "Generate TypeScript type definitions for the database schema",
		Arguments:   []*mcp.PromptArgument{},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Generate TypeScript types from schema",
			Messages: []*mcp.PromptMessage{{
				Role: "user",
				Content: &mcp.TextContent{
					Text: "Use list_tables to get the full database schema, then generate TypeScript " +
						"interfaces for each table. Include Create and Update variants (with optional fields). " +
						"Map PostgreSQL types accurately: TEXT→string, INTEGER→number, BOOLEAN→boolean, " +
						"TIMESTAMPTZ→string, UUID→string, JSONB→Record<string, unknown>, arrays→Type[].",
				},
			}},
		}, nil
	})
}
