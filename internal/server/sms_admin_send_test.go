package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAdminSMSSend_NoSMSProvider_Returns404(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t, func(s *Server) {
		s.smsProvider = nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sms/send", strings.NewReader(`{"to":"+15551234567","body":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.handleAdminSMSSend(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "SMS is not enabled")
}

func TestAdminSMSSend_EmptyTo_Returns400(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sms/send", strings.NewReader(`{"to":"","body":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.handleAdminSMSSend(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "to is required")
}

func TestAdminSMSSend_EmptyBody_Returns400(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sms/send", strings.NewReader(`{"to":"+12025551234","body":""}`))
	req.Header.Set("Content-Type", "application/json")
	srv.handleAdminSMSSend(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "body is required")
}

func TestAdminSMSSend_InvalidPhone_Returns400(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sms/send", strings.NewReader(`{"to":"notaphone","body":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.handleAdminSMSSend(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid phone number")
}

func TestAdminSMSSend_CountryNotAllowed_Returns400(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t, func(s *Server) {
		s.smsAllowedCountries = []string{"GB"} // only UK allowed
	})

	w := httptest.NewRecorder()
	// US number, but only GB allowed.
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sms/send", strings.NewReader(`{"to":"+12025551234","body":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.handleAdminSMSSend(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "phone number country not allowed")
}

func TestAdminSMSSend_BodyTooLong_Returns400(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t)

	longBody := strings.Repeat("x", 1601)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sms/send",
		strings.NewReader(fmt.Sprintf(`{"to":"+12025551234","body":"%s"}`, longBody)))
	req.Header.Set("Content-Type", "application/json")
	srv.handleAdminSMSSend(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "body exceeds maximum length")
}

func TestAdminSMSSend_Success_Returns200(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t, func(s *Server) {
		s.smsProvider = &mockSMSProvider{
			result: &sms.SendResult{MessageID: "SM_test", Status: "queued"},
		}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sms/send",
		strings.NewReader(`{"to":"+12025551234","body":"test message"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.handleAdminSMSSend(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, "SM_test", resp["message_id"])
	testutil.Equal(t, "queued", resp["status"])
	testutil.Equal(t, "+12025551234", resp["to"])

	// Verify no "id" field — admin sends are not stored.
	_, hasID := resp["id"]
	testutil.True(t, !hasID, "admin send response should not have id field")
}

func TestAdminSMSSend_ProviderError_Returns500(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t, func(s *Server) {
		s.smsProvider = &mockSMSProvider{sendErr: errors.New("provider timeout")}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/sms/send",
		strings.NewReader(`{"to":"+12025551234","body":"test message"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.handleAdminSMSSend(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Contains(t, w.Body.String(), "provider timeout")
}
