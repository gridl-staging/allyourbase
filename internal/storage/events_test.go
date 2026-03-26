package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

type captureHandler struct {
	events []StorageEvent
	err    error // if non-nil, OnStorageEvent returns this error
}

func (h *captureHandler) OnStorageEvent(_ context.Context, event StorageEvent) error {
	h.events = append(h.events, event)
	return h.err
}

func TestRegisterEventHandler(t *testing.T) {
	svc := &Service{logger: testutil.DiscardLogger()}
	h := &captureHandler{}

	svc.RegisterEventHandler(h)
	testutil.Equal(t, 1, len(svc.eventHandlers))
}

func TestDispatchEvent_SingleHandler(t *testing.T) {
	svc := &Service{logger: testutil.DiscardLogger()}
	h := &captureHandler{}
	svc.RegisterEventHandler(h)

	event := StorageEvent{
		Bucket:      "images",
		Name:        "photo.jpg",
		Operation:   OperationUpload,
		Size:        1024,
		ContentType: "image/jpeg",
	}
	svc.dispatchEvent(context.Background(), event)

	testutil.Equal(t, 1, len(h.events))
	testutil.Equal(t, "images", h.events[0].Bucket)
	testutil.Equal(t, "photo.jpg", h.events[0].Name)
	testutil.Equal(t, OperationUpload, h.events[0].Operation)
	testutil.Equal(t, int64(1024), h.events[0].Size)
	testutil.Equal(t, "image/jpeg", h.events[0].ContentType)
}

func TestDispatchEvent_MultipleHandlers(t *testing.T) {
	svc := &Service{logger: testutil.DiscardLogger()}
	h1 := &captureHandler{}
	h2 := &captureHandler{}
	svc.RegisterEventHandler(h1)
	svc.RegisterEventHandler(h2)

	svc.dispatchEvent(context.Background(), StorageEvent{
		Bucket:    "docs",
		Name:      "file.pdf",
		Operation: OperationDelete,
	})

	testutil.Equal(t, 1, len(h1.events))
	testutil.Equal(t, 1, len(h2.events))
	testutil.Equal(t, OperationDelete, h1.events[0].Operation)
	testutil.Equal(t, OperationDelete, h2.events[0].Operation)
}

func TestDispatchEvent_HandlerErrorLogged_NotPropagated(t *testing.T) {
	svc := &Service{logger: testutil.DiscardLogger()}
	failing := &captureHandler{err: errors.New("handler failed")}
	succeeding := &captureHandler{}
	svc.RegisterEventHandler(failing)
	svc.RegisterEventHandler(succeeding)

	// Even though first handler fails, second should still be called
	svc.dispatchEvent(context.Background(), StorageEvent{
		Bucket:    "data",
		Name:      "test.csv",
		Operation: OperationUpload,
	})

	testutil.Equal(t, 1, len(failing.events))
	testutil.Equal(t, 1, len(succeeding.events))
}

func TestDispatchEvent_NoHandlers(t *testing.T) {
	svc := &Service{logger: testutil.DiscardLogger()}
	// Should not panic with no handlers
	svc.dispatchEvent(context.Background(), StorageEvent{
		Bucket:    "empty",
		Name:      "test.txt",
		Operation: OperationUpload,
	})
}
