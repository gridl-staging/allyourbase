package edgefunc_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

// --- StorageTrigger.MatchesStorageEvent tests ---

func TestStorageTriggerMatch_BasicBucketAndEvent(t *testing.T) {
	trigger := &edgefunc.StorageTrigger{
		Bucket:     "images",
		EventTypes: []string{"upload"},
		Enabled:    true,
	}

	testutil.True(t, trigger.MatchesStorageEvent("images", "photo.jpg", "upload"), "should match upload on images bucket")
	testutil.False(t, trigger.MatchesStorageEvent("docs", "photo.jpg", "upload"), "wrong bucket should not match")
	testutil.False(t, trigger.MatchesStorageEvent("images", "photo.jpg", "delete"), "wrong event type should not match")
}

func TestStorageTriggerMatch_MultipleEventTypes(t *testing.T) {
	trigger := &edgefunc.StorageTrigger{
		Bucket:     "data",
		EventTypes: []string{"upload", "delete"},
		Enabled:    true,
	}

	testutil.True(t, trigger.MatchesStorageEvent("data", "file.csv", "upload"), "should match upload")
	testutil.True(t, trigger.MatchesStorageEvent("data", "file.csv", "delete"), "should match delete")
}

func TestStorageTriggerMatch_Disabled(t *testing.T) {
	trigger := &edgefunc.StorageTrigger{
		Bucket:     "images",
		EventTypes: []string{"upload"},
		Enabled:    false,
	}

	testutil.False(t, trigger.MatchesStorageEvent("images", "photo.jpg", "upload"), "disabled trigger should not match")
}

func TestStorageTriggerMatch_PrefixFilter(t *testing.T) {
	trigger := &edgefunc.StorageTrigger{
		Bucket:       "uploads",
		EventTypes:   []string{"upload"},
		PrefixFilter: "images/",
		Enabled:      true,
	}

	testutil.True(t, trigger.MatchesStorageEvent("uploads", "images/photo.jpg", "upload"), "should match with prefix")
	testutil.False(t, trigger.MatchesStorageEvent("uploads", "docs/file.pdf", "upload"), "wrong prefix should not match")
}

func TestStorageTriggerMatch_SuffixFilter(t *testing.T) {
	trigger := &edgefunc.StorageTrigger{
		Bucket:       "uploads",
		EventTypes:   []string{"upload"},
		SuffixFilter: ".jpg",
		Enabled:      true,
	}

	testutil.True(t, trigger.MatchesStorageEvent("uploads", "photo.jpg", "upload"), "should match with suffix")
	testutil.False(t, trigger.MatchesStorageEvent("uploads", "photo.png", "upload"), "wrong suffix should not match")
}

func TestStorageTriggerMatch_PrefixAndSuffix(t *testing.T) {
	trigger := &edgefunc.StorageTrigger{
		Bucket:       "uploads",
		EventTypes:   []string{"upload"},
		PrefixFilter: "images/",
		SuffixFilter: ".jpg",
		Enabled:      true,
	}

	testutil.True(t, trigger.MatchesStorageEvent("uploads", "images/photo.jpg", "upload"), "should match both prefix and suffix")
	testutil.False(t, trigger.MatchesStorageEvent("uploads", "images/photo.png", "upload"), "wrong suffix with correct prefix")
	testutil.False(t, trigger.MatchesStorageEvent("uploads", "docs/photo.jpg", "upload"), "wrong prefix with correct suffix")
}

func TestStorageTriggerMatch_NoFilters(t *testing.T) {
	trigger := &edgefunc.StorageTrigger{
		Bucket:     "everything",
		EventTypes: []string{"upload", "delete"},
		Enabled:    true,
	}

	testutil.True(t, trigger.MatchesStorageEvent("everything", "any/path/file.txt", "upload"), "should match any name without filters")
	testutil.True(t, trigger.MatchesStorageEvent("everything", "any/path/file.txt", "delete"), "should match any name without filters")
}

// --- StorageTriggerService tests ---

func TestStorageTriggerService_Create(t *testing.T) {
	store := newMockStorageTriggerStore()
	svc := edgefunc.NewStorageTriggerService(store)

	trigger, err := svc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID:   uuid.New().String(),
		Bucket:       "images",
		EventTypes:   []string{"upload"},
		PrefixFilter: "avatars/",
		SuffixFilter: ".png",
	})
	testutil.NoError(t, err)
	testutil.True(t, trigger.ID != "", "should have an ID")
	testutil.Equal(t, "images", trigger.Bucket)
	testutil.Equal(t, 1, len(trigger.EventTypes))
	testutil.Equal(t, "upload", trigger.EventTypes[0])
	testutil.Equal(t, "avatars/", trigger.PrefixFilter)
	testutil.Equal(t, ".png", trigger.SuffixFilter)
	testutil.True(t, trigger.Enabled, "should be enabled by default")
}

func TestStorageTriggerService_Create_MissingFunctionID(t *testing.T) {
	store := newMockStorageTriggerStore()
	svc := edgefunc.NewStorageTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		Bucket:     "images",
		EventTypes: []string{"upload"},
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrFunctionIDRequired), "should require function ID, got: %v", err)
}

func TestStorageTriggerService_Create_MissingBucket(t *testing.T) {
	store := newMockStorageTriggerStore()
	svc := edgefunc.NewStorageTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: uuid.New().String(),
		EventTypes: []string{"upload"},
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrBucketRequired), "should require bucket, got: %v", err)
}

func TestStorageTriggerService_Create_MissingEventTypes(t *testing.T) {
	store := newMockStorageTriggerStore()
	svc := edgefunc.NewStorageTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: uuid.New().String(),
		Bucket:     "images",
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrEventTypesRequired), "should require event types, got: %v", err)
}

func TestStorageTriggerService_Create_InvalidEventType(t *testing.T) {
	store := newMockStorageTriggerStore()
	svc := edgefunc.NewStorageTriggerService(store)

	_, err := svc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: uuid.New().String(),
		Bucket:     "images",
		EventTypes: []string{"upload", "invalid"},
	})
	testutil.True(t, errors.Is(err, edgefunc.ErrInvalidEventType), "should reject invalid event type, got: %v", err)
}

func TestStorageTriggerService_Get(t *testing.T) {
	store := newMockStorageTriggerStore()
	svc := edgefunc.NewStorageTriggerService(store)

	created, err := svc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: uuid.New().String(),
		Bucket:     "images",
		EventTypes: []string{"upload"},
	})
	testutil.NoError(t, err)

	got, err := svc.Get(context.Background(), created.ID)
	testutil.NoError(t, err)
	testutil.Equal(t, created.ID, got.ID)
}

func TestStorageTriggerService_Get_NotFound(t *testing.T) {
	store := newMockStorageTriggerStore()
	svc := edgefunc.NewStorageTriggerService(store)

	_, err := svc.Get(context.Background(), "nonexistent")
	testutil.True(t, errors.Is(err, edgefunc.ErrStorageTriggerNotFound), "should return not found, got: %v", err)
}

func TestStorageTriggerService_List(t *testing.T) {
	store := newMockStorageTriggerStore()
	svc := edgefunc.NewStorageTriggerService(store)

	funcID := uuid.New().String()
	svc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: funcID, Bucket: "images", EventTypes: []string{"upload"},
	})
	svc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: funcID, Bucket: "docs", EventTypes: []string{"delete"},
	})

	triggers, err := svc.List(context.Background(), funcID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, triggers, 2)
}

func TestStorageTriggerService_Delete(t *testing.T) {
	store := newMockStorageTriggerStore()
	svc := edgefunc.NewStorageTriggerService(store)

	created, err := svc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: uuid.New().String(),
		Bucket:     "images",
		EventTypes: []string{"upload"},
	})
	testutil.NoError(t, err)

	err = svc.Delete(context.Background(), created.ID)
	testutil.NoError(t, err)

	_, err = svc.Get(context.Background(), created.ID)
	testutil.True(t, errors.Is(err, edgefunc.ErrStorageTriggerNotFound), "should be deleted")
}

func TestStorageTriggerService_SetEnabled(t *testing.T) {
	store := newMockStorageTriggerStore()
	svc := edgefunc.NewStorageTriggerService(store)

	created, err := svc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: uuid.New().String(),
		Bucket:     "images",
		EventTypes: []string{"upload"},
	})
	testutil.NoError(t, err)
	testutil.True(t, created.Enabled, "should start enabled")

	updated, err := svc.SetEnabled(context.Background(), created.ID, false)
	testutil.NoError(t, err)
	testutil.False(t, updated.Enabled, "should be disabled")

	updated, err = svc.SetEnabled(context.Background(), created.ID, true)
	testutil.NoError(t, err)
	testutil.True(t, updated.Enabled, "should be re-enabled")
}

// --- StorageTriggerDispatcher tests ---

func TestStorageTriggerDispatcher_MatchAndInvoke(t *testing.T) {
	store := newMockStorageTriggerStore()
	triggerSvc := edgefunc.NewStorageTriggerService(store)

	funcID := uuid.New().String()
	triggerSvc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: funcID,
		Bucket:     "images",
		EventTypes: []string{"upload"},
	})

	var invocations []dispatcherInvokeCall
	invoker := &mockDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations = append(invocations, dispatcherInvokeCall{functionID: functionID, req: req})
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewStorageTriggerDispatcher(store, invoker)

	err := dispatcher.OnStorageEvent(context.Background(), storage.StorageEvent{
		Bucket:      "images",
		Name:        "photo.jpg",
		Operation:   storage.OperationUpload,
		Size:        2048,
		ContentType: "image/jpeg",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(invocations))
	testutil.Equal(t, funcID, invocations[0].functionID)
	testutil.Equal(t, "POST", invocations[0].req.Method)
	testutil.Equal(t, "/storage", invocations[0].req.Path)

	// Verify payload contains event data
	var payload map[string]interface{}
	testutil.NoError(t, json.Unmarshal(invocations[0].req.Body, &payload))
	testutil.Equal(t, "images", payload["bucket"])
	testutil.Equal(t, "photo.jpg", payload["name"])
	testutil.Equal(t, "upload", payload["operation"])
}

func TestStorageTriggerDispatcher_NoMatchingTriggers(t *testing.T) {
	store := newMockStorageTriggerStore()
	triggerSvc := edgefunc.NewStorageTriggerService(store)

	// Create trigger for "images" bucket
	triggerSvc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: uuid.New().String(),
		Bucket:     "images",
		EventTypes: []string{"upload"},
	})

	var invocations []dispatcherInvokeCall
	invoker := &mockDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations = append(invocations, dispatcherInvokeCall{functionID: functionID, req: req})
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewStorageTriggerDispatcher(store, invoker)

	// Event on different bucket - should not trigger
	err := dispatcher.OnStorageEvent(context.Background(), storage.StorageEvent{
		Bucket:    "docs",
		Name:      "file.pdf",
		Operation: storage.OperationUpload,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 0, len(invocations))
}

func TestStorageTriggerDispatcher_PrefixFilterApplied(t *testing.T) {
	store := newMockStorageTriggerStore()
	triggerSvc := edgefunc.NewStorageTriggerService(store)

	funcID := uuid.New().String()
	triggerSvc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID:   funcID,
		Bucket:       "uploads",
		EventTypes:   []string{"upload"},
		PrefixFilter: "images/",
	})

	var invocations []dispatcherInvokeCall
	invoker := &mockDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations = append(invocations, dispatcherInvokeCall{functionID: functionID, req: req})
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewStorageTriggerDispatcher(store, invoker)

	// Matching prefix
	dispatcher.OnStorageEvent(context.Background(), storage.StorageEvent{
		Bucket: "uploads", Name: "images/photo.jpg", Operation: storage.OperationUpload,
	})
	testutil.Equal(t, 1, len(invocations))

	// Non-matching prefix
	dispatcher.OnStorageEvent(context.Background(), storage.StorageEvent{
		Bucket: "uploads", Name: "docs/file.pdf", Operation: storage.OperationUpload,
	})
	testutil.Equal(t, 1, len(invocations)) // Still 1, not invoked again
}

func TestStorageTriggerDispatcher_MultipleTriggersSameBucket(t *testing.T) {
	store := newMockStorageTriggerStore()
	triggerSvc := edgefunc.NewStorageTriggerService(store)

	funcID1 := uuid.New().String()
	funcID2 := uuid.New().String()

	triggerSvc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: funcID1, Bucket: "shared", EventTypes: []string{"upload"},
	})
	triggerSvc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: funcID2, Bucket: "shared", EventTypes: []string{"upload"},
	})

	var invocations []dispatcherInvokeCall
	invoker := &mockDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations = append(invocations, dispatcherInvokeCall{functionID: functionID, req: req})
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewStorageTriggerDispatcher(store, invoker)

	dispatcher.OnStorageEvent(context.Background(), storage.StorageEvent{
		Bucket: "shared", Name: "file.txt", Operation: storage.OperationUpload,
	})

	testutil.Equal(t, 2, len(invocations))
}

func TestStorageTriggerDispatcher_InvokerError_DoesNotPropagateToStorage(t *testing.T) {
	store := newMockStorageTriggerStore()
	triggerSvc := edgefunc.NewStorageTriggerService(store)

	triggerSvc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: uuid.New().String(),
		Bucket:     "images",
		EventTypes: []string{"upload"},
	})

	invoker := &mockDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			return edgefunc.Response{}, errors.New("function execution failed")
		},
	}

	dispatcher := edgefunc.NewStorageTriggerDispatcher(store, invoker)

	// Error from function invocation should not be returned as an error from OnStorageEvent
	err := dispatcher.OnStorageEvent(context.Background(), storage.StorageEvent{
		Bucket: "images", Name: "photo.jpg", Operation: storage.OperationUpload,
	})
	testutil.NoError(t, err)
}

func TestStorageTriggerDispatcher_RecursionGuard(t *testing.T) {
	store := newMockStorageTriggerStore()
	triggerSvc := edgefunc.NewStorageTriggerService(store)

	funcID := uuid.New().String()
	triggerSvc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: funcID,
		Bucket:     "images",
		EventTypes: []string{"upload"},
	})

	var invocations int
	invoker := &mockDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations++
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewStorageTriggerDispatcher(store, invoker)

	// Simulate a context already marked as being inside a storage trigger
	ctx := edgefunc.WithStorageTriggerSource(context.Background(), funcID)

	err := dispatcher.OnStorageEvent(ctx, storage.StorageEvent{
		Bucket: "images", Name: "photo.jpg", Operation: storage.OperationUpload,
	})
	testutil.NoError(t, err)
	// The trigger for funcID should be skipped due to recursion guard
	testutil.Equal(t, 0, invocations)
}

func TestStorageTriggerDispatcher_DeleteEvent(t *testing.T) {
	store := newMockStorageTriggerStore()
	triggerSvc := edgefunc.NewStorageTriggerService(store)

	funcID := uuid.New().String()
	triggerSvc.Create(context.Background(), edgefunc.CreateStorageTriggerInput{
		FunctionID: funcID,
		Bucket:     "images",
		EventTypes: []string{"delete"},
	})

	var invocations []dispatcherInvokeCall
	invoker := &mockDispatcherInvoker{
		fn: func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
			invocations = append(invocations, dispatcherInvokeCall{functionID: functionID, req: req})
			return edgefunc.Response{StatusCode: 200}, nil
		},
	}

	dispatcher := edgefunc.NewStorageTriggerDispatcher(store, invoker)

	err := dispatcher.OnStorageEvent(context.Background(), storage.StorageEvent{
		Bucket:    "images",
		Name:      "old-photo.jpg",
		Operation: storage.OperationDelete,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(invocations))

	var payload map[string]interface{}
	testutil.NoError(t, json.Unmarshal(invocations[0].req.Body, &payload))
	testutil.Equal(t, "delete", payload["operation"])
}

// --- Mock StorageTriggerStore ---

type mockStorageTriggerStore struct {
	triggers map[string]*edgefunc.StorageTrigger
	byFunc   map[string][]string
	byBucket map[string][]string
}

func newMockStorageTriggerStore() *mockStorageTriggerStore {
	return &mockStorageTriggerStore{
		triggers: make(map[string]*edgefunc.StorageTrigger),
		byFunc:   make(map[string][]string),
		byBucket: make(map[string][]string),
	}
}

func (m *mockStorageTriggerStore) CreateStorageTrigger(_ context.Context, t *edgefunc.StorageTrigger) (*edgefunc.StorageTrigger, error) {
	t.ID = uuid.New().String()
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt
	m.triggers[t.ID] = t
	m.byFunc[t.FunctionID] = append(m.byFunc[t.FunctionID], t.ID)
	m.byBucket[t.Bucket] = append(m.byBucket[t.Bucket], t.ID)
	return t, nil
}

func (m *mockStorageTriggerStore) GetStorageTrigger(_ context.Context, id string) (*edgefunc.StorageTrigger, error) {
	t, ok := m.triggers[id]
	if !ok {
		return nil, edgefunc.ErrStorageTriggerNotFound
	}
	return t, nil
}

func (m *mockStorageTriggerStore) ListStorageTriggers(_ context.Context, functionID string) ([]*edgefunc.StorageTrigger, error) {
	var result []*edgefunc.StorageTrigger
	for _, id := range m.byFunc[functionID] {
		if t, ok := m.triggers[id]; ok {
			result = append(result, t)
		}
	}
	if result == nil {
		result = []*edgefunc.StorageTrigger{}
	}
	return result, nil
}

func (m *mockStorageTriggerStore) ListStorageTriggersByBucket(_ context.Context, bucket string) ([]*edgefunc.StorageTrigger, error) {
	var result []*edgefunc.StorageTrigger
	for _, id := range m.byBucket[bucket] {
		if t, ok := m.triggers[id]; ok {
			result = append(result, t)
		}
	}
	if result == nil {
		result = []*edgefunc.StorageTrigger{}
	}
	return result, nil
}

func (m *mockStorageTriggerStore) UpdateStorageTrigger(_ context.Context, t *edgefunc.StorageTrigger) (*edgefunc.StorageTrigger, error) {
	if _, ok := m.triggers[t.ID]; !ok {
		return nil, edgefunc.ErrStorageTriggerNotFound
	}
	t.UpdatedAt = time.Now()
	m.triggers[t.ID] = t
	return t, nil
}

func (m *mockStorageTriggerStore) DeleteStorageTrigger(_ context.Context, id string) error {
	t, ok := m.triggers[id]
	if !ok {
		return edgefunc.ErrStorageTriggerNotFound
	}
	delete(m.triggers, id)
	// Remove from byFunc
	for i, tid := range m.byFunc[t.FunctionID] {
		if tid == id {
			m.byFunc[t.FunctionID] = append(m.byFunc[t.FunctionID][:i], m.byFunc[t.FunctionID][i+1:]...)
			break
		}
	}
	// Remove from byBucket
	for i, tid := range m.byBucket[t.Bucket] {
		if tid == id {
			m.byBucket[t.Bucket] = append(m.byBucket[t.Bucket][:i], m.byBucket[t.Bucket][i+1:]...)
			break
		}
	}
	return nil
}

// --- Mock Invoker for Dispatcher ---

type dispatcherInvokeCall struct {
	functionID string
	req        edgefunc.Request
}

type mockDispatcherInvoker struct {
	fn func(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error)
}

func (m *mockDispatcherInvoker) InvokeByID(ctx context.Context, functionID string, req edgefunc.Request) (edgefunc.Response, error) {
	if m.fn != nil {
		return m.fn(ctx, functionID, req)
	}
	return edgefunc.Response{StatusCode: 200}, nil
}
