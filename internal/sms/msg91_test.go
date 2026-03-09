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

func TestMSG91SendSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v5/flow/", r.URL.Path)

		// Verify authkey header
		assert.Equal(t, "MSG91_AUTH_KEY", r.Header.Get("authkey"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var reqBody struct {
			TemplateID string `json:"template_id"`
			Recipients []struct {
				Mobiles string `json:"mobiles"`
				OTP     string `json:"otp"`
			} `json:"recipients"`
		}
		require.NoError(t, json.Unmarshal(body, &reqBody))
		assert.Equal(t, "TMPL_123", reqBody.TemplateID)
		require.Len(t, reqBody.Recipients, 1)
		assert.Equal(t, "+15551234567", reqBody.Recipients[0].Mobiles)
		assert.Equal(t, "123456", reqBody.Recipients[0].OTP)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"success","message":"request-id-abc","request_id":"req-abc-123"}`))
	}))
	defer srv.Close()

	p := sms.NewMSG91Provider("MSG91_AUTH_KEY", "TMPL_123", srv.URL)
	result, err := p.Send(t.Context(), "+15551234567", "123456")
	require.NoError(t, err)
	assert.Equal(t, "req-abc-123", result.MessageID)
	assert.Equal(t, "success", result.Status)
}

func TestMSG91SendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","message":"invalid template"}`))
	}))
	defer srv.Close()

	p := sms.NewMSG91Provider("MSG91_AUTH_KEY", "TMPL_123", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid template")
}

func TestMSG91SendErrorHTTP200WithErrorType(t *testing.T) {
	// MSG91 API can return HTTP 200 with {"type":"error",...} in the body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"error","message":"invalid authkey"}`))
	}))
	defer srv.Close()

	p := sms.NewMSG91Provider("BAD_KEY", "TMPL_123", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid authkey")
}

func TestMSG91SendErrorNonJSON(t *testing.T) {
	// Proxy/CDN returning non-JSON error — must not produce a confusing parse error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`<html>Bad Gateway</html>`))
	}))
	defer srv.Close()

	p := sms.NewMSG91Provider("MSG91_AUTH_KEY", "TMPL_123", srv.URL)
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "msg91: error 502")
}

func TestMSG91SendNetworkError(t *testing.T) {
	p := sms.NewMSG91Provider("MSG91_AUTH_KEY", "TMPL_123", "http://127.0.0.1:1")
	_, err := p.Send(t.Context(), "+15551234567", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "msg91: send request:")
}

func TestMSG91ImplementsInterface(t *testing.T) {
	var _ sms.Provider = (*sms.MSG91Provider)(nil)
}
