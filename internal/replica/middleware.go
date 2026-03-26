// Package replica This file provides HTTP middleware for routing database requests to replicas based on read consistency requirements and request method.
package replica

import (
	"net/http"
	"strings"
)

const readConsistencyHeader = "X-Read-Consistency"

// ReplicaRoutingMiddleware returns HTTP middleware that routes requests based on replica availability and read consistency requirements. When the pool router is unavailable or has no replicas, a pass-through middleware is returned. Otherwise, the middleware attaches a RoutingState to the request context with ForcePrimary set based on the X-Read-Consistency header and request method, ensuring mutations and strong-consistency reads route to the primary.
func ReplicaRoutingMiddleware(router *PoolRouter) func(http.Handler) http.Handler {
	if router == nil || !router.HasReplicas() {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := &RoutingState{
				ForcePrimary: shouldForcePrimary(r),
			}
			ctx := WithRoutingState(r.Context(), state)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func shouldForcePrimary(r *http.Request) bool {
	if strings.EqualFold(strings.TrimSpace(r.Header.Get(readConsistencyHeader)), "strong") {
		return true
	}

	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
