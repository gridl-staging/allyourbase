// Package edgefunc GojaRuntime implements the Runtime interface using the Goja pure-Go JavaScript interpreter, providing execution of edge functions with support for timeouts, console output, fetch, and database queries.
package edgefunc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dop251/goja"
)

// GojaOption configures a GojaRuntime.
type GojaOption func(*GojaRuntime)

// WithHTTPClient sets the HTTP client used for fetch() calls.
// If not set, http.DefaultClient is used.
func WithHTTPClient(c *http.Client) GojaOption {
	return func(r *GojaRuntime) { r.httpClient = c }
}

// GojaRuntime implements Runtime using the Goja pure-Go JS interpreter.
type GojaRuntime struct {
	httpClient    *http.Client
	queryExecutor QueryExecutor
}

// NewGojaRuntime returns a GojaRuntime that satisfies the Runtime interface.
func NewGojaRuntime(opts ...GojaOption) *GojaRuntime {
	r := &GojaRuntime{httpClient: http.DefaultClient}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Execute runs JavaScript code in a Goja VM and calls the specified entry point function with the request. The runtime provides console.log capture, a fetch() bridge for HTTP calls, and database access. Execution is interrupted if the context is cancelled. The entry point defaults to "handler" if not specified. Async handlers returning Promises are awaited before returning.
func (g *GojaRuntime) Execute(ctx context.Context, code string, entryPoint string, request Request) (Response, error) {
	if entryPoint == "" {
		entryPoint = "handler"
	}
	vm := goja.New()

	// Wire context cancellation to VM interrupt for timeout support.
	// The done channel ensures the goroutine exits when Execute returns
	// normally, preventing goroutine/VM leaks for long-lived contexts.
	if ctx.Done() != nil {
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				vm.Interrupt("execution timeout")
			case <-done:
			}
		}()
	}

	// Capture console.log output.
	var stdout strings.Builder
	console := vm.NewObject()
	if err := console.Set("log", func(call goja.FunctionCall) goja.Value {
		parts := make([]string, len(call.Arguments))
		for i, arg := range call.Arguments {
			parts[i] = arg.String()
		}
		stdout.WriteString(strings.Join(parts, " "))
		stdout.WriteString("\n")
		return goja.Undefined()
	}); err != nil {
		return Response{}, fmt.Errorf("setting console.log: %w", err)
	}
	if err := vm.Set("console", console); err != nil {
		return Response{}, fmt.Errorf("setting console: %w", err)
	}

	// Register fetch() bridge to Go's net/http.
	if err := registerFetch(vm, ctx, g.httpClient); err != nil {
		return Response{}, fmt.Errorf("registering fetch: %w", err)
	}

	// Register ayb.db bridge for database queries (if executor configured).
	if err := registerDBBridge(vm, ctx, g.queryExecutor); err != nil {
		return Response{}, fmt.Errorf("registering db bridge: %w", err)
	}

	// Run user code to define the handler function.
	if _, err := vm.RunString(code); err != nil {
		return Response{}, fmt.Errorf("compiling edge function: %w", err)
	}

	handler, ok := goja.AssertFunction(vm.Get(entryPoint))
	if !ok {
		return Response{}, fmt.Errorf("edge function must export a '%s' function", entryPoint)
	}

	// Build request object for JS.
	reqObj := vm.NewObject()
	_ = reqObj.Set("method", request.Method)
	_ = reqObj.Set("path", request.Path)
	if request.Headers != nil {
		_ = reqObj.Set("headers", request.Headers)
	}
	if request.Body != nil {
		_ = reqObj.Set("body", string(request.Body))
	}

	// Call handler(request).
	result, err := handler(goja.Undefined(), reqObj)
	if err != nil {
		return Response{}, fmt.Errorf("executing handler: %w", err)
	}

	// Unwrap Promise returns (from async handlers). Goja drains the
	// microtask queue after the top-level call, so the Promise is
	// already settled by the time we reach here.
	result, err = unwrapPromise(result)
	if err != nil {
		return Response{}, err
	}

	return gojaResultToResponse(result, stdout.String())
}

// unwrapPromise detects if val is a settled Promise and returns its result.
// Async handlers (transpiled from async/await) return a Promise. Goja drains
// microtasks after top-level calls, so the Promise is already settled.
func unwrapPromise(val goja.Value) (goja.Value, error) {
	obj, ok := val.(*goja.Object)
	if !ok {
		return val, nil
	}
	p, ok := obj.Export().(*goja.Promise)
	if !ok {
		return val, nil
	}
	switch p.State() {
	case goja.PromiseStateFulfilled:
		return p.Result(), nil
	case goja.PromiseStateRejected:
		reason := p.Result()
		// JS Error objects have a .message property; extract it for clear Go errors.
		if obj, ok := reason.(*goja.Object); ok {
			if msg := obj.Get("message"); msg != nil && !goja.IsUndefined(msg) {
				return goja.Null(), fmt.Errorf("executing handler: %s", msg.String())
			}
		}
		return goja.Null(), fmt.Errorf("executing handler: promise rejected: %v", reason.Export())
	default:
		return goja.Null(), fmt.Errorf("executing handler: promise still pending after execution")
	}
}

// gojaResultToResponse converts the JS return value to our Response struct.
func gojaResultToResponse(val goja.Value, stdout string) (Response, error) {
	raw := val.Export()
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return Response{}, errors.New("handler must return an object")
	}

	resp := Response{Stdout: stdout, StatusCode: 200}

	if sc, ok := obj["statusCode"]; ok {
		switch v := sc.(type) {
		case int64:
			resp.StatusCode = int(v)
		case float64:
			resp.StatusCode = int(v)
		}
	}

	if body, ok := obj["body"]; ok {
		switch v := body.(type) {
		case string:
			resp.Body = []byte(v)
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return Response{}, fmt.Errorf("marshaling response body: %w", err)
			}
			resp.Body = b
		}
	}

	if hdrs, ok := obj["headers"]; ok {
		if hdrMap, ok := hdrs.(map[string]interface{}); ok {
			resp.Headers = make(map[string][]string, len(hdrMap))
			for k, v := range hdrMap {
				switch vals := v.(type) {
				case []interface{}:
					strs := make([]string, len(vals))
					for i, s := range vals {
						strs[i] = fmt.Sprint(s)
					}
					resp.Headers[k] = strs
				case string:
					resp.Headers[k] = []string{vals}
				}
			}
		}
	}

	return resp, nil
}
