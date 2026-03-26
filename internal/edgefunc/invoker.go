package edgefunc

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// MaxInvocationDepth is the default limit for nested function-to-function calls.
const MaxInvocationDepth = 8

// MaxTriggerChainDepth is the maximum depth for trigger→function→trigger chains.
// This prevents indirect recursion cycles (A triggers B triggers A ...).
const MaxTriggerChainDepth = 4

// MinRemainingBudgetMs is the minimum context deadline budget (in ms) required
// to allow a nested invocation.
const MinRemainingBudgetMs = 500

var (
	ErrMaxDepthExceeded          = errors.New("maximum invocation depth exceeded")
	ErrInsufficientTimeBudget    = errors.New("insufficient time budget for nested invocation")
	ErrTriggerChainDepthExceeded = errors.New("maximum trigger chain depth exceeded")
)

// invocationDepthKey is the context key for tracking invocation depth.
type invocationDepthKey struct{}

// parentInvocationKey is the context key for tracking the parent invocation ID.
type parentInvocationKey struct{}

// InvocationDepth returns the current invocation depth from the context.
func InvocationDepth(ctx context.Context) int {
	if v, ok := ctx.Value(invocationDepthKey{}).(int); ok {
		return v
	}
	return 0
}

// WithInvocationDepth returns a context with the invocation depth set.
func WithInvocationDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, invocationDepthKey{}, depth)
}

// ParentInvocationID returns the parent invocation ID from the context, if any.
func ParentInvocationID(ctx context.Context) string {
	if v, ok := ctx.Value(parentInvocationKey{}).(string); ok {
		return v
	}
	return ""
}

// WithParentInvocationID returns a context with the parent invocation ID set.
func WithParentInvocationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, parentInvocationKey{}, id)
}

// checkInvocationBudget validates that the context has enough remaining time
// and the depth limit hasn't been exceeded.
func checkInvocationBudget(ctx context.Context, maxDepth int) error {
	depth := InvocationDepth(ctx)
	if depth >= maxDepth {
		return fmt.Errorf("%w: depth %d >= max %d", ErrMaxDepthExceeded, depth, maxDepth)
	}

	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining.Milliseconds() < int64(MinRemainingBudgetMs) {
			return fmt.Errorf("%w: %dms remaining, need %dms",
				ErrInsufficientTimeBudget, remaining.Milliseconds(), MinRemainingBudgetMs)
		}
	}

	return nil
}

// triggerChainDepthKey is the context key for tracking trigger→function→trigger chain depth.
// This prevents indirect recursion cycles (A triggers B triggers A ...) across all trigger types.
type triggerChainDepthKey struct{}

// TriggerChainDepth returns the current trigger chain depth from the context.
func TriggerChainDepth(ctx context.Context) int {
	if v, ok := ctx.Value(triggerChainDepthKey{}).(int); ok {
		return v
	}
	return 0
}

// WithTriggerChainDepth returns a context with the trigger chain depth set.
func WithTriggerChainDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, triggerChainDepthKey{}, depth)
}

// CheckTriggerChainDepth validates that the trigger chain depth hasn't exceeded the limit.
// Returns a non-nil error if the chain is too deep.
func CheckTriggerChainDepth(ctx context.Context) error {
	depth := TriggerChainDepth(ctx)
	if depth >= MaxTriggerChainDepth {
		return fmt.Errorf("%w: depth %d >= max %d", ErrTriggerChainDepthExceeded, depth, MaxTriggerChainDepth)
	}
	return nil
}
