package edgefunc_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestInvocationDepthContext(t *testing.T) {
	ctx := context.Background()

	// Default depth is 0
	testutil.Equal(t, 0, edgefunc.InvocationDepth(ctx))

	// Set depth
	ctx = edgefunc.WithInvocationDepth(ctx, 3)
	testutil.Equal(t, 3, edgefunc.InvocationDepth(ctx))

	// Set deeper
	ctx = edgefunc.WithInvocationDepth(ctx, 5)
	testutil.Equal(t, 5, edgefunc.InvocationDepth(ctx))
}

func TestParentInvocationIDContext(t *testing.T) {
	ctx := context.Background()

	// Default is empty
	testutil.Equal(t, "", edgefunc.ParentInvocationID(ctx))

	// Set
	ctx = edgefunc.WithParentInvocationID(ctx, "abc-123")
	testutil.Equal(t, "abc-123", edgefunc.ParentInvocationID(ctx))
}

func TestNestedFunctionInvoke_Success(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(4)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	// Deploy a helper function
	_, err := svc.Deploy(context.Background(), "helper", `function handler(req) {
		return { statusCode: 200, body: "helper:" + req.method };
	}`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Deploy a caller function that invokes the helper
	_, err = svc.Deploy(context.Background(), "caller", `function handler(req) {
		var result = ayb.functions.invoke("helper", { method: "POST", path: "/nested" });
		return { statusCode: 200, body: "caller got: " + result.body };
	}`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Invoke caller — it should internally invoke helper
	resp, err := svc.Invoke(context.Background(), "caller", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "caller got: helper:POST", string(resp.Body))
}

func TestNestedFunctionInvoke_ErrorPropagation(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(4)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	// Deploy a function that throws
	_, err := svc.Deploy(context.Background(), "broken", `function handler(req) { throw new Error("boom"); }`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Deploy a caller that invokes broken function
	_, err = svc.Deploy(context.Background(), "caller-err", `function handler(req) {
		var result = ayb.functions.invoke("broken", { method: "GET", path: "/" });
		return { statusCode: 200, body: "should not reach" };
	}`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Invoke should propagate the error from the nested call
	_, err = svc.Invoke(context.Background(), "caller-err", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.True(t, err != nil, "should propagate error from nested invocation")
}

func TestNestedFunctionInvoke_MaxDepthRejection(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	// Pool must be larger than MaxInvocationDepth so the depth guard triggers
	// instead of pool exhaustion causing a 5s deadlock timeout.
	pool := edgefunc.NewPool(edgefunc.MaxInvocationDepth + 2)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	// Deploy a function that recursively calls itself
	_, err := svc.Deploy(context.Background(), "recursive", `function handler(req) {
		var result = ayb.functions.invoke("recursive", { method: "GET", path: "/" });
		return { statusCode: 200, body: "unreachable" };
	}`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Invoke should hit max depth and fail via depth guard, not timeout
	_, err = svc.Invoke(context.Background(), "recursive", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.True(t, err != nil, "should reject at max depth")
	testutil.True(t,
		errors.Is(err, edgefunc.ErrMaxDepthExceeded) || strings.Contains(err.Error(), "maximum invocation depth exceeded"),
		"should fail due to depth guard, not timeout: %v", err)
}

func TestNestedFunctionInvoke_TimeoutBudgetExhaustion(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(4)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "time-bomb", `function handler(req) {
		var result = ayb.functions.invoke("time-bomb", { method: "GET", path: "/" });
		return { statusCode: 200, body: "unreachable" };
	}`, edgefunc.DeployOptions{TimeoutMs: 100})
	testutil.NoError(t, err)

	// With a very tight deadline, nested calls should be rejected
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err = svc.Invoke(ctx, "time-bomb", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.True(t, err != nil, "should reject when time budget is exhausted")
}

func TestNestedFunctionInvoke_NilInvoker(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	// Function invoker bridge explicitly disabled — ayb.functions should not exist.
	svc := edgefunc.NewService(store, pool, logStore, edgefunc.WithServiceFunctionInvoker(false))

	_, err := svc.Deploy(context.Background(), "no-invoker", `function handler(req) {
		if (typeof ayb.functions === 'undefined') {
			return { statusCode: 200, body: "no functions api" };
		}
		return { statusCode: 500, body: "functions api should not exist" };
	}`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	// Without a FunctionInvoker, ayb.functions should be undefined
	resp, err := svc.Invoke(context.Background(), "no-invoker", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	testutil.Equal(t, "no functions api", string(resp.Body))
}

func TestNestedFunctionInvoke_FunctionNotFound(t *testing.T) {
	store := newMockStore()
	logStore := newMockLogStore()
	pool := edgefunc.NewPool(4)
	defer pool.Close()

	svc := edgefunc.NewService(store, pool, logStore)

	_, err := svc.Deploy(context.Background(), "caller-not-found", `function handler(req) {
		var result = ayb.functions.invoke("nonexistent", { method: "GET", path: "/" });
		return { statusCode: 200, body: "unreachable" };
	}`, edgefunc.DeployOptions{})
	testutil.NoError(t, err)

	_, err = svc.Invoke(context.Background(), "caller-not-found", edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.True(t, err != nil, "should error when invoking nonexistent function")
	testutil.True(t,
		errors.Is(err, edgefunc.ErrFunctionNotFound) || strings.Contains(err.Error(), edgefunc.ErrFunctionNotFound.Error()),
		"should preserve not-found cause, got: %v", err)
}
