package sms

import (
	"context"
	"time"
)

// SendResult holds the outcome of a provider Send call.
type SendResult struct {
	MessageID string
	Status    string
}

// Provider sends an SMS to a phone number.
type Provider interface {
	Send(ctx context.Context, to, body string) (*SendResult, error)
}

// Config holds SMS verification settings.
type Config struct {
	CodeLength       int
	Expiry           time.Duration
	MaxAttempts      int
	DailyLimit       int
	AllowedCountries []string
	TestPhoneNumbers map[string]string // phone → predetermined code (skip provider send)
}
