package storage

import "context"

// StorageOperation represents the type of storage event.
type StorageOperation string

const (
	OperationUpload StorageOperation = "upload"
	OperationDelete StorageOperation = "delete"
)

// StorageEvent describes a storage operation that occurred.
type StorageEvent struct {
	Bucket      string           `json:"bucket"`
	Name        string           `json:"name"`
	Operation   StorageOperation `json:"operation"`
	Size        int64            `json:"size,omitempty"`
	ContentType string           `json:"contentType,omitempty"`
}

// StorageEventHandler receives notifications about storage events.
// Implementations should be fast and non-blocking. Errors are logged
// but not propagated to the original caller.
type StorageEventHandler interface {
	OnStorageEvent(ctx context.Context, event StorageEvent) error
}
