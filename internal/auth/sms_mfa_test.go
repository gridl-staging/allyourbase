package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Step 2: MFA Claims & Pending Token ---

func TestGenerateMFAPendingToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	user := &User{ID: "550e8400-e29b-41d4-a716-446655440000", Email: "mfa@example.com"}

	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)
	testutil.True(t, token != "", "token should not be empty")

	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, claims.Subject)
	testutil.Equal(t, user.Email, claims.Email)
	testutil.True(t, claims.MFAPending, "MFAPending should be true")
	testutil.Equal(t, "aal1", claims.AAL)
	testutil.NotNil(t, claims.ExpiresAt)
	testutil.NotNil(t, claims.IssuedAt)

	// Verify token expires in ≤5 minutes.
	dur := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	testutil.True(t, dur <= 5*time.Minute+time.Second,
		"MFA pending token should expire in ≤5 min, got %v", dur)
	testutil.True(t, dur >= 4*time.Minute,
		"MFA pending token should expire in ~5 min, got %v", dur)
}

func TestGenerateMFAPendingTokenWithMethod(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	user := &User{ID: "73d4cb53-8cd6-4a0e-99ab-8dd433d9fd72", Email: "method@example.com"}

	token, err := svc.generateMFAPendingTokenWithMethod(user, "oauth")
	testutil.NoError(t, err)

	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal1", claims.AAL)
	testutil.Equal(t, 1, len(claims.AMR))
	testutil.Equal(t, "oauth", claims.AMR[0])
}

func TestMFAPendingToken_RejectedByRequireAuth(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	user := &User{ID: "550e8400-e29b-41d4-a716-446655440000", Email: "mfa@example.com"}

	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)

	called := false
	handler := RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
	testutil.False(t, called, "handler should not be called for MFA pending token")

	var resp map[string]any
	testutil.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	msg, _ := resp["message"].(string)
	testutil.Contains(t, msg, "MFA verification required")
}

// --- SMS MFA AAL2 Enrollment Guard Tests ---

func TestSMSMFAEnrollRoute_AAL2RequiredWhenExistingMFA(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetSMSEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-sms-aal2-guard", Email: "smsguard@example.com"}
	token, err := svc.generateToken(context.Background(), user) // AAL1
	testutil.NoError(t, err)

	h.SetExistingMFAOverride(true)

	req := newTestRequest(t, "POST", "/mfa/sms/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "AAL2")
}

func TestSMSMFAEnrollRoute_AAL2PassesThroughWithExistingMFA(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetSMSEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-sms-aal2-pass", Email: "smspass@example.com"}
	token, err := svc.generateTokenWithOpts(context.Background(), user, &tokenOptions{AAL: "aal2", AMR: []string{"password", "totp"}})
	testutil.NoError(t, err)

	h.SetExistingMFAOverride(true)

	req := newTestRequest(t, "POST", "/mfa/sms/enroll", `{}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	// Passes AAL2 check and reaches request validation.
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "phone is required")
}

func TestSMSMFAVerifyRoute_LockedOut(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetSMSEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-sms-lockout", Email: "sms-lockout@example.com"}
	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)

	// Simulate lockout by recording failures beyond threshold.
	for i := 0; i < emailMFALockoutCount; i++ {
		svc.mfaFailureTracker.recordFailure("user-sms-lockout")
	}

	req := newTestRequest(t, "POST", "/mfa/sms/verify", `{"code":"123456"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Contains(t, w.Body.String(), "too many failed attempts")
}

func TestSMSMFAEnrollConfirmRoute_BlocksAnonymousUser(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetSMSEnabled(true)
	router := h.Routes()

	user := &User{ID: "anon-sms-confirm-1", IsAnonymous: true}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/sms/enroll/confirm", `{"phone":"+15551234567","code":"123456"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 403, w.Code)
	testutil.Contains(t, w.Body.String(), "link your account")
}
