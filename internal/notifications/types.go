package notifications

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a notification does not exist or is not visible to the user.
var ErrNotFound = errors.New("notification not found")

// Notification represents an in-app notification for a user.
type Notification struct {
	ID        string         `json:"id"`
	UserID    string         `json:"user_id"`
	Title     string         `json:"title"`
	Body      string         `json:"body,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Channel   string         `json:"channel"`
	ReadAt    *time.Time     `json:"read_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// NotificationStore defines persistence operations for notifications.
type NotificationStore interface {
	Create(ctx context.Context, userID, title, body string, metadata map[string]any, channel string) (*Notification, error)
	ListByUser(ctx context.Context, userID string, unreadOnly bool, page, perPage int) ([]*Notification, int, error)
	GetByID(ctx context.Context, id, userID string) (*Notification, error)
	MarkRead(ctx context.Context, id, userID string) error
	MarkAllRead(ctx context.Context, userID string) (int64, error)
}
