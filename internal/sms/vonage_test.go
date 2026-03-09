package sms_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/allyourbase/ayb/internal/sms"
)

func TestVonageSendSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/sms/json", r.URL.Path)

		// Vonage uses form-encoded POST
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "VONAGE_KEY", r.FormValue("api_key"))
		assert.Equal(t, "VONAGE_SECRET", r.FormValue("api_secret"))
		assert.Equal(t, "+15550000000", r.FormValue("from"))
		assert.Equal(t, "+15551234567", r.FormValue("to"))
		assert.Equal(t, "Your code is 123456", r.FormValue("text"))

		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"message-count": "1",
			"messages": []map[string]string{
				{"message-id": "vonage-msg-123", "status": "0", "to": "+15551234567"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := sms.NewVonageProvider("VONAGE_KEY", "VONAGE_SECRET", "+15550000000", srv.URL)
	result, err := p.Send(t.Context(), "+15551234567", "Your code is 123456")
	require.NoError(t, err)
	assert.Equal(t, "vonage-msg-123", result.MessageID)
	assert.Equal(t, "sent", result.Status)
}

func TestVonageSendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"message-count": "1",
			"messages": []map[string]string{
				{"status": "2", "error-text": "Missing api_key"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := sms.NewVonageProvider("VONAGE_KEY", "VONAGE_SECRET", "+15550000000", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Missing api_key")
}

func TestVonageSendHTTPError(t *testing.T) {
	// Proxy/CDN returning non-JSON non-200 — must not produce a confusing parse error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`<html>Bad Gateway</html>`))
	}))
	defer srv.Close()

	p := sms.NewVonageProvider("VONAGE_KEY", "VONAGE_SECRET", "+15550000000", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vonage: error 502")
}

func TestVonageSendEmptyMessages(t *testing.T) {
	// Vonage returns 200 with empty messages array — must not panic on index access.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message-count":"0","messages":[]}`))
	}))
	defer srv.Close()

	p := sms.NewVonageProvider("VONAGE_KEY", "VONAGE_SECRET", "+15550000000", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vonage: empty response")
}

func TestVonageSendNetworkError(t *testing.T) {
	p := sms.NewVonageProvider("VONAGE_KEY", "VONAGE_SECRET", "+15550000000", "http://127.0.0.1:1")
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vonage: send request:")
}

func TestVonageImplementsInterface(t *testing.T) {
	var _ sms.Provider = (*sms.VonageProvider)(nil)
}
