package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Service-level unit tests ---

func TestMagicLinkDurationDefault(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	testutil.Equal(t, magicLinkDefaultDur, svc.MagicLinkDuration())
}

func TestMagicLinkDurationCustom(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	svc.SetMagicLinkDuration(5 * time.Minute)
	testutil.Equal(t, 5*time.Minute, svc.MagicLinkDuration())
}

func TestRequestMagicLinkNoMailer(t *testing.T) {
	// With no mailer configured, RequestMagicLink is a no-op (returns nil).
	t.Parallel()

	svc := newTestService()
	err := svc.RequestMagicLink(context.TODO(), "user@example.com")
	testutil.NoError(t, err)
}

func TestErrInvalidMagicLinkToken(t *testing.T) {
	// Verify the sentinel error has a useful message.
	t.Parallel()

	testutil.Contains(t, ErrInvalidMagicLinkToken.Error(), "invalid or expired")
}

// --- Handler-level unit tests ---

func newMagicLinkHandler(enabled bool) *Handler {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.magicLinkEnabled = enabled
	return h
}

func TestHandleMagicLinkRequestDisabled(t *testing.T) {
	t.Parallel()
	h := newMagicLinkHandler(false)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/magic-link",
		strings.NewReader(`{"email":"user@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "not enabled")
}

func TestHandleMagicLinkConfirmDisabled(t *testing.T) {
	t.Parallel()
	h := newMagicLinkHandler(false)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/magic-link/confirm",
		strings.NewReader(`{"token":"sometoken"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "not enabled")
}

func TestHandleMagicLinkRequestMissingEmail(t *testing.T) {
	t.Parallel()
	h := newMagicLinkHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/magic-link",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "email is required")
}

func TestHandleMagicLinkRequestMalformedJSON(t *testing.T) {
	t.Parallel()
	h := newMagicLinkHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/magic-link",
		strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandleMagicLinkRequestAlwaysReturns200(t *testing.T) {
	// Even with no mailer/DB, the endpoint should return 200 (prevent enumeration).
	t.Parallel()

	h := newMagicLinkHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/magic-link",
		strings.NewReader(`{"email":"anyone@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "if valid, a login link has been sent")
}

func TestHandleMagicLinkConfirmMissingToken(t *testing.T) {
	t.Parallel()
	h := newMagicLinkHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/magic-link/confirm",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "token is required")
}

func TestHandleMagicLinkConfirmMalformedJSON(t *testing.T) {
	t.Parallel()
	h := newMagicLinkHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/magic-link/confirm",
		strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

func TestHandleMagicLinkRoutesRegistered(t *testing.T) {
	// Verify both magic link routes are registered and respond (not 405 Method Not Allowed).
	t.Parallel()

	h := newMagicLinkHandler(true)
	router := h.Routes()

	// POST /magic-link should work (not 405).
	req := httptest.NewRequest(http.MethodPost, "/magic-link",
		strings.NewReader(`{"email":"test@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	testutil.True(t, w.Code != http.StatusMethodNotAllowed,
		"POST /magic-link should be registered, got %d", w.Code)

	// POST /magic-link/confirm should work (not 405).
	// Send empty token so validation stops before hitting DB.
	req2 := httptest.NewRequest(http.MethodPost, "/magic-link/confirm",
		strings.NewReader(`{}`))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	testutil.True(t, w2.Code != http.StatusMethodNotAllowed,
		"POST /magic-link/confirm should be registered, got %d", w2.Code)
	testutil.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestHandleMagicLinkRequestEmptyEmailString(t *testing.T) {
	t.Parallel()
	h := newMagicLinkHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/magic-link",
		strings.NewReader(`{"email":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "email is required")
}

func TestSetMagicLinkEnabled(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger())
	testutil.False(t, h.magicLinkEnabled, "magic link should be disabled by default")

	h.SetMagicLinkEnabled(true)
	testutil.True(t, h.magicLinkEnabled, "magic link should be enabled after SetMagicLinkEnabled(true)")

	h.SetMagicLinkEnabled(false)
	testutil.False(t, h.magicLinkEnabled, "magic link should be disabled after SetMagicLinkEnabled(false)")
}
