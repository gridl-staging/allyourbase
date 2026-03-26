package replica

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReplicaRoutingMiddlewareGetWithoutHeaderIsReadOnly(t *testing.T) {
	router := testRouterWithReplica()

	var gotReadOnly bool
	var gotState *RoutingState

	handler := ReplicaRoutingMiddleware(router)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotReadOnly = IsReadOnly(r.Context())
		gotState = RoutingStateFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotState == nil {
		t.Fatal("routing state missing from request context")
	}
	if gotState.ForcePrimary {
		t.Fatal("ForcePrimary = true, want false for GET without strong header")
	}
	if !gotReadOnly {
		t.Fatal("IsReadOnly() = false, want true for GET without strong header")
	}
}

func TestReplicaRoutingMiddlewareStrongHeaderForcesPrimary(t *testing.T) {
	router := testRouterWithReplica()

	var gotReadOnly bool
	var gotState *RoutingState

	handler := ReplicaRoutingMiddleware(router)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotReadOnly = IsReadOnly(r.Context())
		gotState = RoutingStateFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Read-Consistency", "strong")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotState == nil {
		t.Fatal("routing state missing from request context")
	}
	if !gotState.ForcePrimary {
		t.Fatal("ForcePrimary = false, want true for strong consistency header")
	}
	if gotReadOnly {
		t.Fatal("IsReadOnly() = true, want false when strong consistency is requested")
	}
}

func TestReplicaRoutingMiddlewareWriteMethodsForcePrimary(t *testing.T) {
	router := testRouterWithReplica()

	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var gotReadOnly bool
			var gotState *RoutingState

			handler := ReplicaRoutingMiddleware(router)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				gotReadOnly = IsReadOnly(r.Context())
				gotState = RoutingStateFromContext(r.Context())
			}))

			req := httptest.NewRequest(method, "/", nil)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if gotState == nil {
				t.Fatal("routing state missing from request context")
			}
			if !gotState.ForcePrimary {
				t.Fatal("ForcePrimary = false, want true for write method")
			}
			if gotReadOnly {
				t.Fatal("IsReadOnly() = true, want false for write method")
			}
		})
	}
}

func TestReplicaRoutingMiddlewareNoOpWhenRouterNil(t *testing.T) {
	var gotState *RoutingState
	var gotReadOnly bool

	handler := ReplicaRoutingMiddleware(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotState = RoutingStateFromContext(r.Context())
		gotReadOnly = IsReadOnly(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotState != nil {
		t.Fatalf("RoutingStateFromContext() = %v, want nil for no-op middleware", gotState)
	}
	if gotReadOnly {
		t.Fatal("IsReadOnly() = true, want false when middleware is a no-op")
	}
}

func TestReplicaRoutingMiddlewareNoOpWhenRouterHasNoReplicas(t *testing.T) {
	router := NewPoolRouter(nil, nil, nil)

	var gotState *RoutingState
	var gotReadOnly bool

	handler := ReplicaRoutingMiddleware(router)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotState = RoutingStateFromContext(r.Context())
		gotReadOnly = IsReadOnly(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotState != nil {
		t.Fatalf("RoutingStateFromContext() = %v, want nil when router has no replicas", gotState)
	}
	if gotReadOnly {
		t.Fatal("IsReadOnly() = true, want false when middleware is a no-op")
	}
}

func testRouterWithReplica() *PoolRouter {
	var replicaPool pgxpool.Pool
	return NewPoolRouter(nil, []ReplicaPool{{Pool: &replicaPool}}, nil)
}
