package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"net/http"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- TOTP Algorithm Tests (pure unit, no DB) ---

func TestGenerateTOTPCode_Deterministic(t *testing.T) {
	t.Parallel()
	secret := []byte("12345678901234567890") // 20 bytes
	step := int64(1)
	code1 := generateTOTPCode(secret, step)
	code2 := generateTOTPCode(secret, step)
	testutil.Equal(t, code1, code2)
	testutil.Equal(t, 6, len(code1))
}

func TestGenerateTOTPCode_DifferentSteps(t *testing.T) {
	t.Parallel()
	secret := []byte("12345678901234567890")
	code1 := generateTOTPCode(secret, 1)
	code2 := generateTOTPCode(secret, 2)
	// Different time steps should produce different codes (overwhelmingly likely).
	testutil.True(t, code1 != code2, "different time steps should produce different codes")
}

func TestValidateTOTPCode_CurrentStep(t *testing.T) {
	t.Parallel()
	secret := []byte("12345678901234567890")
	now := time.Now()
	step := now.Unix() / totpPeriod
	code := generateTOTPCode(secret, step)

	matchedStep, ok := validateTOTPCode(secret, code, now)
	testutil.True(t, ok, "code at current step should be valid")
	testutil.Equal(t, step, matchedStep)
}

func TestValidateTOTPCode_PreviousStep(t *testing.T) {
	t.Parallel()
	secret := []byte("12345678901234567890")
	now := time.Now()
	step := now.Unix()/totpPeriod - 1 // one period ago
	code := generateTOTPCode(secret, step)

	matchedStep, ok := validateTOTPCode(secret, code, now)
	testutil.True(t, ok, "code at previous step should be valid (skew=1)")
	testutil.Equal(t, step, matchedStep)
}

func TestValidateTOTPCode_NextStep(t *testing.T) {
	t.Parallel()
	secret := []byte("12345678901234567890")
	now := time.Now()
	step := now.Unix()/totpPeriod + 1 // one period ahead
	code := generateTOTPCode(secret, step)

	matchedStep, ok := validateTOTPCode(secret, code, now)
	testutil.True(t, ok, "code at next step should be valid (skew=1)")
	testutil.Equal(t, step, matchedStep)
}

func TestValidateTOTPCode_TooOld(t *testing.T) {
	t.Parallel()
	secret := []byte("12345678901234567890")
	now := time.Now()
	step := now.Unix()/totpPeriod - 2 // two periods ago — outside skew
	code := generateTOTPCode(secret, step)

	_, ok := validateTOTPCode(secret, code, now)
	testutil.True(t, !ok, "code two steps ago should be rejected")
}

func TestValidateTOTPCode_WrongCode(t *testing.T) {
	t.Parallel()
	secret := []byte("12345678901234567890")
	_, ok := validateTOTPCode(secret, "000000", time.Now())
	// Note: extremely unlikely to match. If it does, this is a 1-in-333333 flake.
	testutil.True(t, !ok, "wrong code should be rejected")
}

func TestValidateTOTPCode_ReturnsDeterministicStep(t *testing.T) {
	// Verify that validateTOTPCode returns the same step for the same code+time.
	// The caller uses this step for replay detection by comparing step <= lastUsedStep.
	t.Parallel()
	secret := []byte("12345678901234567890")
	now := time.Now()
	currentStep := now.Unix() / totpPeriod
	code := generateTOTPCode(secret, currentStep)

	// First validation succeeds and returns the step.
	step1, ok1 := validateTOTPCode(secret, code, now)
	testutil.True(t, ok1, "first validation should succeed")
	testutil.Equal(t, currentStep, step1)

	// Same code at same time returns the same step deterministically.
	step2, ok2 := validateTOTPCode(secret, code, now)
	testutil.True(t, ok2, "same code at same time still validates")
	testutil.Equal(t, step1, step2)
}

func TestTOTPConstants_MatchDocumentedSpec(t *testing.T) {
	// These constants match the TOTP spec documentation: SHA-1, 6 digits, 30s period, ±1 skew.
	t.Parallel()
	testutil.Equal(t, 6, totpDigits)
	testutil.Equal(t, int64(30), int64(totpPeriod))
	testutil.Equal(t, 1, totpSkew)
	testutil.Equal(t, 20, totpKeyLen)
}

// --- Encryption Tests (pure unit) ---

func TestEncryptDecryptAESGCM(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	testutil.NoError(t, svc.SetEncryptionKey(key))

	plaintext := []byte("this is a secret TOTP key")
	ciphertext, err := svc.encryptAESGCM(plaintext)
	testutil.NoError(t, err)
	testutil.True(t, len(ciphertext) > len(plaintext), "ciphertext should be longer than plaintext")

	decrypted, err := svc.decryptAESGCM(ciphertext)
	testutil.NoError(t, err)
	testutil.Equal(t, string(plaintext), string(decrypted))
}

func TestEncryptAESGCM_NoKey(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	_, err := svc.encryptAESGCM([]byte("data"))
	testutil.True(t, err != nil, "should fail without encryption key")
}

func TestDecryptAESGCM_NoKey(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	_, err := svc.decryptAESGCM([]byte("data"))
	testutil.True(t, err != nil, "should fail without encryption key")
}

func TestSetEncryptionKey_WrongSize(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	err := svc.SetEncryptionKey([]byte("too-short"))
	testutil.True(t, err != nil, "should reject non-32-byte key")
}

func TestDecryptAESGCM_TamperedData(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	key := make([]byte, 32)
	rand.Read(key)
	svc.SetEncryptionKey(key)

	ciphertext, err := svc.encryptAESGCM([]byte("secret"))
	testutil.NoError(t, err)

	// Tamper with the ciphertext.
	ciphertext[len(ciphertext)-1] ^= 0xff
	_, err = svc.decryptAESGCM(ciphertext)
	testutil.True(t, err != nil, "tampered ciphertext should fail decryption")
}

// --- OTP Auth URI Tests ---

func TestBuildOTPAuthURI(t *testing.T) {
	t.Parallel()
	secret := []byte("12345678901234567890")
	uri := buildOTPAuthURI(secret, "user@example.com", "MyApp")

	testutil.Contains(t, uri, "otpauth://totp/")
	testutil.Contains(t, uri, "MyApp")
	testutil.Contains(t, uri, "user@example.com")

	b32 := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	testutil.Contains(t, uri, "secret="+b32)
	testutil.Contains(t, uri, "algorithm=SHA1")
	testutil.Contains(t, uri, "digits=6")
	testutil.Contains(t, uri, "period=30")
}

// --- TOTP Handler Route Tests (no DB, validate routing + middleware) ---

func TestTOTPEnrollRoute_NotAuthenticated(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	req := newTestRequest(t, "POST", "/mfa/totp/enroll", nil)
	w := serveRequest(router, req)
	testutil.Equal(t, 401, w.Code)
}

func TestTOTPEnrollRoute_BlocksAnonymousUser(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	user := &User{ID: "anon-totp-123", IsAnonymous: true}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/totp/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 403, w.Code)
	testutil.Contains(t, w.Body.String(), "link your account")
}

func TestTOTPEnrollRoute_Disabled(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	// totpEnabled defaults to false
	router := h.Routes()

	user := &User{ID: "user-totp-1", Email: "totp@example.com"}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/totp/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 404, w.Code)
}

func TestTOTPChallengeRoute_RequiresMFAPending(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	// Regular auth token (not MFA pending) should be rejected.
	user := &User{ID: "user-totp-2", Email: "totp2@example.com"}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/totp/challenge", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 401, w.Code)
}

func TestTOTPVerifyRoute_RequiresMFAPending(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-totp-3", Email: "totp3@example.com"}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/totp/verify", `{"challenge_id":"abc","code":"123456"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 401, w.Code)
}

func TestMFAFactorsRoute_RequiresAuth(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	req := newTestRequest(t, "GET", "/mfa/factors", nil)
	w := serveRequest(router, req)
	testutil.Equal(t, 401, w.Code)
}

func TestMFAFactorsRoute_AllowsMFAPendingToken(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-mfa-factors-pending", Email: "pending@example.com"}
	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "GET", "/mfa/factors", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)

	// With no DB in this unit-test service, successful auth should proceed to handler and fail at DB access.
	testutil.Equal(t, 500, w.Code)
}

func TestMFAFactorsRoute_RejectsMissingSubject(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	token, err := svc.generateToken(context.Background(), &User{ID: "", Email: "no-subject@example.com"})
	testutil.NoError(t, err)

	req := newTestRequest(t, "GET", "/mfa/factors", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBackupGenerateRoute_RequiresAAL2(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	router := h.Routes()

	user := &User{ID: "user-backup-aal1", Email: "aal1@example.com"}
	token, err := svc.generateToken(context.Background(), user) // AAL1 by default
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/backup/generate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 403, w.Code)
}

func TestTOTPEnrollConfirmRoute_BlocksAnonymousUser(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	user := &User{ID: "anon-totp-confirm-1", IsAnonymous: true}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/totp/enroll/confirm", `{"code":"123456"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 403, w.Code)
	testutil.Contains(t, w.Body.String(), "link your account")
}

func TestTOTPEnrollConfirmRoute_NotAuthenticated(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	req := newTestRequest(t, "POST", "/mfa/totp/enroll/confirm", `{"code":"123456"}`)
	w := serveRequest(router, req)
	testutil.Equal(t, 401, w.Code)
}

func TestTOTPVerifyRoute_MissingFields(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-totp-fields", Email: "fields@example.com"}
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
			req := newTestRequest(t, "POST", "/mfa/totp/verify", tc.body)
			req.Header.Set("Authorization", "Bearer "+token)
			w := serveRequest(router, req)
			testutil.Equal(t, 400, w.Code)
		})
	}
}

func TestTOTPEnrollConfirmRoute_MissingCode(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-totp-confirm-empty", Email: "confirm@example.com"}
	token, err := svc.generateToken(context.Background(), user)
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/totp/enroll/confirm", `{}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, 400, w.Code)
	testutil.Contains(t, w.Body.String(), "code")
}

// --- AAL2 Enrollment Guard Tests ---

func TestTOTPEnrollRoute_AAL2RequiredWhenExistingMFA(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	// AAL1 token — if the user has existing MFA, the handler should reject enrollment.
	user := &User{ID: "user-totp-aal2-guard", Email: "guard@example.com"}
	token, err := svc.generateToken(context.Background(), user) // generates AAL1 by default
	testutil.NoError(t, err)

	// Set the override to simulate existing MFA factor.
	h.SetExistingMFAOverride(true)

	req := newTestRequest(t, "POST", "/mfa/totp/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "AAL2")
}

func TestTOTPEnrollRoute_AAL2PassesThroughWithExistingMFA(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	// AAL2 token should be allowed even with existing MFA.
	user := &User{ID: "user-totp-aal2-pass", Email: "pass@example.com"}
	token, err := svc.generateTokenWithOpts(context.Background(), user, &tokenOptions{AAL: "aal2", AMR: []string{"password", "sms"}})
	testutil.NoError(t, err)

	h.SetExistingMFAOverride(true)

	req := newTestRequest(t, "POST", "/mfa/totp/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	// Should pass AAL2 check but fail at DB (nil pool) → 500.
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- Unverified TOTP Enrollment Cleanup ---

func TestCleanupUnverifiedTOTPEnrollments_NilPool(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	err := svc.CleanupUnverifiedTOTPEnrollments(context.Background(), 10*time.Minute)
	testutil.True(t, err != nil, "should fail with nil pool")
}

func TestHasTOTPMFA_NilPool(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	_, err := svc.HasTOTPMFA(context.Background(), "user-1")
	testutil.True(t, err != nil, "should fail with nil pool")
}

func TestDefaultUnverifiedTOTPTTL(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, 10*time.Minute, DefaultUnverifiedTOTPTTL)
}

func TestMFAChallengeIP_ParsesRemoteAddrHostPort(t *testing.T) {
	t.Parallel()
	req := newTestRequest(t, "POST", "/mfa/totp/challenge", nil)
	req.RemoteAddr = "198.51.100.10:12345"
	testutil.Equal(t, "198.51.100.10", mfaChallengeIP(req))
}

func TestMFAChallengeIP_UsesTrustedProxyForwardedIP(t *testing.T) {
	t.Parallel()
	req := newTestRequest(t, "POST", "/mfa/totp/challenge", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	testutil.Equal(t, "203.0.113.50", mfaChallengeIP(req))
}

// --- RequireAAL2 Middleware ---

func TestRequireAAL2_RejectsAAL1(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	user := &User{ID: "user-aal1", Email: "aal1@example.com"}
	token, err := svc.generateToken(context.Background(), user) // AAL1 by default
	testutil.NoError(t, err)

	called := false
	handler := RequireAuth(svc)(RequireAAL2(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})))

	req := newTestRequest(t, http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(handler, req)

	testutil.Equal(t, 403, w.Code)
	testutil.False(t, called, "AAL1 token should not pass RequireAAL2")
	testutil.Contains(t, w.Body.String(), "insufficient_aal")
}

func TestRequireAAL2_AcceptsAAL2(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	user := &User{ID: "user-aal2", Email: "aal2@example.com"}
	token, err := svc.generateTokenWithOpts(context.Background(), user, &tokenOptions{AAL: "aal2", AMR: []string{"password", "totp"}})
	testutil.NoError(t, err)

	called := false
	handler := RequireAuth(svc)(RequireAAL2(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})))

	req := newTestRequest(t, http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(handler, req)

	testutil.Equal(t, 200, w.Code)
	testutil.True(t, called, "AAL2 token should pass RequireAAL2")
}

func TestTOTPEnrollRoute_AAL2GuardFailsClosedOnDBError(t *testing.T) {
	t.Parallel()
	svc := newTestService() // nil pool — HasAnyMFA will return error
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	// Deliberately NOT setting existingMFAOverride — forces real HasAnyMFA call with nil pool
	router := h.Routes()

	user := &User{ID: "user-totp-failclosed", Email: "failclosed@example.com"}
	token, err := svc.generateToken(context.Background(), user) // AAL1
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/totp/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	// Should fail-closed with 500, NOT pass through to enrollment
	testutil.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestTOTPEnrollRoute_RejectsMissingSubject(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	token, err := svc.generateToken(context.Background(), &User{ID: "", Email: "no-subject-enroll@example.com"})
	testutil.NoError(t, err)

	req := newTestRequest(t, "POST", "/mfa/totp/enroll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestTOTPVerifyRoute_LockedOut(t *testing.T) {
	t.Parallel()
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.SetTOTPEnabled(true)
	router := h.Routes()

	user := &User{ID: "user-totp-lockout", Email: "totp-lockout@example.com"}
	token, err := svc.generateMFAPendingToken(user)
	testutil.NoError(t, err)

	// Simulate lockout by recording failures beyond threshold.
	for i := 0; i < emailMFALockoutCount; i++ {
		svc.mfaFailureTracker.recordFailure("user-totp-lockout")
	}

	req := newTestRequest(t, "POST", "/mfa/totp/verify", `{"challenge_id":"abc","code":"123456"}`)
	req.Header.Set("Authorization", "Bearer "+token)
	w := serveRequest(router, req)
	testutil.Equal(t, http.StatusTooManyRequests, w.Code)
	testutil.Contains(t, w.Body.String(), "too many failed attempts")
}

func TestMaskEmail(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "t***@example.com", maskEmail("test@example.com"))
	testutil.Equal(t, "a***@b.com", maskEmail("a@b.com"))
	testutil.Equal(t, "u***@x.co", maskEmail("user123@x.co"))
	testutil.Equal(t, "***", maskEmail("noatsign"))
	testutil.Equal(t, "***", maskEmail(""))
}
