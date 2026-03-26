// Package openapi generates OpenAPI path items and operations for RPC functions.
package openapi

import (
	"sort"

	"github.com/allyourbase/ayb/internal/schema"
)

func sortedFuncKeys(functionsByKey map[string]*schema.Function) []string {
	keys := make([]string, 0, len(functionsByKey))
	for key := range functionsByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func rpcPathForFunction(basePath, functionName string) string {
	base := normalizeBasePath(basePath)
	if base == "" {
		return "/rpc/" + functionName
	}
	return base + "/rpc/" + functionName
}

// buildRPCPathItem creates the POST operation for an RPC function.
func buildRPCPathItem(fn *schema.Function) *pathItem {
	responseSchema := returnTypeToSchema(fn.ReturnType, fn.ReturnsSet)
	return &pathItem{
		Post: buildRPCPostOp(fn, responseSchema),
	}
}

// buildRPCPostOp creates an OpenAPI operation for POST requests to an RPC function. It includes the response schema and, if the function has parameters, a JSON request body schema containing them.
func buildRPCPostOp(fn *schema.Function, responseSchema *schemaProperty) *operation {
	op := &operation{
		Summary:     "Call " + fn.Name,
		Tags:        []string{"rpc"},
		OperationID: "rpc_post_" + fn.Name,
		Responses: map[string]*response{
			"200": {
				Description: "Successful response",
				Content: map[string]*mediaContent{
					"application/json": {Schema: responseSchema},
				},
			},
		},
	}

	if len(fn.Parameters) > 0 {
		bodySchema := rpcBodySchema(fn)
		op.RequestBody = &requestBody{
			Required: true,
			Content: map[string]*mediaContent{
				"application/json": {Schema: bodySchema},
			},
		}
	}

	return op
}

func rpcBodySchema(fn *schema.Function) *schemaProperty {
	properties := make(map[string]*schemaProperty, len(fn.Parameters))
	for _, param := range fn.Parameters {
		properties[param.Name] = funcParamTypeToSchema(param.Type)
	}
	return &schemaProperty{Type: "object", Properties: properties}
}
