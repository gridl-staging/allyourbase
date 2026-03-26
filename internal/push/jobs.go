package push

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/allyourbase/ayb/internal/jobs"
)

type deliveryProcessor interface {
	ProcessDelivery(ctx context.Context, deliveryID string) error
}

type staleTokenCleaner interface {
	CleanupStaleTokens(ctx context.Context, staleDays int) (int64, error)
}

// PushDeliveryJobHandler handles push_delivery jobs by dispatching ProcessDelivery.
func PushDeliveryJobHandler(processor deliveryProcessor) jobs.JobHandler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p struct {
			DeliveryID string `json:"delivery_id"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("push_delivery: invalid payload: %w", err)
		}
		if p.DeliveryID == "" {
			return fmt.Errorf("push_delivery: delivery_id is required")
		}
		if processor == nil {
			return fmt.Errorf("push_delivery: processor not configured")
		}
		return processor.ProcessDelivery(ctx, p.DeliveryID)
	}
}

// PushTokenCleanupJobHandler marks stale tokens inactive. Defaults to 270 days.
func PushTokenCleanupJobHandler(store staleTokenCleaner, staleDays int) jobs.JobHandler {
	if staleDays <= 0 {
		staleDays = 270
	}
	return func(ctx context.Context, payload json.RawMessage) error {
		if store == nil {
			return fmt.Errorf("push_token_cleanup: store not configured")
		}
		_, err := store.CleanupStaleTokens(ctx, staleDays)
		if err != nil {
			return fmt.Errorf("push_token_cleanup: %w", err)
		}
		return nil
	}
}
