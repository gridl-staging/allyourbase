package push

import (
	"context"
	"errors"
)

// Sentinel errors returned by providers.
var (
	ErrInvalidToken    = errors.New("invalid device token")
	ErrUnregistered    = errors.New("device token unregistered")
	ErrProviderError   = errors.New("provider error")
	ErrPayloadTooLarge = errors.New("payload too large")
	ErrProviderAuth    = errors.New("provider auth error")
)

// Message is a push notification payload.
type Message struct {
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
}

// Result represents a provider send result.
type Result struct {
	MessageID string `json:"message_id"`
}

// Provider sends a push message to a device token.
type Provider interface {
	Send(ctx context.Context, token string, msg *Message) (*Result, error)
}
