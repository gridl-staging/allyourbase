package openapi

import (
	"encoding/json"
	"testing"

	"github.com/allyourbase/ayb/internal/schema"
)

func TestGenerate_Stage2Endpoints(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "users", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "email", TypeName: "text"},
			{Name: "created_at", TypeName: "timestamptz"},
		}, PrimaryKey: []string{"id"}},
	}, nil)

	data, err := Generate(cache, Options{BasePath: "/api"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	json.Unmarshal(data, &doc)
	paths := doc["paths"].(map[string]any)

	aggregatePath, ok := paths["/api/collections/users/aggregate"].(map[string]any)
	if !ok {
		t.Fatal("missing /api/collections/users/aggregate path")
	}
	if aggregatePath["get"] == nil {
		t.Error("aggregate path should have GET operation")
	}
	aggOp := aggregatePath["get"].(map[string]any)
	aggParams := aggOp["parameters"].([]any)
	hasAggregate := false
	for _, p := range aggParams {
		pm := p.(map[string]any)
		if pm["name"] == "aggregate" && pm["required"] == true {
			hasAggregate = true
		}
	}
	if !hasAggregate {
		t.Error("aggregate param should be required")
	}

	importPath, ok := paths["/api/collections/users/import"].(map[string]any)
	if !ok {
		t.Fatal("missing /api/collections/users/import path")
	}
	if importPath["post"] == nil {
		t.Error("import path should have POST operation")
	}
	impOp := importPath["post"].(map[string]any)
	reqBody := impOp["requestBody"].(map[string]any)
	content := reqBody["content"].(map[string]any)
	if _, ok := content["text/csv"]; !ok {
		t.Error("import should support text/csv content type")
	}
	jsonImportContent, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Error("import should support application/json content type")
	} else {
		importSchema := jsonImportContent["schema"].(map[string]any)
		importItems := importSchema["items"].(map[string]any)
		if ref := importItems["$ref"]; ref != "#/components/schemas/UsersWrite" {
			t.Errorf("import JSON items $ref = %v, want #/components/schemas/UsersWrite", ref)
		}

		comps := doc["components"].(map[string]any)
		schemas := comps["schemas"].(map[string]any)
		usersWriteSchema := schemas["UsersWrite"].(map[string]any)
		importProps := usersWriteSchema["properties"].(map[string]any)
		if _, ok := importProps["id"]; !ok {
			t.Error("import JSON schema should include primary key field when table has one")
		}
	}
	impParams := impOp["parameters"].([]any)
	impParamNames := make(map[string]bool)
	for _, p := range impParams {
		pm := p.(map[string]any)
		impParamNames[pm["name"].(string)] = true
	}
	if !impParamNames["mode"] {
		t.Error("import should have mode query param")
	}
	if !impParamNames["on_conflict"] {
		t.Error("import should have on_conflict query param")
	}

	exportCSVPath, ok := paths["/api/collections/users/export.csv"].(map[string]any)
	if !ok {
		t.Fatal("missing /api/collections/users/export.csv path")
	}
	if exportCSVPath["get"] == nil {
		t.Error("export.csv path should have GET operation")
	}
	csvOp := exportCSVPath["get"].(map[string]any)
	csvParams := csvOp["parameters"].([]any)
	csvParamNames := make(map[string]bool, len(csvParams))
	for _, p := range csvParams {
		pm := p.(map[string]any)
		csvParamNames[pm["name"].(string)] = true
	}
	for _, name := range []string{"filter", "sort", "search", "fields"} {
		if !csvParamNames[name] {
			t.Errorf("export.csv missing query param %q", name)
		}
	}
	if csvParamNames["limit"] {
		t.Error("export.csv must not document unsupported limit query param")
	}
	csvResp := csvOp["responses"].(map[string]any)["200"].(map[string]any)
	csvContent := csvResp["content"].(map[string]any)
	if _, ok := csvContent["text/csv"]; !ok {
		t.Error("export.csv should have text/csv response")
	}

	exportJSONPath, ok := paths["/api/collections/users/export.json"].(map[string]any)
	if !ok {
		t.Fatal("missing /api/collections/users/export.json path")
	}
	if exportJSONPath["get"] == nil {
		t.Error("export.json path should have GET operation")
	}
	jsonOp := exportJSONPath["get"].(map[string]any)
	jsonParams := jsonOp["parameters"].([]any)
	jsonParamNames := make(map[string]bool, len(jsonParams))
	for _, p := range jsonParams {
		pm := p.(map[string]any)
		jsonParamNames[pm["name"].(string)] = true
	}
	if jsonParamNames["limit"] {
		t.Error("export.json must not document unsupported limit query param")
	}
	jsonResp := jsonOp["responses"].(map[string]any)["200"].(map[string]any)
	jsonContent := jsonResp["content"].(map[string]any)
	if _, ok := jsonContent["application/json"]; !ok {
		t.Error("export.json should have application/json response")
	}

	batchPath, ok := paths["/api/collections/users/batch"].(map[string]any)
	if !ok {
		t.Fatal("missing /api/collections/users/batch path")
	}
	if batchPath["post"] == nil {
		t.Error("batch path should have POST operation")
	}
	batchOp := batchPath["post"].(map[string]any)
	if batchOp["requestBody"] == nil {
		t.Error("batch should have requestBody")
	} else {
		batchReq := batchOp["requestBody"].(map[string]any)
		batchContent := batchReq["content"].(map[string]any)["application/json"].(map[string]any)
		batchSchema := batchContent["schema"].(map[string]any)
		ops := batchSchema["properties"].(map[string]any)["operations"].(map[string]any)
		opBody := ops["items"].(map[string]any)["properties"].(map[string]any)["body"].(map[string]any)
		if ref := opBody["$ref"]; ref != "#/components/schemas/UsersWrite" {
			t.Errorf("batch body $ref = %v, want #/components/schemas/UsersWrite", ref)
		}

		comps := doc["components"].(map[string]any)
		schemas := comps["schemas"].(map[string]any)
		usersWriteSchema := schemas["UsersWrite"].(map[string]any)
		bodyProps := usersWriteSchema["properties"].(map[string]any)
		if _, ok := bodyProps["id"]; !ok {
			t.Error("batch body schema should include primary key field when table has one")
		}
	}
}

func TestGenerate_ListQueryParamsExtended(t *testing.T) {
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

	paramNames := make(map[string]bool, len(params))
	for _, p := range params {
		pm := p.(map[string]any)
		paramNames[pm["name"].(string)] = true
	}

	for _, name := range []string{"fields", "filter", "sort", "page", "perPage", "search", "skipTotal", "cursor", "direction", "aggregate", "group"} {
		if !paramNames[name] {
			t.Errorf("missing query parameter %q", name)
		}
	}
}

func TestGenerate_ComponentSchemas(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "users", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "email", TypeName: "text"},
		}, PrimaryKey: []string{"id"}},
	}, nil)

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	comps, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatal("components is missing")
	}

	schemas, ok := comps["schemas"].(map[string]any)
	if !ok {
		t.Fatal("components.schemas is missing")
	}

	expectedKeys := []string{"Users", "UsersCreate", "UsersWrite"}
	for _, key := range expectedKeys {
		if _, ok := schemas[key]; !ok {
			t.Errorf("components.schemas missing key %q", key)
		}
	}

	usersSchema, ok := schemas["Users"].(map[string]any)
	if !ok {
		t.Fatal("Users schema is not an object")
	}
	if usersSchema["type"] != "object" {
		t.Errorf("Users schema type = %v, want object", usersSchema["type"])
	}
	usersProps := usersSchema["properties"].(map[string]any)
	if _, ok := usersProps["id"]; !ok {
		t.Error("Users schema should have id property")
	}
	if _, ok := usersProps["email"]; !ok {
		t.Error("Users schema should have email property")
	}

	usersCreateSchema, ok := schemas["UsersCreate"].(map[string]any)
	if !ok {
		t.Fatal("UsersCreate schema is not an object")
	}
	createProps := usersCreateSchema["properties"].(map[string]any)
	if _, ok := createProps["id"]; ok {
		t.Error("UsersCreate schema should exclude primary key")
	}
	if _, ok := createProps["email"]; !ok {
		t.Error("UsersCreate schema should have email property")
	}

	usersWriteSchema, ok := schemas["UsersWrite"].(map[string]any)
	if !ok {
		t.Fatal("UsersWrite schema is not an object")
	}
	writeProps := usersWriteSchema["properties"].(map[string]any)
	if _, ok := writeProps["id"]; !ok {
		t.Error("UsersWrite schema should include primary key")
	}
	if _, ok := writeProps["email"]; !ok {
		t.Error("UsersWrite schema should have email property")
	}
}

func TestGenerate_CRUDUsesRefs(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "posts", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "title", TypeName: "text"},
		}, PrimaryKey: []string{"id"}},
	}, nil)

	data, err := Generate(cache, Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	paths := doc["paths"].(map[string]any)

	collectionPath := paths["/posts"].(map[string]any)
	listOp := collectionPath["get"].(map[string]any)
	listResp := listOp["responses"].(map[string]any)["200"].(map[string]any)
	listContent := listResp["content"].(map[string]any)["application/json"].(map[string]any)
	listSchema := listContent["schema"].(map[string]any)
	listItems := listSchema["items"].(map[string]any)
	if ref := listItems["$ref"]; ref != "#/components/schemas/Posts" {
		t.Errorf("list response items $ref = %v, want #/components/schemas/Posts", ref)
	}

	readPath := paths["/posts/{id}"].(map[string]any)
	readOp := readPath["get"].(map[string]any)
	readResp := readOp["responses"].(map[string]any)["200"].(map[string]any)
	readContent := readResp["content"].(map[string]any)["application/json"].(map[string]any)
	readSchema := readContent["schema"].(map[string]any)
	if ref := readSchema["$ref"]; ref != "#/components/schemas/Posts" {
		t.Errorf("read response $ref = %v, want #/components/schemas/Posts", ref)
	}

	insertOp := collectionPath["post"].(map[string]any)
	insertBody := insertOp["requestBody"].(map[string]any)
	insertContent := insertBody["content"].(map[string]any)["application/json"].(map[string]any)
	insertReqSchema := insertContent["schema"].(map[string]any)
	if ref := insertReqSchema["$ref"]; ref != "#/components/schemas/PostsCreate" {
		t.Errorf("insert request $ref = %v, want #/components/schemas/PostsCreate", ref)
	}

	insertResp := insertOp["responses"].(map[string]any)["201"].(map[string]any)
	insertRespContent := insertResp["content"].(map[string]any)["application/json"].(map[string]any)
	insertRespSchema := insertRespContent["schema"].(map[string]any)
	if ref := insertRespSchema["$ref"]; ref != "#/components/schemas/Posts" {
		t.Errorf("insert response $ref = %v, want #/components/schemas/Posts", ref)
	}

	updateOp := readPath["patch"].(map[string]any)
	updateBody := updateOp["requestBody"].(map[string]any)
	updateContent := updateBody["content"].(map[string]any)["application/json"].(map[string]any)
	updateReqSchema := updateContent["schema"].(map[string]any)
	if ref := updateReqSchema["$ref"]; ref != "#/components/schemas/PostsCreate" {
		t.Errorf("update request $ref = %v, want #/components/schemas/PostsCreate", ref)
	}

	updateResp := updateOp["responses"].(map[string]any)["200"].(map[string]any)
	updateRespContent := updateResp["content"].(map[string]any)["application/json"].(map[string]any)
	updateRespSchema := updateRespContent["schema"].(map[string]any)
	if ref := updateRespSchema["$ref"]; ref != "#/components/schemas/Posts" {
		t.Errorf("update response $ref = %v, want #/components/schemas/Posts", ref)
	}
}

func TestGenerate_Stage2EndpointsUseRefs(t *testing.T) {
	cache := testCache([]*schema.Table{
		{Schema: "public", Name: "items", Kind: "table", Columns: []*schema.Column{
			{Name: "id", TypeName: "integer", IsPrimaryKey: true},
			{Name: "name", TypeName: "text"},
		}, PrimaryKey: []string{"id"}},
	}, nil)

	data, err := Generate(cache, Options{BasePath: "/api"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	paths := doc["paths"].(map[string]any)

	aggPath := paths["/api/collections/items/aggregate"].(map[string]any)
	aggOp := aggPath["get"].(map[string]any)
	aggResp := aggOp["responses"].(map[string]any)["200"].(map[string]any)
	aggContent := aggResp["content"].(map[string]any)["application/json"].(map[string]any)
	aggSchema := aggContent["schema"].(map[string]any)
	if aggSchema["type"] != "object" {
		t.Error("aggregate response should remain inline with type object")
	}

	importPath := paths["/api/collections/items/import"].(map[string]any)
	importOp := importPath["post"].(map[string]any)
	importBody := importOp["requestBody"].(map[string]any)
	importContent := importBody["content"].(map[string]any)
	jsonImport := importContent["application/json"].(map[string]any)
	jsonImportSchema := jsonImport["schema"].(map[string]any)
	jsonImportItems := jsonImportSchema["items"].(map[string]any)
	if ref := jsonImportItems["$ref"]; ref != "#/components/schemas/ItemsWrite" {
		t.Errorf("import JSON items $ref = %v, want #/components/schemas/ItemsWrite", ref)
	}

	exportJSONPath := paths["/api/collections/items/export.json"].(map[string]any)
	exportJSONOp := exportJSONPath["get"].(map[string]any)
	exportJSONResp := exportJSONOp["responses"].(map[string]any)["200"].(map[string]any)
	exportJSONContent := exportJSONResp["content"].(map[string]any)["application/json"].(map[string]any)
	exportJSONSchema := exportJSONContent["schema"].(map[string]any)
	exportJSONItems := exportJSONSchema["items"].(map[string]any)
	if ref := exportJSONItems["$ref"]; ref != "#/components/schemas/Items" {
		t.Errorf("export.json items $ref = %v, want #/components/schemas/Items", ref)
	}

	batchPath := paths["/api/collections/items/batch"].(map[string]any)
	batchOp := batchPath["post"].(map[string]any)
	batchBody := batchOp["requestBody"].(map[string]any)
	batchContent := batchBody["content"].(map[string]any)["application/json"].(map[string]any)
	batchSchema := batchContent["schema"].(map[string]any)
	ops := batchSchema["properties"].(map[string]any)["operations"].(map[string]any)
	opItem := ops["items"].(map[string]any)
	opBody := opItem["properties"].(map[string]any)["body"].(map[string]any)
	if ref := opBody["$ref"]; ref != "#/components/schemas/ItemsWrite" {
		t.Errorf("batch body $ref = %v, want #/components/schemas/ItemsWrite", ref)
	}

	importResp := importOp["responses"].(map[string]any)["200"].(map[string]any)
	importRespContent := importResp["content"].(map[string]any)["application/json"].(map[string]any)
	importRespSchema := importRespContent["schema"].(map[string]any)
	if importRespSchema["type"] != "object" {
		t.Error("import response should remain inline")
	}
}
