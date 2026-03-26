// Package mcp implements a Model Context Protocol server that exposes AYB's PostgreSQL database through MCP tools, resources, and prompts for AI coding assistants.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
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
	Tables         []map[string]any `json:"tables"`
	HasPostGIS     bool             `json:"hasPostGIS"`
	PostGISVersion string           `json:"postGISVersion,omitempty"`
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

type SpatialInfoInput struct{}

type SpatialColumnInfo = schema.SpatialInfoColumn
type SpatialTableInfo = schema.SpatialInfoTable
type SpatialInfoOutput = schema.SpatialInfoSummary

// TODO: Document SpatialQueryInput.
type SpatialQueryInput struct {
	Table      string   `json:"table" jsonschema:"Table name"`
	Column     string   `json:"column" jsonschema:"Spatial column name"`
	FilterType string   `json:"filter_type" jsonschema:"Spatial filter type: near, within, intersects, or bbox"`
	Longitude  *float64 `json:"longitude,omitempty" jsonschema:"Longitude for near filter"`
	Latitude   *float64 `json:"latitude,omitempty" jsonschema:"Latitude for near filter"`
	Distance   *float64 `json:"distance,omitempty" jsonschema:"Distance in meters for near filter"`
	GeoJSON    string   `json:"geojson,omitempty" jsonschema:"GeoJSON geometry for within/intersects filters"`
	MinLng     *float64 `json:"min_lng,omitempty" jsonschema:"Minimum longitude for bbox filter"`
	MinLat     *float64 `json:"min_lat,omitempty" jsonschema:"Minimum latitude for bbox filter"`
	MaxLng     *float64 `json:"max_lng,omitempty" jsonschema:"Maximum longitude for bbox filter"`
	MaxLat     *float64 `json:"max_lat,omitempty" jsonschema:"Maximum latitude for bbox filter"`
	Filter     string   `json:"filter,omitempty" jsonschema:"Optional non-spatial filter expression"`
	Sort       string   `json:"sort,omitempty" jsonschema:"Optional sort expression"`
	Limit      *int     `json:"limit,omitempty" jsonschema:"Optional row limit"`
	Offset     *int     `json:"offset,omitempty" jsonschema:"Optional row offset"`
}

type SpatialQueryOutput struct {
	Items      []map[string]any `json:"items"`
	Page       int              `json:"page"`
	PerPage    int              `json:"perPage"`
	TotalItems int              `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
}

// --- Resource registration ---

// registerResources registers the database schema and server health status as readable MCP resources that can be accessed by the client.
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

// registerPrompts registers interactive MCP prompts that guide Claude in exploring table structures, generating SQL migrations, and creating TypeScript type definitions from the schema.
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
