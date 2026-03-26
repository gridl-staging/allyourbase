package openapi

import (
	"encoding/json"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
)

func TestGenerate_BasePathPrefix(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "posts", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", TypeName: "text"},
		}, PrimaryKey: []string{"id"}},
	}, map[string]*schema.Function{
		"public.add_numbers": {
			Schema:     "public",
			Name:       "add_numbers",
			ReturnType: "integer",
			ReturnsSet: false,
			Parameters: []*schema.FuncParam{
				{Name: "a", Type: "integer", Position: 1},
				{Name: "b", Type: "integer", Position: 2},
			},
		},
	})

	data, err := Generate(cache, Options{BasePath: "/api"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)
	paths := doc["paths"].(map[string]any)

	expectedPaths := []string{
		"/api/collections/posts",
		"/api/collections/posts/{id}",
		"/api/collections/posts/aggregate",
		"/api/collections/posts/import",
		"/api/collections/posts/export.csv",
		"/api/collections/posts/export.json",
		"/api/collections/posts/batch",
		"/api/rpc/add_numbers",
	}
	for _, p := range expectedPaths {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %q with BasePath", p)
		}
	}
	if _, ok := paths["/api/collections/rpc/add_numbers"]; ok {
		t.Error("rpc paths must not be nested under /collections")
	}
}

func TestGenerate_DefaultBasePath(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "items", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
		}, PrimaryKey: []string{"id"}},
	}, nil)

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)
	paths := doc["paths"].(map[string]any)

	if _, ok := paths["/items"]; !ok {
		t.Error("missing /items path with empty BasePath")
	}
	if _, ok := paths["/items/{id}"]; !ok {
		t.Error("missing /items/{id} path with empty BasePath")
	}
}

func TestGenerate_BasePathNormalization(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "items", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
		}, PrimaryKey: []string{"id"}},
	}, map[string]*schema.Function{
		"public.echo": {
			Schema:     "public",
			Name:       "echo",
			ReturnType: "text",
			Parameters: []*schema.FuncParam{{Name: "value", Type: "text", Position: 1}},
		},
	})

	data, err := Generate(cache, Options{BasePath: " /api/// "})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)
	paths := doc["paths"].(map[string]any)

	expected := []string{
		"/api/collections/items",
		"/api/collections/items/{id}",
		"/api/collections/items/aggregate",
		"/api/collections/items/import",
		"/api/collections/items/export.csv",
		"/api/collections/items/export.json",
		"/api/collections/items/batch",
		"/api/rpc/echo",
	}
	for _, path := range expected {
		if _, ok := paths[path]; !ok {
			t.Errorf("missing normalized path %q", path)
		}
	}
}

func TestGenerate_RPCPathsArePostOnly(t *testing.T) {
	cache := testCache(nil, map[string]*schema.Function{
		"public.echo": {
			Schema:     "public",
			Name:       "echo",
			ReturnType: "text",
			Parameters: []*schema.FuncParam{{Name: "value", Type: "text", Position: 1}},
		},
	})

	data, err := Generate(cache, Options{BasePath: "/api"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	paths := doc["paths"].(map[string]any)
	rpcPath, ok := paths["/api/rpc/echo"].(map[string]any)
	if !ok {
		t.Fatal("missing /api/rpc/echo path")
	}
	if rpcPath["post"] == nil {
		t.Fatal("rpc path should expose POST")
	}
	if rpcPath["get"] != nil {
		t.Fatal("rpc path should not expose GET when the server route is POST-only")
	}
}
