package openapi

import (
	"encoding/json"
	"strings"

	"github.com/allyourbase/ayb/internal/schema"
)

// Options configures OpenAPI spec generation.
type Options struct {
	Version  string // API version string for info block (default "0.1.0")
	BasePath string // Base path prefix (default empty)
	Title    string // API title (default "AYB Auto-Generated API")
}

// spec is the root OpenAPI 3.1 document structure.
type spec struct {
	OpenAPI    string                `json:"openapi"`
	Info       specInfo              `json:"info"`
	Paths      map[string]*pathItem  `json:"paths"`
	Components *components           `json:"components"`
	Security   []map[string][]string `json:"security"`
}

type specInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type components struct {
	SecuritySchemes map[string]*securityScheme `json:"securitySchemes"`
	Schemas         map[string]*schemaProperty `json:"schemas,omitempty"`
}

type securityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	In           string `json:"in,omitempty"`
	Name         string `json:"name,omitempty"`
}

type pathItem struct {
	Get    *operation `json:"get,omitempty"`
	Post   *operation `json:"post,omitempty"`
	Patch  *operation `json:"patch,omitempty"`
	Delete *operation `json:"delete,omitempty"`
}

type operation struct {
	Summary     string               `json:"summary"`
	Tags        []string             `json:"tags"`
	OperationID string               `json:"operationId"`
	Parameters  []*parameter         `json:"parameters,omitempty"`
	RequestBody *requestBody         `json:"requestBody,omitempty"`
	Responses   map[string]*response `json:"responses"`
}

type parameter struct {
	Name        string          `json:"name"`
	In          string          `json:"in"`
	Description string          `json:"description,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Schema      *schemaProperty `json:"schema"`
}

type requestBody struct {
	Required bool                     `json:"required"`
	Content  map[string]*mediaContent `json:"content"`
}

type mediaContent struct {
	Schema *schemaProperty `json:"schema"`
}

type response struct {
	Description string                   `json:"description"`
	Content     map[string]*mediaContent `json:"content,omitempty"`
}

// Generate builds an OpenAPI 3.1 JSON document from a SchemaCache.
func Generate(cache *schema.SchemaCache, opts Options) ([]byte, error) {
	if opts.Version == "" {
		opts.Version = "0.1.0"
	}
	if opts.Title == "" {
		opts.Title = "AYB Auto-Generated API"
	}

	doc := &spec{
		OpenAPI: "3.1.0",
		Info: specInfo{
			Title:   opts.Title,
			Version: opts.Version,
		},
		Paths: make(map[string]*pathItem),
		Components: &components{
			SecuritySchemes: map[string]*securityScheme{
				"bearerAuth": {
					Type:         "http",
					Scheme:       "bearer",
					BearerFormat: "JWT",
				},
				"apikeyAuth": {
					Type: "apiKey",
					In:   "header",
					Name: "apikey",
				},
			},
			Schemas: make(map[string]*schemaProperty),
		},
		Security: []map[string][]string{
			{"bearerAuth": {}},
			{"apikeyAuth": {}},
		},
	}

	// Add table paths.
	tables := cache.TableList()
	emitGeoJSONComponents := shouldEmitGeoJSONComponents(cache, tables)
	if emitGeoJSONComponents {
		addGeoJSONComponentSchemas(doc.Components.Schemas)
	}
	for _, tbl := range tables {
		if isSystemTable(tbl.Name) {
			continue
		}
		if tbl.Kind != "table" && tbl.Kind != "view" && tbl.Kind != "materialized_view" {
			continue
		}

		rowSchema := tableRowSchema(tbl, emitGeoJSONComponents)
		createSchema := tableCreateSchema(tbl, emitGeoJSONComponents)
		writeSchema := tableWriteSchema(tbl, emitGeoJSONComponents)

		rowCompName := tableComponentName(tbl.Name, "")
		createCompName := tableComponentName(tbl.Name, "Create")
		writeCompName := tableComponentName(tbl.Name, "Write")

		doc.Components.Schemas[rowCompName] = rowSchema
		doc.Components.Schemas[createCompName] = createSchema
		doc.Components.Schemas[writeCompName] = writeSchema

		collectionPath := collectionPathForTable(opts.BasePath, tbl.Name)
		doc.Paths[collectionPath] = buildTableCollectionPath(tbl, refSchema(rowCompName), refSchema(createCompName), cache)

		if len(tbl.PrimaryKey) > 0 {
			recordPath := collectionPath + "/{id}"
			doc.Paths[recordPath] = buildTableRecordPath(tbl, refSchema(rowCompName), refSchema(createCompName))
		}

		doc.Paths[collectionPath+"/aggregate"] = buildAggregatePath(tbl, cache)
		doc.Paths[collectionPath+"/import"] = buildImportPath(tbl, refSchema(writeCompName))
		doc.Paths[collectionPath+"/export.csv"] = buildExportCSVPath(tbl)
		doc.Paths[collectionPath+"/export.json"] = buildExportJSONPath(tbl, refSchema(rowCompName))
		doc.Paths[collectionPath+"/batch"] = buildBatchPath(tbl, refSchema(writeCompName))
	}

	// Add RPC function paths.
	for _, key := range sortedFuncKeys(cache.Functions) {
		fn := cache.Functions[key]
		if isSystemTable(fn.Name) {
			continue
		}
		path := rpcPathForFunction(opts.BasePath, fn.Name)
		doc.Paths[path] = buildRPCPathItem(fn)
	}

	return json.MarshalIndent(doc, "", "  ")
}

func isSystemTable(name string) bool {
	return strings.HasPrefix(name, "_ayb_")
}

func normalizeBasePath(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	if !strings.HasPrefix(base, "/") {
		return "/" + base
	}
	return base
}

func refSchema(componentName string) *schemaProperty {
	return &schemaProperty{Ref: "#/components/schemas/" + componentName}
}
