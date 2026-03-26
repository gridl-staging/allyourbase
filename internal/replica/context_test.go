package replica

import (
	"context"
	"testing"
)

func TestWithRoutingStateRoundTrip(t *testing.T) {
	state := &RoutingState{ForcePrimary: false}
	ctx := WithRoutingState(context.Background(), state)

	got := RoutingStateFromContext(ctx)
	if got == nil {
		t.Fatal("RoutingStateFromContext() returned nil")
	}
	if got != state {
		t.Fatalf("RoutingStateFromContext() = %p, want %p", got, state)
	}
}

func TestIsReadOnlyReturnsFalseWhenForcePrimary(t *testing.T) {
	ctx := WithRoutingState(context.Background(), &RoutingState{ForcePrimary: true})
	if IsReadOnly(ctx) {
		t.Fatal("IsReadOnly() = true, want false when ForcePrimary is set")
	}
}

func TestIsReadOnlyReturnsFalseAfterMarkWrite(t *testing.T) {
	ctx := WithRoutingState(context.Background(), &RoutingState{})
	if !IsReadOnly(ctx) {
		t.Fatal("IsReadOnly() = false, want true before MarkWrite")
	}

	MarkWrite(ctx)
	if IsReadOnly(ctx) {
		t.Fatal("IsReadOnly() = true, want false after MarkWrite")
	}
}

func TestRoutingStateFromContextReturnsNilOnBareContext(t *testing.T) {
	if got := RoutingStateFromContext(context.Background()); got != nil {
		t.Fatalf("RoutingStateFromContext() = %v, want nil", got)
	}
}
