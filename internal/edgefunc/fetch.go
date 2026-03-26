package edgefunc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dop251/goja"
)

const (
	// maxFetchResponseBytes limits fetch() response bodies to 5 MB to prevent OOM
	// from edge functions fetching arbitrarily large responses.
	maxFetchResponseBytes = 5 * 1024 * 1024
)

// registerFetch injects a synchronous fetch(url, opts?) function into the Goja VM.
// The function bridges to Go's net/http using the provided client and context.
func registerFetch(vm *goja.Runtime, ctx context.Context, client *http.Client) error {
	return vm.Set("fetch", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("fetch requires a URL argument"))
		}

		url := call.Arguments[0].String()
		method := "GET"
		var body io.Reader
		reqHeaders := http.Header{}

		// Parse optional second argument: {method, headers, body}
		if len(call.Arguments) > 1 {
			opts := call.Arguments[1].Export()
			if optsMap, ok := opts.(map[string]interface{}); ok {
				if m, ok := optsMap["method"]; ok {
					method = strings.ToUpper(fmt.Sprint(m))
				}
				if h, ok := optsMap["headers"]; ok {
					if hm, ok := h.(map[string]interface{}); ok {
						for k, v := range hm {
							reqHeaders.Set(k, fmt.Sprint(v))
						}
					}
				}
				if b, ok := optsMap["body"]; ok {
					body = strings.NewReader(fmt.Sprint(b))
				}
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("fetch: %w", err)))
		}
		req.Header = reqHeaders

		resp, err := client.Do(req)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("fetch: %w", err)))
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchResponseBytes+1))
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("fetch: reading response: %w", err)))
		}
		if len(respBody) > maxFetchResponseBytes {
			panic(vm.NewGoError(fmt.Errorf("fetch: response body exceeds %d byte limit", maxFetchResponseBytes)))
		}

		return buildFetchResponse(vm, resp, respBody)
	})
}

// buildFetchResponse creates a JS object mimicking the Fetch API Response interface.
func buildFetchResponse(vm *goja.Runtime, resp *http.Response, body []byte) goja.Value {
	obj := vm.NewObject()
	_ = obj.Set("status", resp.StatusCode)
	_ = obj.Set("statusText", http.StatusText(resp.StatusCode))
	_ = obj.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)

	// headers.get(name) — case-insensitive header lookup.
	headers := vm.NewObject()
	_ = headers.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		v := resp.Header.Get(call.Arguments[0].String())
		if v == "" {
			return goja.Null()
		}
		return vm.ToValue(v)
	})
	_ = obj.Set("headers", headers)

	// text() — return body as string.
	bodyStr := string(body)
	_ = obj.Set("text", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(bodyStr)
	})

	// json() — parse body as JSON and return native JS object.
	_ = obj.Set("json", func(call goja.FunctionCall) goja.Value {
		var parsed interface{}
		if err := json.Unmarshal(body, &parsed); err != nil {
			panic(vm.NewGoError(fmt.Errorf("fetch response json(): %w", err)))
		}
		return vm.ToValue(parsed)
	})

	return obj
}
