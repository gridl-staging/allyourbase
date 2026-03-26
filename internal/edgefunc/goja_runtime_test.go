package edgefunc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestGojaRuntime_Execute_SimpleHandler(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `
function handler(request) {
	return {
		statusCode: 200,
		headers: {"Content-Type": ["application/json"]},
		body: JSON.stringify({message: "hello", method: request.method, path: request.path})
	};
}
`
	req := Request{Method: "GET", Path: "/test"}
	resp, err := rt.Execute(context.Background(), code, "handler", req)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), `"method":"GET"`))
	testutil.True(t, strings.Contains(string(resp.Body), `"path":"/test"`))
}

func TestGojaRuntime_Execute_PostWithBody(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `
function handler(request) {
	var parsed = JSON.parse(request.body);
	return {
		statusCode: 201,
		body: JSON.stringify({received: parsed.name})
	};
}
`
	req := Request{
		Method: "POST",
		Path:   "/users",
		Body:   []byte(`{"name":"alice"}`),
	}
	resp, err := rt.Execute(context.Background(), code, "handler", req)
	testutil.NoError(t, err)
	testutil.Equal(t, 201, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), `"received":"alice"`))
}

func TestGojaRuntime_Execute_StdoutCapture(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `
function handler(request) {
	console.log("debug: processing request");
	return { statusCode: 200, body: "ok" };
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.Contains(resp.Stdout, "debug: processing request"))
}

func TestGojaRuntime_Execute_Timeout(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `
function handler(request) {
	while(true) {} // infinite loop
	return { statusCode: 200, body: "never" };
}
`
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := rt.Execute(ctx, code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected timeout error, got nil")
	testutil.True(t, strings.Contains(err.Error(), "timeout"), "expected timeout in error, got: %s", err)
}

func TestGojaRuntime_Execute_SyntaxError(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `function handler(request { return {}; }` // missing closing paren
	_, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected syntax error, got nil")
}

func TestGojaRuntime_Execute_MissingHandler(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `var x = 42;` // no handler function
	_, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected missing handler error, got nil")
}

func TestGojaRuntime_Execute_HandlerThrows(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `
function handler(request) {
	throw new Error("boom");
}
`
	_, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected error from thrown exception, got nil")
	testutil.True(t, strings.Contains(err.Error(), "boom"))
}

func TestGojaRuntime_Execute_DefaultStatusCode(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `
function handler(request) {
	return { body: "no status code set" };
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
}

func TestGojaRuntime_Execute_ConsoleLogMultiArg(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `
function handler(request) {
	console.log("user:", "alice", 42);
	return { statusCode: 200, body: "ok" };
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, "user: alice 42\n", resp.Stdout)
}

func TestGojaRuntime_Execute_ObjectBodySerializedAsJSON(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	// When handler returns a non-string body (object), gojaResultToResponse should JSON-marshal it.
	code := `
function handler(request) {
	return {
		statusCode: 200,
		body: {key: "value", num: 7},
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), `"key":"value"`))
	testutil.True(t, strings.Contains(string(resp.Body), `"num":7`))
}

func TestGojaRuntime_Execute_StringHeaders(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	// gojaResultToResponse handles both string and []string header values.
	code := `
function handler(request) {
	return {
		statusCode: 200,
		headers: {"X-Single": "one", "X-Multi": ["a", "b"]},
		body: "ok",
	};
}
`
	resp, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "one", resp.Headers["X-Single"][0])
	testutil.Equal(t, 2, len(resp.Headers["X-Multi"]))
	testutil.Equal(t, "a", resp.Headers["X-Multi"][0])
	testutil.Equal(t, "b", resp.Headers["X-Multi"][1])
}

func TestGojaRuntime_Execute_CustomEntryPoint(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `
function main(request) {
	return { statusCode: 200, body: "from main" };
}
`
	resp, err := rt.Execute(context.Background(), code, "main", Request{Method: "GET", Path: "/"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "from main", string(resp.Body))
}

func TestGojaRuntime_Execute_CustomEntryPointMissing(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	// Defines "handler" but runtime looks for "main".
	code := `function handler(request) { return { statusCode: 200 }; }`
	_, err := rt.Execute(context.Background(), code, "main", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected error for missing entry point")
	testutil.True(t, strings.Contains(err.Error(), "'main'"),
		"error should mention the entry point name, got: %s", err)
}

func TestGojaRuntime_Execute_AsyncHandlerRejects(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	// Async handler that rejects — the Promise unwrap path should surface
	// the rejection as a Go error.
	code := `
async function handler(request) {
	throw new Error("async boom");
}
`
	_, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected error from rejected async handler")
	testutil.True(t, strings.Contains(err.Error(), "async boom"),
		"expected rejection reason in error, got: %s", err)
}

func TestGojaRuntime_Execute_HandlerReturnsNonObject(t *testing.T) {
	t.Parallel()
	rt := NewGojaRuntime()

	code := `
function handler(request) {
	return "just a string";
}
`
	_, err := rt.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"})
	testutil.True(t, err != nil, "expected error when handler returns non-object")
	testutil.True(t, strings.Contains(err.Error(), "object"),
		"expected 'object' in error, got: %s", err)
}

func TestGojaRuntime_ImplementsRuntimeInterface(t *testing.T) {
	t.Parallel()
	var _ Runtime = NewGojaRuntime()
}
