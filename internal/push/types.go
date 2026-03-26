// Package push types defines core data structures and interfaces for the push notification service, including device tokens, delivery records, and the storage abstraction.
package push

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/allyourbase/ayb/internal/jobs"
)

const (
	ProviderFCM  = "fcm"
	ProviderAPNS = "apns"
)

const (
	PlatformAndroid = "android"
	PlatformIOS     = "ios"
)

const (
	DeliveryStatusPending      = "pending"
	DeliveryStatusSent         = "sent"
	DeliveryStatusFailed       = "failed"
	DeliveryStatusInvalidToken = "invalid_token"
)

const (
	JobTypePushDelivery   = "push_delivery"
	JobTypePushTokenClean = "push_token_cleanup"
)

var (
	ErrNotFound        = errors.New("push record not found")
	ErrInvalidProvider = errors.New("invalid push provider")
	ErrInvalidPlatform = errors.New("invalid push platform")
	ErrInvalidPayload  = errors.New("invalid push payload")
)

// DeviceToken represents a push device registration.
type DeviceToken struct {
	ID              string     `json:"id"`
	AppID           string     `json:"app_id"`
	UserID          string     `json:"user_id"`
	Provider        string     `json:"provider"`
	Platform        string     `json:"platform"`
	Token           string     `json:"token"`
	DeviceName      *string    `json:"device_name,omitempty"`
	IsActive        bool       `json:"is_active"`
	LastUsed        *time.Time `json:"last_used,omitempty"`
	LastRefreshedAt time.Time  `json:"last_refreshed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// PushDelivery is an audit record for push send attempts.
// PushDelivery is an audit record for a push notification send attempt, capturing the notification content, delivery status, error details, and provider response information.
type PushDelivery struct {
	ID                string            `json:"id"`
	DeviceTokenID     string            `json:"device_token_id"`
	JobID             *string           `json:"job_id,omitempty"`
	AppID             string            `json:"app_id"`
	UserID            string            `json:"user_id"`
	Provider          string            `json:"provider"`
	Title             string            `json:"title"`
	Body              string            `json:"body"`
	DataPayload       map[string]string `json:"data_payload,omitempty"`
	Status            string            `json:"status"`
	ErrorCode         *string           `json:"error_code,omitempty"`
	ErrorMessage      *string           `json:"error_message,omitempty"`
	ProviderMessageID *string           `json:"provider_message_id,omitempty"`
	SentAt            *time.Time        `json:"sent_at,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// PushStore defines persistence required by the push service.
// PushStore defines the persistence interface for managing device tokens and recording push notification delivery attempts, with operations for token registration, revocation, cleanup, and delivery audit logging.
type PushStore interface {
	RegisterToken(ctx context.Context, appID, userID, provider, platform, token, deviceName string) (*DeviceToken, error)
	GetToken(ctx context.Context, id string) (*DeviceToken, error)
	ListUserTokens(ctx context.Context, appID, userID string) ([]*DeviceToken, error)
	ListTokens(ctx context.Context, appID, userID string, includeInactive bool) ([]*DeviceToken, error)
	RevokeToken(ctx context.Context, id string) error
	RevokeAllUserTokens(ctx context.Context, appID, userID string) (int64, error)
	DeleteToken(ctx context.Context, id string) error
	UpdateLastUsed(ctx context.Context, id string) error
	MarkInactive(ctx context.Context, id string) error
	CleanupStaleTokens(ctx context.Context, staleDays int) (int64, error)
	RecordDelivery(ctx context.Context, delivery *PushDelivery) (*PushDelivery, error)
	SetDeliveryJobID(ctx context.Context, deliveryID, jobID string) error
	UpdateDeliveryStatus(ctx context.Context, id, status, errorCode, errorMsg, messageID string) error
	ListDeliveries(ctx context.Context, appID, userID, status string, limit, offset int) ([]*PushDelivery, error)
	GetDelivery(ctx context.Context, id string) (*PushDelivery, error)
}

type jobEnqueuer interface {
	Enqueue(ctx context.Context, jobType string, payload json.RawMessage, opts jobs.EnqueueOpts) (*jobs.Job, error)
}
