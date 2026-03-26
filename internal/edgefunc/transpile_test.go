package edgefunc

import (
	"context"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/dop251/goja"
)

func TestTranspile_TypeScriptToES2015(t *testing.T) {
	t.Parallel()

	source := `
		function handler(input: string): string {
			return input.toUpperCase();
		}
	`

	compiled, err := Transpile(source, true, "handler")
	testutil.NoError(t, err)
	testutil.True(t, !strings.Contains(compiled, ": string"), "expected type annotations to be removed")

	_, compileErr := goja.Compile("transpiled.ts", compiled, false)
	testutil.NoError(t, compileErr)
}

func TestTranspile_JavaScriptPassThrough(t *testing.T) {
	t.Parallel()

	source := `function handler(request) { return { statusCode: 200, body: request.path }; }`
	compiled, err := Transpile(source, false, "")
	testutil.NoError(t, err)
	testutil.Equal(t, source, compiled)
}

func TestTranspile_SyntaxErrorHasUsefulMessage(t *testing.T) {
	t.Parallel()

	source := `function handler(input: string) { return input.`
	_, err := Transpile(source, true, "handler")
	testutil.True(t, err != nil, "expected transpile error")
	testutil.True(t, strings.Contains(err.Error(), "transpile"), "expected error to identify transpilation")
	testutil.True(t, strings.Contains(err.Error(), "Expected"), "expected parser message in transpile error")
}

func TestTranspile_AsyncAwaitLoweredForES2015(t *testing.T) {
	t.Parallel()

	source := `
		async function handler() {
			await Promise.resolve();
			return "ok";
		}
	`

	compiled, err := Transpile(source, true, "handler")
	testutil.NoError(t, err)
	testutil.True(t, !strings.Contains(compiled, "async function"), "expected async function syntax to be lowered")
}

func TestTranspile_AsyncAwaitExecutesInGoja(t *testing.T) {
	t.Parallel()

	// Verify that transpiled async/await actually runs end-to-end in Goja.
	// esbuild lowers async/await to generators+Promise; Goja must support both.
	source := `
		async function handler(request: { path: string }) {
			var val = await Promise.resolve("resolved");
			return { statusCode: 200, body: val + ":" + request.path };
		}
	`

	compiled, err := Transpile(source, true, "handler")
	testutil.NoError(t, err)

	rt := NewGojaRuntime()
	resp, execErr := rt.Execute(context.Background(), compiled, "handler", Request{Method: "GET", Path: "/async"})
	if execErr != nil {
		t.Fatalf("transpiled async/await failed to execute in Goja: %v\nCompiled output:\n%s", execErr, compiled)
	}
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "resolved:/async", string(resp.Body))
}

func TestTranspile_TypeScriptOutputExecutesWithRuntimeHandlerContract(t *testing.T) {
	t.Parallel()

	source := `
		function handler(request: { path: string }) {
			return { statusCode: 200, body: request.path };
		}
	`

	compiled, err := Transpile(source, true, "handler")
	testutil.NoError(t, err)

	rt := NewGojaRuntime()
	resp, execErr := rt.Execute(context.Background(), compiled, "handler", Request{Method: "GET", Path: "/ok"})
	testutil.NoError(t, execErr)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "/ok", string(resp.Body))
}

func TestTranspile_CustomEntryPointExportedToGlobal(t *testing.T) {
	t.Parallel()

	source := `
		function main(request: { path: string }) {
			return { statusCode: 200, body: "main:" + request.path };
		}
	`

	compiled, err := Transpile(source, true, "main")
	testutil.NoError(t, err)

	rt := NewGojaRuntime()
	resp, execErr := rt.Execute(context.Background(), compiled, "main", Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, execErr)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "main:/test", string(resp.Body))
}
