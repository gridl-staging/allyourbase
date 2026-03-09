package sms_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/allyourbase/ayb/internal/sms"
)

func TestWebhookSendSuccess(t *testing.T) {
	secret := "webhook-secret-key"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		// Verify request body
		var reqBody map[string]string
		require.NoError(t, json.Unmarshal(body, &reqBody))
		assert.Equal(t, "+15551234567", reqBody["to"])
		assert.Equal(t, "Your code is 123456", reqBody["body"])
		assert.NotEmpty(t, reqBody["timestamp"])

		// Verify HMAC-SHA256 signature
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expectedSig := hex.EncodeToString(mac.Sum(nil))
		assert.Equal(t, expectedSig, r.Header.Get("X-Webhook-Signature"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message_id":"webhook-msg-123"}`))
	}))
	defer srv.Close()

	p := sms.NewWebhookProvider(srv.URL, secret)
	result, err := p.Send(t.Context(), "+15551234567", "Your code is 123456")
	require.NoError(t, err)
	assert.Equal(t, "webhook-msg-123", result.MessageID)
	assert.Equal(t, "sent", result.Status)
}

func TestWebhookSendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal failure"}`))
	}))
	defer srv.Close()

	p := sms.NewWebhookProvider(srv.URL, "secret")
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webhook: error 500")
}

func TestWebhookSendSignatureVerification(t *testing.T) {
	secret := "test-secret"
	var capturedBody []byte
	var capturedSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedSig = r.Header.Get("X-Webhook-Signature")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message_id":"sig-test-123"}`))
	}))
	defer srv.Close()

	p := sms.NewWebhookProvider(srv.URL, secret)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.NoError(t, err)

	// Independently compute expected signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(capturedBody)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	assert.Equal(t, expectedSig, capturedSig)
}

func TestWebhookSendNetworkError(t *testing.T) {
	p := sms.NewWebhookProvider("http://127.0.0.1:1", "secret")
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webhook: send request:")
}

func TestWebhookImplementsInterface(t *testing.T) {
	var _ sms.Provider = (*sms.WebhookProvider)(nil)
}
