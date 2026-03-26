package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Email MFA Code Generation Tests (pure unit, no DB) ---

func TestGenerateEmailMFACode_Format(t *testing.T) {
	t.Parallel()
	code, err := generateEmailMFACode()
	testutil.NoError(t, err)
	testutil.Equal(t, 6, len(code))
	// All digits.
	for _, c := range code {
		testutil.True(t, c >= '0' && c <= '9', "code should be all digits, got: "+string(c))
	}
}

func TestGenerateEmailMFACode_Unique(t *testing.T) {
	t.Parallel()
	codes := make(map[string]bool)
	for i := 0; i < 50; i++ {
		code, err := generateEmailMFACode()
		testutil.NoError(t, err)
		codes[code] = true
	}
	// With a 6-digit code space (1M possibilities), 50 samples from crypto/rand
	// should produce zero collisions. Allow at most 1 for extreme flake tolerance.
	testutil.True(t, len(codes) >= 49, "expected at least 49 unique codes out of 50")
}

// --- Email MFA Handler Route Tests (no DB, validate routing + middleware) ---

func TestEmailMFAEnrollRoute_NotAuthenticated(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	req := newTestRequest(t, "POST", "/mfa/email/enroll", nil)
	w := serveRequest(router, req)
	testutil.Equal(t, 401, w.Code)
}

func TestEmailMFAEnrollRoute_BlocksAnonymousUser(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	user := &User{ID: "anon-email-mfa-1", IsAnonymous: true}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/email/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 403, w.Code)
	testutil.Contains(t, w.Body.String(), "link your account")
}

func TestEmailMFAEnrollRoute_Disabled(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	// emailMFAEnabled defaults to false
	router := h.Routes()

	user := &User{ID: "user-email-mfa-1", Email: "mfa@example.com"}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/email/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 404, w.Code)
}

func TestEmailMFAEnrollConfirmRoute_BlocksAnonymousUser(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	user := &User{ID: "anon-email-confirm-1", IsAnonymous: true}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/email/enroll/confirm", `{"code":"123456"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 403, w.Code)
	testutil.Contains(t, w.Body.String(), "link your account")
}

func TestEmailMFAEnrollConfirmRoute_NotAuthenticated(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	req := newTestRequest(t, "POST", "/mfa/email/enroll/confirm", `{"code":"123456"}`)
	w := serveRequest(router, req)
	testutil.Equal(t, 401, w.Code)
}

func TestEmailMFAChallengeRoute_RequiresMFAPending(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	// Regular auth token (not MFA pending) should be rejected.
	user := &User{ID: "user-email-mfa-2", Email: "mfa2@example.com"}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/email/challenge", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 401, w.Code)
}

func TestEmailMFAVerifyRoute_RequiresMFAPending(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-email-mfa-3", Email: "mfa3@example.com"}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/email/verify", `{"challenge_id":"abc","code":"123456"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 401, w.Code)
}

func TestEmailMFAChallengeRoute_AllowsMFAPending(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-email-mfa-pending", Email: "pending@example.com"}
	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/email/challenge", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	// Should reach handler, fail at DB access (no pool).
	testutil.Equal(t, 500, w.Code)
}

func TestEmailMFAVerifyRoute_MissingFields(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-email-mfa-fields", Email: "fields@example.com"}
	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)

	cases := []struct {
		name string
		body string
	}{
		{"missing challenge_id", `{"code":"123456"}`},
		{"missing code", `{"challenge_id":"abc"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := newTestRequest(t, "POST", "/mfa/email/verify", tc.body)
			req.Header.Set("Authorization", "Bearer "+token)
			w := serveRequest(router, req)
			testutil.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// --- Email MFA AAL2 Enrollment Guard Tests ---

func TestEmailMFAEnrollRoute_AAL2RequiredWhenExistingMFA(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	// AAL1 token with existing MFA override → should be blocked.
	user := &User{ID: "user-email-aal2-guard", Email: "emailguard@example.com"}
	token, err := svc.generateToken(context.Background(), user) // AAL1
	testutil.NoError(t, err)

	h.SetExistingMFAOverride(true)

	req := newTestRequest(t, "POST", "/mfa/email/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "AAL2")
}

func TestEmailMFAEnrollRoute_AAL2PassesThroughWithExistingMFA(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-email-aal2-pass", Email: "emailpass@example.com"}
	token, err := svc.generateTokenWithOpts(context.Background(), user, &tokenOptions{AAL: "aal2", AMR: []string{"password", "totp"}})
	testutil.NoError(t, err)

	h.SetExistingMFAOverride(true)

	req := newTestRequest(t, "POST", "/mfa/email/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	// Passes AAL2 check, fails at DB → 500.
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- Email MFA Lockout Handler Tests ---

func TestEmailMFAVerifyRoute_LockedOut(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-email-lockout", Email: "lockout@example.com"}
	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)

	// Simulate lockout by recording failures beyond threshold.
	for i := 0; i < emailMFALockoutCount; i++ {
		svc.mfaFailureTracker.recordFailure("user-email-lockout")
	}

	req := newTestRequest(t, "POST", "/mfa/email/verify", `{"challenge_id":"abc","code":"123456"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Contains(t, w.Body.String(), "too many failed attempts")
}

// --- Email MFA Lockout Logic (pure unit, no DB) ---

func TestEmailMFALockout_NotLockedInitially(t *testing.T) {
	t.Parallel()
	tracker := newMFAFailureTracker(15, time.Hour, 30*time.Minute)
	testutil.False(t, tracker.isLocked("user-1"), "should not be locked initially")
}

func TestEmailMFALockout_LocksAfterThreshold(t *testing.T) {
	t.Parallel()
	tracker := newMFAFailureTracker(3, time.Hour, 30*time.Minute)
	for i := 0; i < 3; i++ {
		tracker.recordFailure("user-1")
	}
	testutil.True(t, tracker.isLocked("user-1"), "should be locked after 3 failures")
}

func TestEmailMFALockout_DifferentUsersIndependent(t *testing.T) {
	t.Parallel()
	tracker := newMFAFailureTracker(3, time.Hour, 30*time.Minute)
	for i := 0; i < 3; i++ {
		tracker.recordFailure("user-1")
	}
	testutil.True(t, tracker.isLocked("user-1"), "user-1 should be locked")
	testutil.False(t, tracker.isLocked("user-2"), "user-2 should not be locked")
}

func TestEmailMFALockout_ExpiresAfterWindow(t *testing.T) {
	t.Parallel()
	tracker := newMFAFailureTracker(3, 50*time.Millisecond, 50*time.Millisecond)
	for i := 0; i < 3; i++ {
		tracker.recordFailure("user-1")
	}
	testutil.True(t, tracker.isLocked("user-1"), "should be locked")
	time.Sleep(80 * time.Millisecond)
	testutil.False(t, tracker.isLocked("user-1"), "lock should expire")
}

func TestEmailMFALockout_ResetOnSuccess(t *testing.T) {
	t.Parallel()
	tracker := newMFAFailureTracker(3, time.Hour, 30*time.Minute)
	for i := 0; i < 2; i++ {
		tracker.recordFailure("user-1")
	}
	tracker.reset("user-1")
	testutil.False(t, tracker.isLocked("user-1"), "should not be locked after reset")
}

// --- Email MFA Configuration Constants ---

func TestEmailMFAConstants_MatchDocumentedLimits(t *testing.T) {
	t.Parallel()
	// These constants are documented in the stage checklist and API docs.
	// Changing them silently would break documented behavior.
	testutil.Equal(t, 6, emailMFACodeLen)
	testutil.Equal(t, 10*time.Minute, emailMFACodeExpiry)
	testutil.Equal(t, 5, emailMFAMaxAttempts)
	testutil.Equal(t, 3, emailMFAMaxChallenges)
	testutil.Equal(t, 15, emailMFALockoutCount)
	testutil.Equal(t, time.Hour, emailMFALockoutWindow)
	testutil.Equal(t, 30*time.Minute, emailMFALockoutDuration)
}

// --- Email MFA Enroll Confirm Handler: Missing Code ---

func TestEmailMFAEnrollConfirmRoute_MissingCode(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-email-confirm-empty", Email: "confirm-empty@example.com"}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/email/enroll/confirm", `{}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "code")
}

// --- Email MFA Lockout Handler Integration ---

func TestEmailMFAEnrollRoute_RejectsMissingSubject(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetEmailMFAEnabled(true)
	router := h.Routes()

	token, err := svc.generateToken(context.Background(), &User{ID: "", Email: "no-subject-enroll@example.com"})
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/email/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}
