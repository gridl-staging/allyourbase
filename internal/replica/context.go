package replica

import (
	"context"
	"sync"
)

type routingCtxKey struct{}

// RoutingState carries request-scoped routing flags.
type RoutingState struct {
	ForcePrimary bool
	HasWritten   bool
	mu           sync.Mutex
}

func WithRoutingState(ctx context.Context, state *RoutingState) context.Context {
	return context.WithValue(ctx, routingCtxKey{}, state)
}

func RoutingStateFromContext(ctx context.Context) *RoutingState {
	state, _ := ctx.Value(routingCtxKey{}).(*RoutingState)
	return state
}

func IsReadOnly(ctx context.Context) bool {
	state := RoutingStateFromContext(ctx)
	if state == nil {
		return false
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	return !state.ForcePrimary && !state.HasWritten
}

func MarkWrite(ctx context.Context) {
	state := RoutingStateFromContext(ctx)
	if state == nil {
		return
	}

	state.mu.Lock()
	state.HasWritten = true
	state.mu.Unlock()
}
