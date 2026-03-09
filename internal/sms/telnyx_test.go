package sms_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/allyourbase/ayb/internal/sms"
)

func TestTelnyxSendSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v2/messages", r.URL.Path)

		// Verify Bearer token auth
		assert.Equal(t, "Bearer TELNYX_API_KEY", r.Header.Get("Authorization"))

		// Verify JSON body
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var reqBody map[string]string
		require.NoError(t, json.Unmarshal(body, &reqBody))
		assert.Equal(t, "+15550000000", reqBody["from"])
		assert.Equal(t, "+15551234567", reqBody["to"])
		assert.Equal(t, "Your code is 123456", reqBody["text"])

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"id":"msg-telnyx-123","type":"message","record_type":"message"}}`))
	}))
	defer srv.Close()

	p := sms.NewTelnyxProvider("TELNYX_API_KEY", "+15550000000", srv.URL)
	result, err := p.Send(t.Context(), "+15551234567", "Your code is 123456")
	require.NoError(t, err)
	assert.Equal(t, "msg-telnyx-123", result.MessageID)
	assert.Equal(t, "message", result.Status)
}

func TestTelnyxSendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"errors":[{"title":"Invalid phone number","detail":"The to number is not valid"}]}`))
	}))
	defer srv.Close()

	p := sms.NewTelnyxProvider("TELNYX_API_KEY", "+15550000000", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid phone number")
}

func TestTelnyxSendErrorNonJSON(t *testing.T) {
	// Proxy/CDN returning non-JSON error — must fall back to raw body in error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`<html>Bad Gateway</html>`))
	}))
	defer srv.Close()

	p := sms.NewTelnyxProvider("TELNYX_API_KEY", "+15550000000", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telnyx: error 502")
	assert.Contains(t, err.Error(), "Bad Gateway")
}

func TestTelnyxSendNetworkError(t *testing.T) {
	p := sms.NewTelnyxProvider("TELNYX_API_KEY", "+15550000000", "http://127.0.0.1:1")
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telnyx: send request:")
}

func TestTelnyxImplementsInterface(t *testing.T) {
	var _ sms.Provider = (*sms.TelnyxProvider)(nil)
}
