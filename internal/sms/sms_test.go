package sms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/allyourbase/ayb/internal/sms"
)

func TestLogProviderSend(t *testing.T) {
	p := sms.NewLogProvider(nil) // nil logger → default
	result, err := p.Send(context.Background(), "+14155552671", "Your code is: 123456")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "logged", result.Status)
}

func TestLogProviderImplementsInterface(t *testing.T) {
	var _ sms.Provider = (*sms.LogProvider)(nil)
}

func TestSendResultFields(t *testing.T) {
	r := &sms.SendResult{
		MessageID: "SM123",
		Status:    "queued",
	}
	assert.Equal(t, "SM123", r.MessageID)
	assert.Equal(t, "queued", r.Status)
}
