package edgefunc

import (
	"context"
	"fmt"

	"github.com/dop251/goja"
)

// AIGenerateFunc handles ayb.ai.generateText() calls from edge functions.
// messages is a slice of {"role": "user|system|assistant", "content": "..."} objects.
// opts may contain "provider", "model", "systemPrompt", "maxTokens".
// Returns the generated text.
type AIGenerateFunc func(ctx context.Context, messages []map[string]any, opts map[string]any) (string, error)

// AIRenderPromptFunc handles ayb.ai.renderPrompt() calls from edge functions.
// Returns the rendered prompt text for the named template with the given variables.
type AIRenderPromptFunc func(ctx context.Context, name string, vars map[string]any) (string, error)

// AIParseDocumentFunc handles ayb.ai.parseDocument() calls from edge functions.
// url is extracted from opts["url"] for convenience; opts contains all original fields.
// Returns the parsed data as a JSON-serialisable map.
type AIParseDocumentFunc func(ctx context.Context, url string, opts map[string]any) (map[string]any, error)

// registerAIBridge adds ayb.ai.{generateText,renderPrompt,parseDocument} to the given ayb object.
// Only methods whose backing function is non-nil are registered.
// No-op (returns nil) when all three functions are nil.
func registerAIBridge(
	vm *goja.Runtime,
	ctx context.Context,
	ayb *goja.Object,
	generate AIGenerateFunc,
	renderPrompt AIRenderPromptFunc,
	parseDoc AIParseDocumentFunc,
) error {
	if generate == nil && renderPrompt == nil && parseDoc == nil {
		return nil
	}

	aiObj := vm.NewObject()

	if generate != nil {
		_ = aiObj.Set("generateText", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(vm.NewTypeError("ayb.ai.generateText() requires an options object"))
			}
			exported, ok := call.Arguments[0].Export().(map[string]interface{})
			if !ok {
				panic(vm.NewTypeError("ayb.ai.generateText() argument must be an object"))
			}
			var messages []map[string]any
			if raw, ok := exported["messages"]; ok {
				if slice, ok := raw.([]interface{}); ok {
					for _, item := range slice {
						if m, ok := item.(map[string]interface{}); ok {
							messages = append(messages, m)
						}
					}
				}
			}
			opts := make(map[string]any, len(exported))
			for k, v := range exported {
				if k != "messages" {
					opts[k] = v
				}
			}
			text, err := generate(ctx, messages, opts)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("ayb.ai.generateText: %w", err)))
			}
			return vm.ToValue(text)
		})
	}

	if renderPrompt != nil {
		_ = aiObj.Set("renderPrompt", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(vm.NewTypeError("ayb.ai.renderPrompt() requires (name, vars?)"))
			}
			name := call.Arguments[0].String()
			vars := make(map[string]any)
			if len(call.Arguments) > 1 {
				if exported, ok := call.Arguments[1].Export().(map[string]interface{}); ok {
					for k, v := range exported {
						vars[k] = v
					}
				}
			}
			text, err := renderPrompt(ctx, name, vars)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("ayb.ai.renderPrompt: %w", err)))
			}
			return vm.ToValue(text)
		})
	}

	if parseDoc != nil {
		_ = aiObj.Set("parseDocument", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(vm.NewTypeError("ayb.ai.parseDocument() requires an options object"))
			}
			exported, ok := call.Arguments[0].Export().(map[string]interface{})
			if !ok {
				panic(vm.NewTypeError("ayb.ai.parseDocument() argument must be an object"))
			}
			url, _ := exported["url"].(string)
			opts := make(map[string]any, len(exported))
			for k, v := range exported {
				opts[k] = v
			}
			result, err := parseDoc(ctx, url, opts)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("ayb.ai.parseDocument: %w", err)))
			}
			return vm.ToValue(result)
		})
	}

	return ayb.Set("ai", aiObj)
}
