package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeAYB sets up a test HTTP server that mimics the AYB REST API.
func fakeAYB(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/health":
			json.NewEncoder(w).Encode(map[string]any{"status": "ok"})

		case r.URL.Path == "/api/admin/status":
			json.NewEncoder(w).Encode(map[string]any{"auth": true})

		case r.URL.Path == "/api/schema":
			json.NewEncoder(w).Encode(map[string]any{
				"tables": []any{
					map[string]any{
						"name": "posts",
						"columns": []any{
							map[string]any{"name": "id", "type": "integer", "nullable": false},
							map[string]any{"name": "title", "type": "text", "nullable": false},
							map[string]any{"name": "body", "type": "text", "nullable": true},
							map[string]any{"name": "published", "type": "boolean", "nullable": false},
							map[string]any{"name": "author_id", "type": "integer", "nullable": true},
						},
						"primary_keys": []any{"id"},
						"foreign_keys": []any{
							map[string]any{"column": "author_id", "references_table": "authors", "references_column": "id"},
						},
					},
					map[string]any{
						"name": "authors",
						"columns": []any{
							map[string]any{"name": "id", "type": "integer", "nullable": false},
							map[string]any{"name": "name", "type": "text", "nullable": false},
						},
						"primary_keys": []any{"id"},
					},
				},
				"functions": []any{
					map[string]any{"name": "get_post_count", "return_type": "integer"},
				},
			})

		case r.URL.Path == "/api/collections/posts" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]any{
				"items": []any{
					map[string]any{"id": 1, "title": "Hello", "published": true},
					map[string]any{"id": 2, "title": "Draft", "published": false},
				},
				"page":       1,
				"perPage":    20,
				"totalItems": 2,
				"totalPages": 1,
			})

		case r.URL.Path == "/api/collections/posts/1" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]any{"id": 1, "title": "Hello", "published": true})

		case r.URL.Path == "/api/collections/posts" && r.Method == "POST":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			body["id"] = 3
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(body)

		case r.URL.Path == "/api/collections/posts/1" && r.Method == "PATCH":
			json.NewEncoder(w).Encode(map[string]any{"id": 1, "title": "Updated", "published": true})

		case r.URL.Path == "/api/collections/posts/1" && r.Method == "DELETE":
			w.WriteHeader(http.StatusNoContent)

		case r.URL.Path == "/api/admin/sql" && r.Method == "POST":
			if r.Header.Get("Authorization") != "Bearer test-admin-token" {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]any{"code": 401, "message": "unauthorized"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"columns":    []any{"count"},
				"rows":       []any{[]any{42}},
				"rowCount":   1,
				"durationMs": 1.5,
			})

		case r.URL.Path == "/api/rpc/get_post_count" && r.Method == "POST":
			json.NewEncoder(w).Encode(map[string]any{"result": 42})

		case r.URL.Path == "/api/collections/nonexistent" && r.Method == "GET":
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"code": 404, "message": "collection not found"})

		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"code": 404, "message": "not found"})
		}
	}))
}

func TestNewServer(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()

	srv := NewServer(Config{BaseURL: ts.URL, AdminToken: "test-admin-token"})
	testutil.NotNil(t, srv)
}

func TestListTables(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()

	c := newClient(Config{BaseURL: ts.URL})
	_, out, err := handleListTables(context.Background(), c)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(out.Tables))

	name0, _ := out.Tables[0]["name"].(string)
	testutil.Equal(t, "posts", name0)
}

func TestDescribeTable(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	t.Cleanup(ts.Close)
	c := newClient(Config{BaseURL: ts.URL})

	t.Run("existing table", func(t *testing.T) {
		t.Parallel()
		_, out, err := handleDescribeTable(context.Background(), c, DescribeTableInput{Table: "posts"})
		testutil.NoError(t, err)
		testutil.Equal(t, "posts", out.Name)
		testutil.Equal(t, 5, len(out.Columns))
		testutil.Equal(t, 1, len(out.PKs))
		testutil.Equal(t, "id", out.PKs[0])
		testutil.Equal(t, 1, len(out.FKs))
	})

	t.Run("nonexistent table", func(t *testing.T) {
		t.Parallel()
		_, _, err := handleDescribeTable(context.Background(), c, DescribeTableInput{Table: "missing"})
		testutil.ErrorContains(t, err, "not found")
	})
}

func TestListFunctions(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, out, err := handleListFunctions(context.Background(), c)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(out.Functions))
	name, _ := out.Functions[0]["name"].(string)
	testutil.Equal(t, "get_post_count", name)
}

func TestQueryRecords(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, out, err := handleQueryRecords(context.Background(), c, QueryRecordsInput{Table: "posts"})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(out.Items))
	testutil.Equal(t, 1, out.Page)
	testutil.Equal(t, 2, out.TotalItems)
}

func TestGetRecord(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, out, err := handleGetRecord(context.Background(), c, GetRecordInput{Table: "posts", ID: "1"})
	testutil.NoError(t, err)
	testutil.Equal(t, "Hello", out.Record["title"])
}

func TestCreateRecord(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, out, err := handleCreateRecord(context.Background(), c, CreateRecordInput{
		Table: "posts",
		Data:  map[string]any{"title": "New Post"},
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "New Post", out.Record["title"])
	id, _ := out.Record["id"].(float64)
	testutil.Equal(t, float64(3), id)
}

func TestUpdateRecord(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, out, err := handleUpdateRecord(context.Background(), c, UpdateRecordInput{
		Table: "posts", ID: "1",
		Data: map[string]any{"title": "Updated"},
	})
	testutil.NoError(t, err)
	testutil.Equal(t, "Updated", out.Record["title"])
}

func TestDeleteRecord(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, out, err := handleDeleteRecord(context.Background(), c, DeleteRecordInput{Table: "posts", ID: "1"})
	testutil.NoError(t, err)
	testutil.True(t, out.Deleted)
}

func TestRunSQL(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	t.Cleanup(ts.Close)

	t.Run("with admin token", func(t *testing.T) {
		t.Parallel()
		c := newClient(Config{BaseURL: ts.URL, AdminToken: "test-admin-token"})
		_, out, err := handleRunSQL(context.Background(), c, RunSQLInput{Query: "SELECT count(*) FROM posts"})
		testutil.NoError(t, err)
		testutil.Equal(t, 1, len(out.Columns))
		testutil.Equal(t, "count", out.Columns[0])
		testutil.Equal(t, 1, out.RowCount)
	})

	t.Run("without admin token", func(t *testing.T) {
		t.Parallel()
		c := newClient(Config{BaseURL: ts.URL, AdminToken: ""})
		_, _, err := handleRunSQL(context.Background(), c, RunSQLInput{Query: "SELECT 1"})
		testutil.ErrorContains(t, err, "401")
	})
}

func TestCallFunction(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, out, err := handleCallFunction(context.Background(), c, CallFunctionInput{
		Function: "get_post_count",
	})
	testutil.NoError(t, err)
	testutil.NotNil(t, out.Result)
	// Verify the result contains actual data from the fake server
	resultMap, ok := out.Result.(map[string]any)
	testutil.True(t, ok, "expected result to be a map")
	val, ok := resultMap["result"].(float64)
	testutil.True(t, ok, "expected result value to be float64")
	testutil.Equal(t, float64(42), val)
}

func TestCallFunction_VoidReturn(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, out, err := handleCallFunction(context.Background(), c, CallFunctionInput{
		Function: "reset_counters",
	})
	testutil.NoError(t, err)
	testutil.Nil(t, out.Result)
}

func TestGetStatus(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, out, err := handleGetStatus(context.Background(), c)
	testutil.NoError(t, err)
	testutil.Equal(t, "ok", out.Status)
	testutil.NotNil(t, out.Admin)
}

func TestQueryRecords_WithParams(t *testing.T) {
	t.Parallel()
	var capturedURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"items":      []any{map[string]any{"id": 1, "title": "Hello"}},
			"page":       1,
			"perPage":    10,
			"totalItems": 1,
			"totalPages": 1,
		})
	}))
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, out, err := handleQueryRecords(context.Background(), c, QueryRecordsInput{
		Table:   "posts",
		Filter:  "published=true",
		Sort:    "-created_at",
		Page:    1,
		PerPage: 10,
		Expand:  "author_id",
		Search:  "hello",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(out.Items))

	// Verify all query parameters were actually forwarded
	testutil.Contains(t, capturedURL, "filter=published%3Dtrue")
	testutil.Contains(t, capturedURL, "sort=-created_at")
	testutil.Contains(t, capturedURL, "page=1")
	testutil.Contains(t, capturedURL, "perPage=10")
	testutil.Contains(t, capturedURL, "expand=author_id")
	testutil.Contains(t, capturedURL, "search=hello")
}

func TestQueryRecords_NotFound(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, _, err := handleQueryRecords(context.Background(), c, QueryRecordsInput{Table: "nonexistent"})
	testutil.ErrorContains(t, err, "404")
}

func TestResourceSchema(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	// Test the schema resource handler directly
	result, _, err := c.doJSON(context.Background(), "GET", "/api/schema", nil, false)
	testutil.NoError(t, err)
	testutil.NotNil(t, result["tables"])
}

func TestResourceHealth(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	result, _, err := c.doJSON(context.Background(), "GET", "/health", nil, false)
	testutil.NoError(t, err)
	testutil.Equal(t, "ok", result["status"])
}

func TestServerHasToolsRegistered(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()

	srv := NewServer(Config{BaseURL: ts.URL, AdminToken: "test-admin-token"})

	// Use in-memory transport to list tools
	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Connect(ctx, serverTransport, nil)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "v0.0.1",
	}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	testutil.NoError(t, err)

	tools, err := session.ListTools(ctx, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 11, len(tools.Tools))

	// Verify specific tool names exist
	toolNames := make(map[string]bool)
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	testutil.True(t, toolNames["list_tables"])
	testutil.True(t, toolNames["describe_table"])
	testutil.True(t, toolNames["query_records"])
	testutil.True(t, toolNames["get_record"])
	testutil.True(t, toolNames["create_record"])
	testutil.True(t, toolNames["update_record"])
	testutil.True(t, toolNames["delete_record"])
	testutil.True(t, toolNames["run_sql"])
	testutil.True(t, toolNames["call_function"])
	testutil.True(t, toolNames["get_status"])
	testutil.True(t, toolNames["list_functions"])
}

func TestServerHasResourcesRegistered(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()

	srv := NewServer(Config{BaseURL: ts.URL})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Connect(ctx, serverTransport, nil)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "v0.0.1",
	}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	testutil.NoError(t, err)

	resources, err := session.ListResources(ctx, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(resources.Resources))

	resourceURIs := make(map[string]bool)
	for _, r := range resources.Resources {
		resourceURIs[r.URI] = true
	}
	testutil.True(t, resourceURIs["ayb://schema"])
	testutil.True(t, resourceURIs["ayb://health"])
}

func TestServerHasPromptsRegistered(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()

	srv := NewServer(Config{BaseURL: ts.URL})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.Connect(ctx, serverTransport, nil)

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "v0.0.1",
	}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	testutil.NoError(t, err)

	prompts, err := session.ListPrompts(ctx, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, len(prompts.Prompts))

	promptNames := make(map[string]bool)
	for _, p := range prompts.Prompts {
		promptNames[p.Name] = true
	}
	testutil.True(t, promptNames["explore-table"])
	testutil.True(t, promptNames["write-migration"])
	testutil.True(t, promptNames["generate-types"])
}

func TestAPIClientAuthHeaders(t *testing.T) {
	t.Parallel()
	var gotAdminHeader, gotUserHeader string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin":
			gotAdminHeader = r.Header.Get("Authorization")
		case "/user":
			gotUserHeader = r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer ts.Close()

	c := newClient(Config{
		BaseURL:    ts.URL,
		AdminToken: "admin-tok",
		UserToken:  "user-tok",
	})

	c.doJSON(context.Background(), "GET", "/admin", nil, true)
	testutil.Equal(t, "Bearer admin-tok", gotAdminHeader)

	c.doJSON(context.Background(), "GET", "/user", nil, false)
	testutil.Equal(t, "Bearer user-tok", gotUserHeader)
}

func TestAPIClientErrorHandling(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"code": 400, "message": "bad filter syntax"})
	}))
	defer ts.Close()

	c := newClient(Config{BaseURL: ts.URL})
	_, _, err := c.doJSON(context.Background(), "GET", "/api/collections/posts?filter=bad", nil, false)
	testutil.ErrorContains(t, err, "bad filter syntax")
}

func TestGetStatus_Unreachable(t *testing.T) {
	// Health endpoint unreachable → status should be "unreachable", no error
	t.Parallel()

	c := newClient(Config{BaseURL: "http://127.0.0.1:1"})
	_, out, err := handleGetStatus(context.Background(), c)
	testutil.NoError(t, err)
	testutil.Equal(t, "unreachable", out.Status)
}

func TestDeleteRecord_NotFound(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	// Delete a record that doesn't exist (fakeAYB returns 404 for unknown paths)
	_, _, err := handleDeleteRecord(context.Background(), c, DeleteRecordInput{Table: "posts", ID: "999"})
	testutil.ErrorContains(t, err, "404")
}

func TestGetRecord_NotFound(t *testing.T) {
	t.Parallel()
	ts := fakeAYB(t)
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, _, err := handleGetRecord(context.Background(), c, GetRecordInput{Table: "posts", ID: "999"})
	testutil.ErrorContains(t, err, "404")
}

func TestAPIClientConnectionError(t *testing.T) {
	t.Parallel()
	c := newClient(Config{BaseURL: "http://127.0.0.1:1"})
	_, _, err := c.doJSON(context.Background(), "GET", "/health", nil, false)
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "request failed")
}

func TestAPIClientNonJSONResponse(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("plain text response"))
	}))
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	result, status, err := c.doJSON(context.Background(), "GET", "/test", nil, false)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, status)
	testutil.Equal(t, "plain text response", result["raw"])
}

func TestAPIClientEmptyResponse(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	c := newClient(Config{BaseURL: ts.URL})

	_, status, err := c.doJSON(context.Background(), "GET", "/test", nil, false)
	testutil.NoError(t, err)
	testutil.Equal(t, 204, status)
}
