package edgefunc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewPool(t *testing.T) {
	t.Parallel()

	pool := NewPool(4)
	defer pool.Close()

	testutil.NotNil(t, pool)
	testutil.Equal(t, 4, pool.Size())
}

func TestPool_Execute_SimpleHandler(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code := `function handler(request) {
		return { statusCode: 200, body: "hello from pool" };
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "hello from pool", string(resp.Body))
}

func TestPool_Execute_ConcurrentExecution(t *testing.T) {
	t.Parallel()
	pool := NewPool(4)
	defer pool.Close()

	code := `function handler(request) {
		return { statusCode: 200, body: "worker:" + request.path };
	}`

	const goroutines = 20
	var wg sync.WaitGroup
	errors := make(chan error, goroutines)
	responses := make(chan Response, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			req := Request{Method: "GET", Path: fmt.Sprintf("/%d", n)}
			resp, err := pool.Execute(context.Background(), code, "handler", req, nil, nil)
			if err != nil {
				errors <- err
				return
			}
			responses <- resp
		}(i)
	}

	wg.Wait()
	close(errors)
	close(responses)

	for err := range errors {
		t.Fatalf("concurrent execution error: %v", err)
	}

	count := 0
	for resp := range responses {
		testutil.Equal(t, 200, resp.StatusCode)
		testutil.True(t, strings.HasPrefix(string(resp.Body), "worker:/"))
		count++
	}
	testutil.Equal(t, goroutines, count)
}

func TestPool_Execute_TimeoutEnforcement(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code := `function handler(request) {
		while(true) {} // infinite loop
		return { statusCode: 200, body: "never" };
	}`

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := pool.Execute(ctx, code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.True(t, err != nil, "expected timeout error")
	testutil.True(t, strings.Contains(err.Error(), "timeout"), "expected timeout in error, got: %s", err)
}

func TestPool_Execute_VMReturnedAfterTimeout(t *testing.T) {
	t.Parallel()
	pool := NewPool(1) // single VM pool
	defer pool.Close()

	// First: timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	code := `function handler(request) { while(true) {} }`
	_, _ = pool.Execute(ctx, code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)

	// Second: should succeed (semaphore slot was released, fresh VM works)
	code2 := `function handler(request) { return { statusCode: 200, body: "recovered" }; }`
	resp, err := pool.Execute(context.Background(), code2, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "recovered", string(resp.Body))
}

func TestPool_Compile_CachesProgram(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code := `function handler(request) { return { statusCode: 200, body: "cached" }; }`

	prog1, err := pool.Compile("test-fn", code)
	testutil.NoError(t, err)
	testutil.NotNil(t, prog1)

	prog2, err := pool.Compile("test-fn-2", code)
	testutil.NoError(t, err)
	testutil.NotNil(t, prog2)

	// Same code should return same program (content-hash based cache)
	testutil.True(t, prog1 == prog2, "expected same *goja.Program for identical code")
}

func TestPool_Compile_DifferentCodeDifferentProgram(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code1 := `function handler(request) { return { statusCode: 200, body: "v1" }; }`
	code2 := `function handler(request) { return { statusCode: 200, body: "v2" }; }`

	prog1, err := pool.Compile("fn1", code1)
	testutil.NoError(t, err)

	prog2, err := pool.Compile("fn2", code2)
	testutil.NoError(t, err)

	testutil.True(t, prog1 != prog2, "expected different *goja.Program for different code")
}

func TestPool_Execute_WithCompiledProgram(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code := `function handler(request) { return { statusCode: 200, body: "from program" }; }`

	prog, err := pool.Compile("test", code)
	testutil.NoError(t, err)

	resp, err := pool.ExecuteProgram(context.Background(), prog, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "from program", string(resp.Body))
}

func TestPool_Execute_IsolationBetweenInvocations(t *testing.T) {
	t.Parallel()
	pool := NewPool(1) // single VM to ensure reuse
	defer pool.Close()

	// First invocation sets a global
	code1 := `
var leaked = "secret";
function handler(request) { return { statusCode: 200, body: "set" }; }
`
	_, err := pool.Execute(context.Background(), code1, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)

	// Second invocation should NOT see the global from first invocation
	// because each invocation gets a fresh VM
	code2 := `function handler(request) {
		var result = typeof leaked === "undefined" ? "isolated" : "leaked:" + leaked;
		return { statusCode: 200, body: result };
	}`
	resp, err := pool.Execute(context.Background(), code2, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "isolated", string(resp.Body))
}

func TestPool_Execute_EnvVarInjection(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code := `function handler(request) {
		var apiKey = ayb.env.get("API_KEY");
		var missing = ayb.env.get("NONEXISTENT");
		return { statusCode: 200, body: apiKey + "|" + String(missing) };
	}`

	envVars := map[string]string{
		"API_KEY": "sk-test-123",
	}
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, envVars, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "sk-test-123|undefined", string(resp.Body))
}

func TestPool_Execute_EnvVarIsolationBetweenInvocations(t *testing.T) {
	t.Parallel()
	pool := NewPool(1) // single VM to force reuse
	defer pool.Close()

	code := `function handler(request) {
		var val = ayb.env.get("SECRET");
		return { statusCode: 200, body: String(val) };
	}`

	// First invocation with env vars
	env1 := map[string]string{"SECRET": "fn1-secret"}
	resp1, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, env1, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "fn1-secret", string(resp1.Body))

	// Second invocation with different env vars — should NOT see fn1's vars
	env2 := map[string]string{"SECRET": "fn2-secret"}
	resp2, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, env2, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "fn2-secret", string(resp2.Body))

	// Third invocation with NO env vars — should get undefined
	resp3, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "undefined", string(resp3.Body))
}

func TestPool_Execute_ConsoleLogCapture(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code := `function handler(request) {
		console.log("hello", "world");
		return { statusCode: 200, body: "ok" };
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "hello world\n", resp.Stdout)
}

func TestPool_Execute_FetchBridge(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	// Verify fetch is available (will fail to connect but should not panic)
	code := `function handler(request) {
		try {
			var resp = fetch("http://127.0.0.1:1/nonexistent");
			return { statusCode: 200, body: "should not reach" };
		} catch(e) {
			return { statusCode: 200, body: "fetch available: " + e.message.substring(0, 5) };
		}
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.HasPrefix(string(resp.Body), "fetch available:"))
}

func TestPool_Execute_FetchBridge_SSRFBlocksLoopback(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code := `function handler(request) {
		try {
			fetch("http://127.0.0.1:80/");
			return { statusCode: 200, body: "should not reach" };
		} catch(e) {
			return { statusCode: 200, body: e.message };
		}
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), "ssrf"), "expected ssrf marker in error, got: %s", resp.Body)
	testutil.True(t, strings.Contains(string(resp.Body), "blocked"), "expected blocked marker in error, got: %s", resp.Body)
	testutil.False(t, strings.Contains(strings.ToLower(string(resp.Body)), "connection refused"), "expected SSRF block before connect, got: %s", resp.Body)
}

func TestPool_Execute_FetchBridge_SSRFBlocksMetadata(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code := `function handler(request) {
		try {
			fetch("http://169.254.169.254/latest/meta-data/");
			return { statusCode: 200, body: "should not reach" };
		} catch(e) {
			return { statusCode: 200, body: e.message };
		}
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), "ssrf"), "expected ssrf marker in error, got: %s", resp.Body)
	testutil.True(t, strings.Contains(string(resp.Body), "blocked"), "expected blocked marker in error, got: %s", resp.Body)
}

func TestPool_Execute_AybAuthGetProviderToken(t *testing.T) {
	t.Parallel()
	const expectedUserID = "00000000-0000-0000-0000-000000000001"
	const expectedProvider = "google"

	called := false
	pool := NewPool(
		2,
		WithPoolProviderTokenGetter(func(_ context.Context, userID, provider string) (string, error) {
			called = true
			testutil.Equal(t, expectedUserID, userID)
			testutil.Equal(t, expectedProvider, provider)
			return "refreshed-token", nil
		}),
	)
	defer pool.Close()

	code := `function handler(request) {
		var token = ayb.auth.getProviderToken("%s", "%s");
		return { statusCode: 200, body: token };
	}`

	resp, err := pool.Execute(context.Background(), fmt.Sprintf(code, expectedUserID, expectedProvider), "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "refreshed-token", string(resp.Body))
	testutil.True(t, called)
}

func TestPool_Execute_AybAuthGetProviderTokenUnavailable(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code := `function handler(request) {
		var token = ayb.auth.getProviderToken("user-id", "google");
		return { statusCode: 200, body: String(token) };
	}`

	_, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.True(t, err != nil)
	testutil.True(t, strings.Contains(err.Error(), "ayb.auth.getProviderToken"))
}

func TestPool_Close_Idempotent(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)

	pool.Close()
	pool.Close() // should not panic
}

// Verify pool handles concurrent Compile + Execute without races.
func TestPool_ConcurrentCompileAndExecute(t *testing.T) {
	t.Parallel()
	pool := NewPool(4)
	defer pool.Close()

	var wg sync.WaitGroup
	var errCount atomic.Int64

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			code := fmt.Sprintf(`function handler(request) { return { statusCode: 200, body: "%d" }; }`, n)
			prog, err := pool.Compile(fmt.Sprintf("fn-%d", n), code)
			if err != nil {
				errCount.Add(1)
				return
			}
			_, err = pool.ExecuteProgram(context.Background(), prog, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
			if err != nil {
				errCount.Add(1)
			}
		}(i)
	}

	wg.Wait()
	testutil.Equal(t, int64(0), errCount.Load())
}

// Unexported helper test
func TestContentHash(t *testing.T) {
	t.Parallel()
	h1 := contentHash("abc")
	h2 := contentHash("abc")
	h3 := contentHash("def")
	testutil.Equal(t, h1, h2)
	testutil.True(t, h1 != h3)
}

// Verifies Pool satisfies any expected interfaces or contracts
func TestPool_ExecuteWithDBBridge(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	// Reuse mockQueryExecutor from db_bridge_test.go
	qe := &mockQueryExecutor{
		result: QueryResult{
			Rows: []map[string]interface{}{
				{"id": 1, "name": "alice"},
			},
		},
	}

	code := `function handler(request) {
		var rows = ayb.db.from("users").select("*").execute();
		return { statusCode: 200, body: JSON.stringify(rows) };
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, qe)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.True(t, strings.Contains(string(resp.Body), "alice"))
}

// Verify pool enforces strict mode via "use strict" prefix
func TestPool_Execute_StrictMode(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	// In strict mode, `this` at top level inside an IIFE is undefined (not global).
	// Also, undeclared variable assignment throws in strict mode.
	code := `function handler(request) {
		try {
			undeclaredVar = 42; // should throw in strict mode
			return { statusCode: 200, body: "not strict" };
		} catch(e) {
			return { statusCode: 200, body: "strict mode active" };
		}
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "strict mode active", string(resp.Body))
}

// Verify Execute fails after Close.
func TestPool_Execute_AfterClose(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	pool.Close()

	code := `function handler(request) { return { statusCode: 200, body: "nope" }; }`
	_, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.True(t, err != nil, "expected error after pool closed")
	testutil.True(t, strings.Contains(err.Error(), "closed"), "expected 'closed' in error, got: %s", err)
}

// Verify Compile returns meaningful error for invalid JS.
func TestPool_Compile_SyntaxError(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	_, err := pool.Compile("bad", `function handler( { broken }`)
	testutil.True(t, err != nil, "expected compile error for invalid JS")
	testutil.True(t, strings.Contains(err.Error(), "compiling"), "expected 'compiling' in error, got: %s", err)
}

// Verify that async handlers work through pool
func TestPool_Execute_AsyncHandler(t *testing.T) {
	t.Parallel()
	pool := NewPool(2)
	defer pool.Close()

	code := `async function handler(request) {
		return { statusCode: 200, body: "async works" };
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "async works", string(resp.Body))
}

func TestPool_TryAcquire_ConcurrencyLimitExceededImmediate(t *testing.T) {
	t.Parallel()
	pool := NewPool(1, WithPoolMaxConcurrentInvocations(1))
	defer pool.Close()

	testutil.NoError(t, pool.TryAcquire(context.Background()))
	defer pool.releaseInvocation()

	start := time.Now()
	err := pool.TryAcquire(context.Background())
	elapsed := time.Since(start)

	testutil.True(t, errors.Is(err, ErrConcurrencyLimitExceeded), "expected ErrConcurrencyLimitExceeded, got: %v", err)
	testutil.True(t, elapsed < 50*time.Millisecond, "expected immediate rejection, took %s", elapsed)
}

func TestPool_Execute_UnderAdmissionCap(t *testing.T) {
	t.Parallel()
	pool := NewPool(1, WithPoolMaxConcurrentInvocations(2))
	defer pool.Close()

	code := `function handler(request) { return { statusCode: 200, body: "ok" }; }`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "ok", string(resp.Body))
}

func TestPool_Execute_OverAdmissionCapFailsFast(t *testing.T) {
	t.Parallel()
	pool := NewPool(1, WithPoolMaxConcurrentInvocations(1))
	defer pool.Close()

	blockingCode := `function handler(request) { while(true) {} }`
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		_, _ = pool.Execute(ctx, blockingCode, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	}()

	time.Sleep(40 * time.Millisecond)

	start := time.Now()
	_, err := pool.Execute(context.Background(), blockingCode, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	elapsed := time.Since(start)
	testutil.True(t, errors.Is(err, ErrConcurrencyLimitExceeded), "expected ErrConcurrencyLimitExceeded, got: %v", err)
	testutil.True(t, elapsed < 50*time.Millisecond, "expected immediate cap rejection, took %s", elapsed)

	<-firstDone
}

func TestPool_Compile_EvictsLeastRecentlyUsed(t *testing.T) {
	t.Parallel()
	pool := NewPool(1, WithPoolCodeCacheSize(2))
	defer pool.Close()

	codeA := `function handler(request) { return { statusCode: 200, body: "a" }; }`
	codeB := `function handler(request) { return { statusCode: 200, body: "b" }; }`
	codeC := `function handler(request) { return { statusCode: 200, body: "c" }; }`

	_, err := pool.Compile("a", codeA)
	testutil.NoError(t, err)
	_, err = pool.Compile("b", codeB)
	testutil.NoError(t, err)
	_, err = pool.Compile("c", codeC)
	testutil.NoError(t, err)

	pool.cacheMu.RLock()
	_, hasA := pool.programs[contentHash(codeA)]
	_, hasB := pool.programs[contentHash(codeB)]
	_, hasC := pool.programs[contentHash(codeC)]
	cacheLen := len(pool.programs)
	pool.cacheMu.RUnlock()

	testutil.Equal(t, 2, cacheLen)
	testutil.False(t, hasA, "oldest entry should be evicted")
	testutil.True(t, hasB)
	testutil.True(t, hasC)
}

func TestPool_Compile_HitPromotesEntryForEviction(t *testing.T) {
	t.Parallel()
	pool := NewPool(1, WithPoolCodeCacheSize(2))
	defer pool.Close()

	codeA := `function handler(request) { return { statusCode: 200, body: "a" }; }`
	codeB := `function handler(request) { return { statusCode: 200, body: "b" }; }`
	codeC := `function handler(request) { return { statusCode: 200, body: "c" }; }`

	_, err := pool.Compile("a", codeA)
	testutil.NoError(t, err)
	_, err = pool.Compile("b", codeB)
	testutil.NoError(t, err)
	_, err = pool.Compile("a-hit", codeA) // hit A so B becomes LRU
	testutil.NoError(t, err)
	_, err = pool.Compile("c", codeC)
	testutil.NoError(t, err)

	pool.cacheMu.RLock()
	_, hasA := pool.programs[contentHash(codeA)]
	_, hasB := pool.programs[contentHash(codeB)]
	_, hasC := pool.programs[contentHash(codeC)]
	pool.cacheMu.RUnlock()

	testutil.True(t, hasA, "recently hit entry should be retained")
	testutil.False(t, hasB, "least-recently-used entry should be evicted")
	testutil.True(t, hasC)
}

func TestPool_Execute_DeepRecursionHitsStackLimit(t *testing.T) {
	t.Parallel()
	pool := NewPool(1, WithPoolMemoryLimitMB(1))
	defer pool.Close()

	code := `function recurse(){ return recurse(); } function handler(){ recurse(); return {statusCode:200, body:"never"}; }`
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := pool.Execute(ctx, code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.True(t, err != nil, "expected recursion error")
	testutil.True(t,
		!strings.Contains(strings.ToLower(err.Error()), "timeout"),
		"expected recursion guard error before timeout, got: %v",
		err,
	)
	testutil.True(t,
		strings.Contains(strings.ToLower(err.Error()), "recurse"),
		"expected recursion callsite in error, got: %v",
		err,
	)
}

func TestPool_Execute_StdoutTruncation(t *testing.T) {
	t.Parallel()
	pool := NewPool(1, WithPoolMemoryLimitMB(1))
	defer pool.Close()

	code := `function handler() {
		for (var i = 0; i < 50000; i++) { console.log("line-" + i + "-abcdefghijklmnopqrstuvwxyz"); }
		return { statusCode: 200, body: "ok" };
	}`

	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.True(t, strings.Contains(resp.Stdout, "[stdout truncated]"), "expected truncation marker in stdout")
}

func TestPool_SandboxRequireUndefined(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer pool.Close()

	code := `function handler(){ return { statusCode: 200, body: String(typeof require) }; }`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "undefined", string(resp.Body))
}

func TestPool_SandboxImportStatementFails(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer pool.Close()

	code := `import x from "y"; function handler(){ return { statusCode: 200, body: "ok" }; }`
	_, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.True(t, err != nil, "expected syntax error for import statement")
}

func TestPool_SandboxProcessUndefined(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer pool.Close()

	code := `function handler(){ return { statusCode: 200, body: String(typeof process) }; }`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "undefined", string(resp.Body))
}

func TestPool_SandboxDirnameFilenameUndefined(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer pool.Close()

	code := `function handler(){ return { statusCode: 200, body: String(typeof __dirname) + "|" + String(typeof __filename) }; }`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "undefined|undefined", string(resp.Body))
}

func TestPool_SandboxEvalStrictScope(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer pool.Close()

	code := `function handler(){
		var value = "outer";
		eval("var value = 'inner';");
		return { statusCode: 200, body: value };
	}`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "outer", string(resp.Body))
}

func TestPool_SandboxFunctionConstructorStrictNoGlobalEscape(t *testing.T) {
	t.Parallel()
	pool := NewPool(1)
	defer pool.Close()

	code := `function handler(){
		var globalThisRef = Function('"use strict"; return this')();
		return { statusCode: 200, body: String(typeof globalThisRef) };
	}`
	resp, err := pool.Execute(context.Background(), code, "handler", Request{Method: "GET", Path: "/"}, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "undefined", string(resp.Body))
}
