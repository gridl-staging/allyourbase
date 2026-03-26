package edgefunc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/allyourbase/ayb/internal/storage"
)

// CreateStorageTriggerInput holds the parameters for creating a storage trigger.
type CreateStorageTriggerInput struct {
	FunctionID   string
	Bucket       string
	EventTypes   []string
	PrefixFilter string
	SuffixFilter string
}

// StorageTriggerService manages the lifecycle of storage triggers.
type StorageTriggerService struct {
	store StorageTriggerStore
}

// NewStorageTriggerService creates a new StorageTriggerService.
func NewStorageTriggerService(store StorageTriggerStore) *StorageTriggerService {
	return &StorageTriggerService{store: store}
}

// Create validates input and persists a new storage trigger.
func (s *StorageTriggerService) Create(ctx context.Context, input CreateStorageTriggerInput) (*StorageTrigger, error) {
	if strings.TrimSpace(input.FunctionID) == "" {
		return nil, ErrFunctionIDRequired
	}
	if strings.TrimSpace(input.Bucket) == "" {
		return nil, ErrBucketRequired
	}
	if len(input.EventTypes) == 0 {
		return nil, ErrEventTypesRequired
	}
	for _, et := range input.EventTypes {
		if !ValidStorageEventTypes[et] {
			return nil, fmt.Errorf("%w: %s", ErrInvalidEventType, et)
		}
	}

	trigger := &StorageTrigger{
		FunctionID:   input.FunctionID,
		Bucket:       input.Bucket,
		EventTypes:   input.EventTypes,
		PrefixFilter: input.PrefixFilter,
		SuffixFilter: input.SuffixFilter,
		Enabled:      true,
	}

	return s.store.CreateStorageTrigger(ctx, trigger)
}

// Get returns a storage trigger by ID.
func (s *StorageTriggerService) Get(ctx context.Context, id string) (*StorageTrigger, error) {
	return s.store.GetStorageTrigger(ctx, id)
}

// List returns all storage triggers for a function.
func (s *StorageTriggerService) List(ctx context.Context, functionID string) ([]*StorageTrigger, error) {
	return s.store.ListStorageTriggers(ctx, functionID)
}

// Delete removes a storage trigger.
func (s *StorageTriggerService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteStorageTrigger(ctx, id)
}

// SetEnabled toggles a storage trigger's enabled state.
func (s *StorageTriggerService) SetEnabled(ctx context.Context, id string, enabled bool) (*StorageTrigger, error) {
	trigger, err := s.store.GetStorageTrigger(ctx, id)
	if err != nil {
		return nil, err
	}
	trigger.Enabled = enabled
	return s.store.UpdateStorageTrigger(ctx, trigger)
}

// storageTriggerSourceKey is the context key for tracking which function triggered a storage event.
type storageTriggerSourceKey struct{}

// StorageTriggerSource returns the function ID that is the source of a storage event, if any.
func StorageTriggerSource(ctx context.Context) string {
	if v, ok := ctx.Value(storageTriggerSourceKey{}).(string); ok {
		return v
	}
	return ""
}

// WithStorageTriggerSource returns a context marking the given function as the source of storage events.
func WithStorageTriggerSource(ctx context.Context, functionID string) context.Context {
	return context.WithValue(ctx, storageTriggerSourceKey{}, functionID)
}

// StorageTriggerDispatcher implements storage.StorageEventHandler.
// It loads matching triggers for a bucket and invokes the associated functions.
type StorageTriggerDispatcher struct {
	store   StorageTriggerStore
	invoker FunctionInvoker
	logger  *slog.Logger
}

// NewStorageTriggerDispatcher creates a dispatcher that invokes edge functions
// in response to storage events.
func NewStorageTriggerDispatcher(store StorageTriggerStore, invoker FunctionInvoker, opts ...func(*StorageTriggerDispatcher)) *StorageTriggerDispatcher {
	d := &StorageTriggerDispatcher{
		store:   store,
		invoker: invoker,
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// WithDispatcherLogger sets the logger for the dispatcher.
func WithDispatcherLogger(logger *slog.Logger) func(*StorageTriggerDispatcher) {
	return func(d *StorageTriggerDispatcher) {
		d.logger = logger
	}
}

// OnStorageEvent implements storage.StorageEventHandler.
// It loads triggers matching the bucket, filters by event type/prefix/suffix,
// and invokes the matched functions. Invocation errors are logged but not propagated.
func (d *StorageTriggerDispatcher) OnStorageEvent(ctx context.Context, event storage.StorageEvent) error {
	triggers, err := d.store.ListStorageTriggersByBucket(ctx, event.Bucket)
	if err != nil {
		d.logger.Error("loading storage triggers", "bucket", event.Bucket, "error", err)
		return nil // Don't fail the storage operation
	}

	operation := string(event.Operation)
	sourceFunc := StorageTriggerSource(ctx)

	// Chain depth guard: prevent indirect cycles across all trigger types
	if err := CheckTriggerChainDepth(ctx); err != nil {
		d.logger.Warn("skipping all storage trigger dispatch (chain depth exceeded)",
			"bucket", event.Bucket,
			"name", event.Name,
			"depth", TriggerChainDepth(ctx),
		)
		return nil
	}

	for _, trigger := range triggers {
		if !trigger.MatchesStorageEvent(event.Bucket, event.Name, operation) {
			continue
		}

		// Direct recursion guard: skip if this function is the source of the event
		if sourceFunc != "" && trigger.FunctionID == sourceFunc {
			d.logger.Info("skipping storage trigger (direct recursion guard)",
				"trigger_id", trigger.ID,
				"function_id", trigger.FunctionID,
				"bucket", event.Bucket,
				"name", event.Name,
			)
			continue
		}

		payload, err := json.Marshal(storageEventPayload{
			Bucket:      event.Bucket,
			Name:        event.Name,
			Operation:   operation,
			Size:        event.Size,
			ContentType: event.ContentType,
		})
		if err != nil {
			d.logger.Error("marshaling storage event payload", "error", err)
			continue
		}

		req := Request{
			Method: "POST",
			Path:   "/storage",
			Body:   payload,
		}

		childCtx := WithStorageTriggerSource(ctx, trigger.FunctionID)
		childCtx = WithTriggerMeta(childCtx, TriggerStorage, trigger.ID)
		childCtx = WithTriggerChainDepth(childCtx, TriggerChainDepth(ctx)+1)
		if _, err := d.invoker.InvokeByID(childCtx, trigger.FunctionID, req); err != nil {
			d.logger.Error("storage trigger dispatch failed",
				"trigger_id", trigger.ID,
				"function_id", trigger.FunctionID,
				"bucket", event.Bucket,
				"name", event.Name,
				"operation", operation,
				"error", err,
			)
		} else {
			d.logger.Info("storage trigger dispatch success",
				"trigger_id", trigger.ID,
				"function_id", trigger.FunctionID,
				"bucket", event.Bucket,
				"name", event.Name,
				"operation", operation,
			)
		}
	}

	return nil
}

// storageEventPayload is the JSON payload sent to edge functions on storage events.
type storageEventPayload struct {
	Bucket      string `json:"bucket"`
	Name        string `json:"name"`
	Operation   string `json:"operation"`
	Size        int64  `json:"size,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}
