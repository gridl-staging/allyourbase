package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestGenerateBackupCode_Format(t *testing.T) {
	t.Parallel()
	code, err := generateBackupCode()
	testutil.NoError(t, err)

	// Format: xxxxx-xxxxx (11 chars with hyphen).
	testutil.Equal(t, 11, len(code))
	parts := strings.Split(code, "-")
	testutil.Equal(t, 2, len(parts))
	testutil.Equal(t, 5, len(parts[0]))
	testutil.Equal(t, 5, len(parts[1]))

	// Only contains valid alphabet characters.
	for _, c := range strings.ReplaceAll(code, "-", "") {
		testutil.True(t, strings.ContainsRune(backupCodeAlphabet, c),
			"code should only contain valid characters, got: "+string(c))
	}
}

func TestGenerateBackupCode_Unique(t *testing.T) {
	t.Parallel()
	codes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code, err := generateBackupCode()
		testutil.NoError(t, err)
		testutil.True(t, !codes[code], "backup codes should be unique")
		codes[code] = true
	}
}

func TestBackupCodeAlphabet_NoAmbiguousCharacters(t *testing.T) {
	t.Parallel()
	// Backup codes must not contain characters that are visually ambiguous:
	// 0/O, 1/l/I. This prevents user-entry errors when typing codes manually.
	ambiguous := "0O1lI"
	for _, c := range ambiguous {
		testutil.True(t, !strings.ContainsRune(backupCodeAlphabet, c),
			"backupCodeAlphabet should not contain ambiguous character: "+string(c))
	}
	// Sanity: alphabet should have reasonable length for entropy.
	testutil.True(t, len(backupCodeAlphabet) >= 20,
		"backupCodeAlphabet should have at least 20 characters for entropy")
}

// --- Backup Code Handler Route Tests ---

func TestBackupVerifyRoute_RequiresMFAPending(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	// Regular auth token should be rejected by RequireMFAPending.
	user := &User{ID: "user-backup-verify-1", Email: "backup@example.com"}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/backup/verify", `{"code":"abcde-fghij"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBackupVerifyRoute_MissingCode(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	user := &User{ID: "user-backup-verify-2", Email: "backup2@example.com"}
	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/backup/verify", `{}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "code")
}

func TestBackupCountRoute_RequiresAuth(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := newTestRequest(t, "GET", "/mfa/backup/count", nil)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBackupVerifyRoute_NotAuthenticated(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := newTestRequest(t, "POST", "/mfa/backup/verify", `{"code":"abcde-fghij"}`)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBackupRegenerateRoute_RequiresAuth(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	req := newTestRequest(t, "POST", "/mfa/backup/regenerate", nil)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBackupRegenerateRoute_RequiresAAL2(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	user := &User{ID: "user-backup-regen-aal1", Email: "regen-aal1@example.com"}
	token, err := svc.generateToken(context.Background(), user) // AAL1 by default
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/backup/regenerate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
}

func TestBackupRegenerateRoute_AAL2PassesMiddleware(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	user := &User{ID: "user-backup-regen-aal2", Email: "regen-aal2@example.com"}
	token, err := svc.generateTokenWithOpts(context.Background(), user, &tokenOptions{AAL: "aal2", AMR: []string{"password", "totp"}})
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/backup/regenerate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	// With no DB configured in test service, route should reach handler and fail at service call.
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestBackupGenerateRoute_RejectsMissingSubject(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	token, err := svc.generateTokenWithOpts(context.Background(), &User{ID: "", Email: "no-subject-generate@example.com"}, &tokenOptions{
		AAL: "aal2",
		AMR: []string{"password", "totp"},
	})
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/backup/generate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBackupRegenerateRoute_RejectsMissingSubject(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	token, err := svc.generateTokenWithOpts(context.Background(), &User{ID: "", Email: "no-subject-regenerate@example.com"}, &tokenOptions{
		AAL: "aal2",
		AMR: []string{"password", "totp"},
	})
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/backup/regenerate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBackupCountRoute_RejectsMissingSubject(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	token, err := svc.generateToken(context.Background(), &User{ID: "", Email: "no-subject-count@example.com"})
	testutil.NoError(t, err)

	req := newTestRequest(t, "GET", "/mfa/backup/count", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBackupVerifyRoute_LockedOut(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	user := &User{ID: "user-backup-lockout", Email: "backup-lockout@example.com"}
	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)

	// Simulate lockout by recording failures beyond threshold.
	for i := 0; i < emailMFALockoutCount; i++ {
		svc.mfaFailureTracker.recordFailure("user-backup-lockout")
	}

	req := newTestRequest(t, "POST", "/mfa/backup/verify", `{"code":"abcde-fghij"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Contains(t, w.Body.String(), "too many failed attempts")
}

func TestBackupVerifyRoute_RejectsMissingSubject(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	token, err := svc.generateMFAPendingToken(&User{ID: "", Email: "no-subject-verify@example.com"})
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/backup/verify", `{"code":"abcde-fghij"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}
