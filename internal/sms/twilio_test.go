package sms_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/allyourbase/ayb/internal/sms"
)

func TestTwilioSendSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/2010-04-01/Accounts/ACtest/Messages.json")

		auth := r.Header.Get("Authorization")
		expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("ACtest:token"))
		assert.Equal(t, expected, auth)

		assert.Equal(t, "+15551234567", r.FormValue("To"))
		assert.Equal(t, "+15550000000", r.FormValue("From"))
		assert.Equal(t, "hello", r.FormValue("Body"))

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"SM123","status":"queued"}`))
	}))
	defer srv.Close()

	p := sms.NewTwilioProvider("ACtest", "token", "+15550000000", srv.URL)
	result, err := p.Send(t.Context(), "+15551234567", "hello")
	require.NoError(t, err)
	assert.Equal(t, "SM123", result.MessageID)
	assert.Equal(t, "queued", result.Status)
}

func TestTwilioSendHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":21211,"message":"invalid To"}`))
	}))
	defer srv.Close()

	p := sms.NewTwilioProvider("ACtest", "token", "+15550000000", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "21211")
}

func TestTwilioSendHTTPErrorNonJSON(t *testing.T) {
	// Proxy returning non-JSON error — must not produce a confusing parse error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`<html>Bad Gateway</html>`))
	}))
	defer srv.Close()

	p := sms.NewTwilioProvider("ACtest", "token", "+15550000000", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "twilio: error 502")
}

func TestTwilioSendNetworkError(t *testing.T) {
	// Point at a server that immediately closes to simulate network failure.
	p := sms.NewTwilioProvider("ACtest", "token", "+15550000000", "http://127.0.0.1:1")
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "twilio: send request:")
}

func TestTwilioImplementsInterface(t *testing.T) {
	var _ sms.Provider = (*sms.TwilioProvider)(nil)
}
