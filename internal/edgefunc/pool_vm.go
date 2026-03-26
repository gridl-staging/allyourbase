package edgefunc

import (
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/dop251/goja"

	"github.com/allyourbase/ayb/internal/schema"
)

// createVM creates a fresh Goja runtime with frozen built-ins.
// Freezing prevents user code from mutating prototypes, which would
// matter if VMs were reused (they're not currently, but this is defensive).
func createVM(memoryLimitMB int) *goja.Runtime {
	vm := goja.New()
	vm.SetMaxCallStackSize(maxCallStackSizeForMemoryLimit(memoryLimitMB))
	freezeScript := `
Object.freeze(Object.prototype);
Object.freeze(Array.prototype);
Object.freeze(String.prototype);
Object.freeze(Number.prototype);
Object.freeze(Boolean.prototype);
Object.freeze(Function.prototype);
Object.freeze(RegExp.prototype);
Object.freeze(Date.prototype);
Object.freeze(Error.prototype);
Object.freeze(JSON);
Object.freeze(Math);
`
	_, _ = vm.RunString(freezeScript)
	return vm
}

// wrapStrict prepends "use strict" to enable strict mode enforcement.
func wrapStrict(code string) string {
	return `"use strict";` + "\n" + code
}

// contentHash returns a hex-encoded SHA-256 hash of the source code.
func contentHash(code string) string {
	h := sha256.Sum256([]byte(code))
	return fmt.Sprintf("%x", h)
}

// setupConsole registers console.log on the VM, capturing output to the writer.
func setupConsole(vm *goja.Runtime, stdout *strings.Builder, stdoutLimitBytes int) error {
	truncated := false
	console := vm.NewObject()
	if err := console.Set("log", func(call goja.FunctionCall) goja.Value {
		parts := make([]string, len(call.Arguments))
		for i, arg := range call.Arguments {
			parts[i] = arg.String()
		}
		appendBoundedStdout(stdout, strings.Join(parts, " ")+"\n", stdoutLimitBytes, &truncated)
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("setting console.log: %w", err)
	}
	return vm.Set("console", console)
}

// BridgeConfig bundles all dependencies for the ayb JS namespace.
type BridgeConfig struct {
	EnvVars             map[string]string
	QueryExecutor       QueryExecutor
	SchemaCacheGetter   func() *schema.SchemaCache
	NamedInvoker        NamedInvoker
	ProviderTokenGetter ProviderTokenGetter
	AIGenerate          AIGenerateFunc
	AIRenderPrompt      AIRenderPromptFunc
	AIParseDocument     AIParseDocumentFunc
	EmailSend           EmailSendFunc
}

// setupAYB registers the ayb namespace (env, db, functions, auth, ai, email, spatial) on the VM.
func setupAYB(vm *goja.Runtime, ctx context.Context, cfg BridgeConfig) error {
	ayb := vm.NewObject()

	registerEnvOnAYB(vm, ayb, cfg.EnvVars)

	if cfg.QueryExecutor != nil {
		registerDBOnAYB(vm, ctx, ayb, cfg.QueryExecutor)
	}
	if cfg.NamedInvoker != nil {
		registerFuncInvokeOnAYB(vm, ctx, ayb, cfg.NamedInvoker)
	}

	registerAuthOnAYB(vm, ctx, ayb, cfg.ProviderTokenGetter)

	if err := registerAIBridge(vm, ctx, ayb, cfg.AIGenerate, cfg.AIRenderPrompt, cfg.AIParseDocument); err != nil {
		return fmt.Errorf("setting ayb.ai: %w", err)
	}
	if err := registerEmailBridge(vm, ctx, ayb, cfg.EmailSend); err != nil {
		return fmt.Errorf("setting ayb.email: %w", err)
	}

	if err := vm.Set("ayb", ayb); err != nil {
		return fmt.Errorf("setting ayb: %w", err)
	}

	spatialExecutor, _ := cfg.QueryExecutor.(SpatialQueryExecutor)
	return setupSpatialWithContext(vm, ctx, spatialExecutor, cfg.SchemaCacheGetter)
}

// registerEnvOnAYB adds ayb.env.get(key) to the ayb object.
func registerEnvOnAYB(vm *goja.Runtime, ayb *goja.Object, envVars map[string]string) {
	env := vm.NewObject()
	_ = env.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		key := call.Arguments[0].String()
		if envVars == nil {
			return goja.Undefined()
		}
		val, ok := envVars[key]
		if !ok {
			return goja.Undefined()
		}
		return vm.ToValue(val)
	})
	_ = ayb.Set("env", env)
}

// registerDBOnAYB adds ayb.db.from(table) to the ayb object.
func registerDBOnAYB(vm *goja.Runtime, ctx context.Context, ayb *goja.Object, qe QueryExecutor) {
	db := vm.NewObject()
	_ = db.Set("from", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("ayb.db.from() requires a table name"))
		}
		table := call.Arguments[0].String()
		return newQueryBuilder(vm, ctx, qe, table)
	})
	_ = ayb.Set("db", db)
}

// registerFuncInvokeOnAYB adds ayb.functions.invoke(name, request) to the ayb object.
func registerFuncInvokeOnAYB(vm *goja.Runtime, ctx context.Context, ayb *goja.Object, ni NamedInvoker) {
	functions := vm.NewObject()
	_ = functions.Set("invoke", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.NewTypeError("ayb.functions.invoke() requires (name, request)"))
		}

		name := call.Arguments[0].String()
		reqExported := call.Arguments[1].Export()
		reqMap, ok := reqExported.(map[string]interface{})
		if !ok {
			panic(vm.NewTypeError("ayb.functions.invoke() request must be an object"))
		}

		req := Request{
			Method: stringFromMap(reqMap, "method"),
			Path:   stringFromMap(reqMap, "path"),
			Query:  stringFromMap(reqMap, "query"),
		}
		if body, ok := reqMap["body"]; ok {
			if v, ok := body.(string); ok {
				req.Body = []byte(v)
			}
		}

		resp, err := ni(ctx, name, req)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("ayb.functions.invoke(%q) failed: %w", name, err)))
		}

		respObj := vm.NewObject()
		_ = respObj.Set("statusCode", resp.StatusCode)
		_ = respObj.Set("body", string(resp.Body))
		if resp.Headers != nil {
			_ = respObj.Set("headers", resp.Headers)
		}
		return respObj
	})
	_ = ayb.Set("functions", functions)
}

// registerAuthOnAYB adds ayb.auth.getProviderToken(userId, provider) to the ayb object.
func registerAuthOnAYB(vm *goja.Runtime, ctx context.Context, ayb *goja.Object, getter ProviderTokenGetter) {
	auth := vm.NewObject()
	_ = auth.Set("getProviderToken", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(vm.NewTypeError("ayb.auth.getProviderToken() requires (userId, provider)"))
		}
		if getter == nil {
			panic(vm.NewTypeError("ayb.auth.getProviderToken() is not configured"))
		}

		userID := call.Arguments[0].String()
		provider := call.Arguments[1].String()
		token, err := getter(ctx, userID, provider)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("ayb.auth.getProviderToken() failed: %w", err)))
		}

		return vm.ToValue(token)
	})
	_ = ayb.Set("auth", auth)
}

func stdoutLimitBytesForMemoryLimit(memoryLimitMB int) int {
	if memoryLimitMB < 1 {
		return minStdoutCaptureBytes
	}
	limit := (memoryLimitMB * 1024 * 1024) / 4
	if limit > maxStdoutCaptureBytes {
		limit = maxStdoutCaptureBytes
	}
	if limit < minStdoutCaptureBytes {
		limit = minStdoutCaptureBytes
	}
	return limit
}

func maxCallStackSizeForMemoryLimit(memoryLimitMB int) int {
	if memoryLimitMB < 1 {
		return 256
	}
	frames := (memoryLimitMB * 1024) / defaultMemoryLimitMB
	if frames < 256 {
		frames = 256
	}
	return frames
}

// appendBoundedStdout appends string s to stdout while respecting a byte limit. If the limit is zero or negative, s is appended without restriction. If already truncated, the call is ignored. When appending s would exceed the limit, the function writes what fits, appends a truncation marker, and sets *truncated to true.
func appendBoundedStdout(stdout *strings.Builder, s string, limit int, truncated *bool) {
	if limit <= 0 {
		stdout.WriteString(s)
		return
	}
	if *truncated {
		return
	}
	remaining := limit - stdout.Len()
	if remaining <= 0 {
		stdout.WriteString(stdoutTruncationMarker)
		*truncated = true
		return
	}
	if len(s) <= remaining {
		stdout.WriteString(s)
		return
	}
	stdout.WriteString(s[:remaining])
	stdout.WriteString(stdoutTruncationMarker)
	*truncated = true
}

func (p *Pool) touchProgramLRULocked(hash string) {
	if p.cacheAccessLRU == nil {
		p.cacheAccessLRU = list.New()
	}
	if p.cacheLRUByHash == nil {
		p.cacheLRUByHash = make(map[string]*list.Element)
	}
	if elem, ok := p.cacheLRUByHash[hash]; ok {
		p.cacheAccessLRU.MoveToFront(elem)
		return
	}
	p.cacheLRUByHash[hash] = p.cacheAccessLRU.PushFront(hash)
}

func (p *Pool) evictLRULocked() {
	if p.cacheAccessLRU == nil {
		return
	}
	oldest := p.cacheAccessLRU.Back()
	if oldest == nil {
		return
	}
	hash, _ := oldest.Value.(string)
	p.cacheAccessLRU.Remove(oldest)
	delete(p.cacheLRUByHash, hash)
	delete(p.programs, hash)
}

// stringFromMap safely extracts a string from a map[string]interface{}.
func stringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// buildRequestObject creates the JS request object passed to the handler.
func buildRequestObject(vm *goja.Runtime, req Request) goja.Value {
	reqObj := vm.NewObject()
	_ = reqObj.Set("method", req.Method)
	_ = reqObj.Set("path", req.Path)
	if req.Headers != nil {
		_ = reqObj.Set("headers", req.Headers)
	}
	if req.Query != "" {
		_ = reqObj.Set("query", req.Query)
	}
	if req.Body != nil {
		_ = reqObj.Set("body", string(req.Body))
	}
	return reqObj
}
