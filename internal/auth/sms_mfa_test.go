package auth

import (
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
	testutil.NotNil(t, claims.ExpiresAt)
	testutil.NotNil(t, claims.IssuedAt)

	// Verify token expires in ≤5 minutes.
	dur := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	testutil.True(t, dur <= 5*time.Minute+time.Second,
		"MFA pending token should expire in ≤5 min, got %v", dur)
	testutil.True(t, dur >= 4*time.Minute,
		"MFA pending token should expire in ~5 min, got %v", dur)
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
