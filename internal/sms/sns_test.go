package sms_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/allyourbase/ayb/internal/sms"
)

// mockSNSPublisher implements sms.SNSPublisher for testing.
type mockSNSPublisher struct {
	publishFunc func(ctx context.Context, phoneNumber, message string) (string, error)
}

func (m *mockSNSPublisher) Publish(ctx context.Context, phoneNumber, message string) (string, error) {
	return m.publishFunc(ctx, phoneNumber, message)
}

func TestSNSSendSuccess(t *testing.T) {
	mock := &mockSNSPublisher{
		publishFunc: func(ctx context.Context, phoneNumber, message string) (string, error) {
			assert.Equal(t, "+15551234567", phoneNumber)
			assert.Equal(t, "Your code is 123456", message)
			return "sns-msg-id-abc", nil
		},
	}

	p := sms.NewSNSProvider(mock)
	result, err := p.Send(t.Context(), "+15551234567", "Your code is 123456")
	require.NoError(t, err)
	assert.Equal(t, "sns-msg-id-abc", result.MessageID)
	assert.Equal(t, "sent", result.Status)
}

func TestSNSSendError(t *testing.T) {
	mock := &mockSNSPublisher{
		publishFunc: func(ctx context.Context, phoneNumber, message string) (string, error) {
			return "", fmt.Errorf("AccessDeniedException: not authorized")
		},
	}

	p := sms.NewSNSProvider(mock)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sns: publish:")
	assert.Contains(t, err.Error(), "AccessDeniedException")
}

func TestSNSImplementsInterface(t *testing.T) {
	var _ sms.Provider = (*sms.SNSProvider)(nil)
}
