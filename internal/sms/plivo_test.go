package sms_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/allyourbase/ayb/internal/sms"
)

func TestPlivoSendSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/Account/PLIVO_AUTH_ID/Message/", r.URL.Path)

		// Verify HTTP Basic auth
		auth := r.Header.Get("Authorization")
		expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("PLIVO_AUTH_ID:PLIVO_AUTH_TOKEN"))
		assert.Equal(t, expected, auth)

		// Verify JSON body
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var reqBody map[string]string
		require.NoError(t, json.Unmarshal(body, &reqBody))
		assert.Equal(t, "+15550000000", reqBody["src"])
		assert.Equal(t, "+15551234567", reqBody["dst"])
		assert.Equal(t, "Your code is 123456", reqBody["text"])

		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"message_uuid":["abc-123-uuid"],"api_id":"api-xyz","message":"message(s) queued"}`))
	}))
	defer srv.Close()

	p := sms.NewPlivoProvider("PLIVO_AUTH_ID", "PLIVO_AUTH_TOKEN", "+15550000000", srv.URL)
	result, err := p.Send(t.Context(), "+15551234567", "Your code is 123456")
	require.NoError(t, err)
	assert.Equal(t, "abc-123-uuid", result.MessageID)
	assert.Equal(t, "queued", result.Status)
}

func TestPlivoSendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"api_id":"api-xyz","error":"invalid destination number"}`))
	}))
	defer srv.Close()

	p := sms.NewPlivoProvider("PLIVO_AUTH_ID", "PLIVO_AUTH_TOKEN", "+15550000000", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid destination number")
}

func TestPlivoSendEmptyMessageUUID(t *testing.T) {
	// Plivo returns 200 with empty message_uuid array — must not panic on index access.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"message_uuid":[],"api_id":"api-xyz","message":"message(s) queued"}`))
	}))
	defer srv.Close()

	p := sms.NewPlivoProvider("PLIVO_AUTH_ID", "PLIVO_AUTH_TOKEN", "+15550000000", srv.URL)
	result, err := p.Send(t.Context(), "+15551234567", "hello")
	require.NoError(t, err)
	assert.Equal(t, "", result.MessageID)
	assert.Equal(t, "queued", result.Status)
}

func TestPlivoSendErrorNonJSON(t *testing.T) {
	// Proxy/CDN returning non-JSON error — must fall back to raw body in error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`<html>Bad Gateway</html>`))
	}))
	defer srv.Close()

	p := sms.NewPlivoProvider("PLIVO_AUTH_ID", "PLIVO_AUTH_TOKEN", "+15550000000", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plivo: error 502")
	assert.Contains(t, err.Error(), "Bad Gateway")
}

func TestPlivoSendNetworkError(t *testing.T) {
	p := sms.NewPlivoProvider("PLIVO_AUTH_ID", "PLIVO_AUTH_TOKEN", "+15550000000", "http://127.0.0.1:1")
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plivo: send request:")
}

func TestPlivoImplementsInterface(t *testing.T) {
	var _ sms.Provider = (*sms.PlivoProvider)(nil)
}
