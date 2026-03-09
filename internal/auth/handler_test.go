package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/testutil"
)

// Handler tests that don't require a database test the HTTP layer only:
// decoding, error mapping, response format. DB-dependent tests (register,
// login with real users) are in the integration test file.

func TestHandleRegisterValidation(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "invalid email",
			body:       `{"email":"notanemail","password":"12345678"}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "invalid email format",
		},
		{
			name:       "empty email",
			body:       `{"email":"","password":"12345678"}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "email is required",
		},
		{
			name:       "short password",
			body:       `{"email":"user@example.com","password":"short"}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "at least 8 characters",
		},
		{
			name:       "empty body",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "email is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			testutil.Equal(t, tt.wantStatus, w.Code)
			testutil.Contains(t, w.Body.String(), tt.wantMsg)
		})
	}
}

func TestHandleLoginValidation(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	// Login with no DB pool will fail at query level — we get an internal error.
	// But we can still test the malformed JSON path.
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandleMeWithoutToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleRegisterMalformedJSON(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandleRegisterBodyTooLarge(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	// Valid JSON structure that exceeds MaxBodySize. Without body-size
	// enforcement this would parse as valid JSON and proceed to validation
	// (returning "invalid email format" or similar). The MaxBytesReader
	// truncates the read so json.Decode fails with "invalid JSON body".
	padding := bytes.Repeat([]byte("a"), httputil.MaxBodySize)
	largeBody := append([]byte(`{"email":"`), padding...)
	largeBody = append(largeBody, []byte(`@example.com","password":"12345678"}`)...)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

// TestAuthResponseFormat removed — tested json.Marshal on a struct literal without
// exercising any handler. Auth response JSON shape is covered by integration tests.

func TestHandleRefreshMalformedJSON(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandleRefreshMissingToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "refreshToken is required")
}

func TestHandleLogoutMissingToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "refreshToken is required")
}

func TestHandlePasswordResetMissingEmail(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/password-reset", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "email is required")
}

func TestHandlePasswordResetMalformedJSON(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/password-reset", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandlePasswordResetConfirmMissingToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/password-reset/confirm",
		strings.NewReader(`{"password":"newpassword123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "token is required")
}

func TestHandlePasswordResetConfirmMissingPassword(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/password-reset/confirm",
		strings.NewReader(`{"token":"sometoken"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "password is required")
}

func TestHandleVerifyEmailMissingToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/verify", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "token is required")
}

func TestHandleResendVerificationNoAuth(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/verify/resend", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestHandleDeleteMeWithoutToken removed — exact duplicate of TestHandleDeleteMeRouteRegistered
// which additionally asserts on the error message body.

func TestHandleDeleteMeRouteRegistered(t *testing.T) {
	// Verify the DELETE /me route is registered and requires auth.
	// With no auth token, we should get 401 (proving the route+middleware are wired).
	t.Parallel()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodDelete, "/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 401 proves the route exists and requires auth (405 would mean no route).
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "missing or invalid authorization")
}

func TestHandlePasswordResetAlwaysReturns200(t *testing.T) {
	// Even with no DB pool (will fail internally), password-reset
	// should always return 200 to prevent email enumeration.
	t.Parallel()

	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/password-reset",
		strings.NewReader(`{"email":"nonexistent@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "if that email exists")
}
