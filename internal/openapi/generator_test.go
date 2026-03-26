package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
)

func testCache(tables []*schema.Table, funcs map[string]*schema.Function) *schema.SchemaCache {
	tblMap := make(map[string]*schema.Table, len(tables))
	for _, t := range tables {
		key := t.Schema + "." + t.Name
		tblMap[key] = t
	}
	return &schema.SchemaCache{
		Tables:    tblMap,
		Functions: funcs,
		Schemas:   []string{"public"},
	}
}

func TestGenerate_SingleTable(t *testing.T) {
	cache := testCache([]*schema.Table{
		{
			Schema: "public",
			Name:   "posts",
			Kind:   "table",
			Columns: []*schema.Column{
				{Name: "id", TypeName: "integer", IsPrimaryKey: true},
				{Name: "title", TypeName: "text"},
				{Name: "body", TypeName: "text", IsNullable: true},
				{Name: "created_at", TypeName: "timestamptz"},
			},
			PrimaryKey: []string{"id"},
		},
	}, nil)

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Verify OpenAPI version.
	if v := doc["openapi"]; v != "3.1.0" {
		t.Errorf("openapi = %v, want 3.1.0", v)
	}

	// Verify paths exist.
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("paths is not an object")
	}

	// Collection path: GET (list) + POST (create).
	postsPath, ok := paths["/posts"].(map[string]any)
	if !ok {
		t.Fatal("missing /posts path")
	}
	for _, method := range []string{"get", "post"} {
		if _, ok := postsPath[method]; !ok {
			t.Errorf("missing %s operation on /posts", method)
		}
	}
	for _, method := range []string{"patch", "delete"} {
		if _, ok := postsPath[method]; ok {
			t.Errorf("%s should not be on /posts (belongs on /posts/{id})", method)
		}
	}

	// Record path: GET (read) + PATCH (update) + DELETE.
	recordPath, ok := paths["/posts/{id}"].(map[string]any)
	if !ok {
		t.Fatal("missing /posts/{id} path")
	}
	for _, method := range []string{"get", "patch", "delete"} {
		if _, ok := recordPath[method]; !ok {
			t.Errorf("missing %s operation on /posts/{id}", method)
		}
	}

	getOp := postsPath["get"].(map[string]any)
	resp200 := getOp["responses"].(map[string]any)["200"].(map[string]any)
	content := resp200["content"].(map[string]any)["application/json"].(map[string]any)
	schemaObj := content["schema"].(map[string]any)
	if schemaObj["type"] != "array" {
		t.Errorf("GET response type = %v, want array", schemaObj["type"])
	}
	items := schemaObj["items"].(map[string]any)
	if ref := items["$ref"]; ref != "#/components/schemas/Posts" {
		t.Errorf("GET response items $ref = %v, want #/components/schemas/Posts", ref)
	}

	// Verify POST request body exists.
	postOp := postsPath["post"].(map[string]any)
	if _, ok := postOp["requestBody"]; !ok {
		t.Error("POST missing requestBody")
	}

	readOp := recordPath["get"].(map[string]any)
	readResp := readOp["responses"].(map[string]any)["200"].(map[string]any)
	readContent := readResp["content"].(map[string]any)["application/json"].(map[string]any)
	readSchema := readContent["schema"].(map[string]any)
	if ref := readSchema["$ref"]; ref != "#/components/schemas/Posts" {
		t.Errorf("read GET response $ref = %v, want #/components/schemas/Posts", ref)
	}

	// Verify {id} path parameter on record-level operations.
	patchOp := recordPath["patch"].(map[string]any)
	patchParams := patchOp["parameters"].([]any)
	if len(patchParams) == 0 {
		t.Error("PATCH missing {id} path parameter")
	} else {
		p := patchParams[0].(map[string]any)
		if p["name"] != "id" || p["in"] != "path" {
			t.Errorf("PATCH param: got name=%v in=%v, want id/path", p["name"], p["in"])
		}
	}
}

func TestGenerate_ExcludesSystemTables(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "users", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
		}, PrimaryKey: []string{"id"}},
		{Schema: "public", Name: "_ayb_migrations", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
		}, PrimaryKey: []string{"id"}},
		{Schema: "public", Name: "_ayb_backups", Kind: "table", Columns: []*schema.Column{
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

	if _, ok := paths["/users"]; !ok {
		t.Error("expected /users path")
	}
	if _, ok := paths["/users/{id}"]; !ok {
		t.Error("expected /users/{id} path")
	}
	for _, name := range []string{"/_ayb_migrations", "/_ayb_migrations/{id}", "/_ayb_backups", "/_ayb_backups/{id}"} {
		if _, ok := paths[name]; ok {
			t.Errorf("%s should be excluded", name)
		}
	}
}

func TestGenerate_PKReadOnly(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "items", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid", IsPrimaryKey: true},
			{Name: "name", TypeName: "text"},
		}, PrimaryKey: []string{"id"}},
	}, nil)

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)

	comps := doc["components"].(map[string]any)
	schemas := comps["schemas"].(map[string]any)

	itemsSchema := schemas["Items"].(map[string]any)
	itemsProps := itemsSchema["properties"].(map[string]any)
	idProp := itemsProps["id"].(map[string]any)
	if idProp["readOnly"] != true {
		t.Error("PK column should be readOnly in row schema")
	}

	itemsCreateSchema := schemas["ItemsCreate"].(map[string]any)
	createProps := itemsCreateSchema["properties"].(map[string]any)
	if _, ok := createProps["id"]; ok {
		t.Error("PK column should be excluded from create schema")
	}

	paths := doc["paths"].(map[string]any)
	recordPath, ok := paths["/items/{id}"].(map[string]any)
	if !ok {
		t.Fatal("missing /items/{id} path")
	}
	if _, ok := recordPath["patch"]; !ok {
		t.Error("missing PATCH on /items/{id}")
	}
}

func TestGenerate_EnumColumn(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "tasks", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "status", TypeName: "task_status", IsEnum: true, EnumValues: []string{"open", "closed", "pending"}},
		}, PrimaryKey: []string{"id"}},
	}, nil)

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)

	comps := doc["components"].(map[string]any)
	schemas := comps["schemas"].(map[string]any)
	tasksSchema := schemas["Tasks"].(map[string]any)
	props := tasksSchema["properties"].(map[string]any)

	statusProp := props["status"].(map[string]any)
	if statusProp["type"] != "string" {
		t.Error("enum column type should be string")
	}
	enumVals, ok := statusProp["enum"].([]any)
	if !ok || len(enumVals) != 3 {
		t.Errorf("enum values = %v, want 3 values", enumVals)
	}
}

func TestGenerate_NullableColumn(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "things", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "desc", TypeName: "text", IsNullable: true},
		}, PrimaryKey: []string{"id"}},
	}, nil)

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)

	comps := doc["components"].(map[string]any)
	schemas := comps["schemas"].(map[string]any)
	thingsSchema := schemas["Things"].(map[string]any)
	props := thingsSchema["properties"].(map[string]any)

	descProp := props["desc"].(map[string]any)
	oneOf, ok := descProp["oneOf"].([]any)
	if !ok || len(oneOf) != 2 {
		t.Fatalf("nullable column should have oneOf with 2 entries, got %v", descProp)
	}
	secondType := oneOf[1].(map[string]any)["type"]
	if secondType != "null" {
		t.Errorf("oneOf[1].type = %v, want null", secondType)
	}
}

func TestGenerate_RPCFunction(t *testing.T) {
	cache := testCache(nil, map[string]*schema.Function{
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

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)
	paths := doc["paths"].(map[string]any)

	rpcPath, ok := paths["/rpc/add_numbers"].(map[string]any)
	if !ok {
		t.Fatal("missing /rpc/add_numbers path")
	}
	if _, ok := rpcPath["post"]; !ok {
		t.Error("missing POST on /rpc/add_numbers")
	}
	if _, ok := rpcPath["get"]; ok {
		t.Error("unexpected GET on /rpc/add_numbers; server routes only expose POST")
	}

	// Verify POST request body has parameters.
	postOp := rpcPath["post"].(map[string]any)
	reqBody := postOp["requestBody"].(map[string]any)
	bodyContent := reqBody["content"].(map[string]any)["application/json"].(map[string]any)
	bodySchema := bodyContent["schema"].(map[string]any)
	bodyProps := bodySchema["properties"].(map[string]any)
	if _, ok := bodyProps["a"]; !ok {
		t.Error("missing parameter 'a' in RPC body schema")
	}
	if _, ok := bodyProps["b"]; !ok {
		t.Error("missing parameter 'b' in RPC body schema")
	}
}

func TestGenerate_SecuritySchemes(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "x", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
		}, PrimaryKey: []string{"id"}},
	}, nil)

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)

	comps := doc["components"].(map[string]any)
	schemes := comps["securitySchemes"].(map[string]any)

	bearer, ok := schemes["bearerAuth"].(map[string]any)
	if !ok {
		t.Fatal("missing bearerAuth scheme")
	}
	if bearer["type"] != "http" || bearer["scheme"] != "bearer" || bearer["bearerFormat"] != "JWT" {
		t.Errorf("bearerAuth scheme incorrect: %v", bearer)
	}

	apikey, ok := schemes["apikeyAuth"].(map[string]any)
	if !ok {
		t.Fatal("missing apikeyAuth scheme")
	}
	if apikey["type"] != "apiKey" || apikey["in"] != "header" || apikey["name"] != "apikey" {
		t.Errorf("apikeyAuth scheme incorrect: %v", apikey)
	}

	// Verify global security.
	security := doc["security"].([]any)
	if len(security) != 2 {
		t.Errorf("expected 2 security entries, got %d", len(security))
	}
}

func TestGenerate_ListQueryParams(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "items", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", TypeName: "text"},
		}, PrimaryKey: []string{"id"}},
	}, nil)

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)
	paths := doc["paths"].(map[string]any)
	getOp := paths["/items"].(map[string]any)["get"].(map[string]any)
	params := getOp["parameters"].([]any)

	// Should have fields, filter, sort, page, perPage, search, skipTotal.
	paramNames := make(map[string]bool, len(params))
	for _, p := range params {
		pm := p.(map[string]any)
		paramNames[pm["name"].(string)] = true
	}

	for _, name := range []string{"fields", "filter", "sort", "page", "perPage", "search", "skipTotal"} {
		if !paramNames[name] {
			t.Errorf("missing query parameter %q", name)
		}
	}
}

func TestGenerate_AllColumnTypes(t *testing.T) {
	columns := []*schema.Column{
		{Name: "c_int", TypeName: "integer"},
		{Name: "c_big", TypeName: "bigint"},
		{Name: "c_text", TypeName: "text"},
		{Name: "c_bool", TypeName: "boolean"},
		{Name: "c_ts", TypeName: "timestamptz"},
		{Name: "c_date", TypeName: "date"},
		{Name: "c_time", TypeName: "time"},
		{Name: "c_uuid", TypeName: "uuid"},
		{Name: "c_json", TypeName: "jsonb"},
		{Name: "c_num", TypeName: "numeric"},
		{Name: "c_float", TypeName: "real"},
		{Name: "c_bytes", TypeName: "bytea"},
		{Name: "c_vec", TypeName: "vector", IsVector: true},
		{Name: "c_geom", TypeName: "geometry", IsGeometry: true},
		{Name: "c_enum", TypeName: "mood", IsEnum: true, EnumValues: []string{"happy", "sad"}},
		{Name: "c_arr", TypeName: "integer[]"},
	}
	// First column is PK.
	columns[0].IsPrimaryKey = true

	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "everything", Kind: "table", Columns: columns, PrimaryKey: []string{"c_int"}},
	}, nil)

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Just verify valid JSON output.
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	paths := doc["paths"].(map[string]any)
	if _, ok := paths["/everything"]; !ok {
		t.Error("missing /everything path")
	}
}

func TestGenerate_EmptySchema(t *testing.T) {
	cache := testCache(nil, nil)
	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)
	paths := doc["paths"].(map[string]any)
	if len(paths) != 0 {
		t.Errorf("expected empty paths, got %d", len(paths))
	}
}

func TestGenerate_ValidJSON(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "users", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "uuid", IsPrimaryKey: true},
			{Name: "email", TypeName: "text"},
			{Name: "bio", TypeName: "text", IsNullable: true},
		}, PrimaryKey: []string{"id"}},
	}, map[string]*schema.Function{
		"public.hello": {
			Schema:     "public",
			Name:       "hello",
			ReturnType: "text",
			Parameters: []*schema.FuncParam{{Name: "name", Type: "text"}},
		},
	})

	data, err := Generate(cache, Options{Version: "1.0.0", Title: "Test API"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Validate that we get well-formed JSON.
	if !json.Valid(data) {
		t.Error("generated spec is not valid JSON")
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)

	info := doc["info"].(map[string]any)
	if info["title"] != "Test API" {
		t.Errorf("title = %v, want Test API", info["title"])
	}
	if info["version"] != "1.0.0" {
		t.Errorf("version = %v, want 1.0.0", info["version"])
	}
}

func containsAll(source string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(source, part) {
			return false
		}
	}
	return true
}
