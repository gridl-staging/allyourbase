//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

const sessionsAdminPassword = "session-admin-password"

type sessionAPIResponse struct {
	ID           string    `json:"id"`
	UserAgent    string    `json:"user_agent"`
	IPAddress    string    `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	Current      bool      `json:"current"`
}

func doJSONWithHeaders(
	t *testing.T,
	srv *server.Server,
	method, path string,
	body any,
	token string,
	headers map[string]string,
	remoteAddr string,
) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("doJSONWithHeaders: marshal body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}

	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

func parseSessionsResponse(t *testing.T, w *httptest.ResponseRecorder) []sessionAPIResponse {
	t.Helper()
	var sessions []sessionAPIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("parsing sessions response: %v (body: %s)", err, w.Body.String())
	}
	return sessions
}

func setupAuthServerWithAdmin(t *testing.T, ctx context.Context) *server.Server {
	t.Helper()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Admin.Password = sessionsAdminPassword

	authSvc := newAuthService()
	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
}

func adminLoginForSessions(t *testing.T, srv *server.Server) string {
	t.Helper()
	w := doJSON(t, srv, http.MethodPost, "/api/admin/auth", map[string]string{
		"password": sessionsAdminPassword,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var resp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token := resp["token"]
	testutil.True(t, token != "", "admin login should return token")
	return token
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TestSessionsListIncludesMetadataAndCurrentFlag(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	registerHeaders := map[string]string{
		"User-Agent":      "SessionTestBrowser/1.0",
		"X-Forwarded-For": "203.0.113.10, 70.41.3.18",
	}
	w := doJSONWithHeaders(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "sessions-list@example.com",
		"password": "password123",
	}, "", registerHeaders, "127.0.0.1:12345")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)

	w = doJSON(t, srv, http.MethodGet, "/api/auth/sessions", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	sessions := parseSessionsResponse(t, w)
	testutil.Equal(t, 1, len(sessions))
	testutil.True(t, sessions[0].ID != "", "session id should be present")
	testutil.Equal(t, "SessionTestBrowser/1.0", sessions[0].UserAgent)
	testutil.Equal(t, "203.0.113.10", sessions[0].IPAddress)
	testutil.True(t, sessions[0].Current, "registered session should be marked current")
	testutil.True(t, !sessions[0].CreatedAt.IsZero(), "created_at should be set")
	testutil.True(t, !sessions[0].LastActiveAt.IsZero(), "last_active_at should be set")
}

func TestSessionsRevokeSingleInvalidatesRefreshToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "sessions-revoke-one@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)

	w = doJSON(t, srv, http.MethodGet, "/api/auth/sessions", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	sessions := parseSessionsResponse(t, w)
	testutil.Equal(t, 1, len(sessions))

	w = doJSON(t, srv, http.MethodDelete, "/api/auth/sessions/"+sessions[0].ID, nil, resp.Token)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestSessionRevocation_DenyListBlocksRevokedToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "sessions-revoke-denylist@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)

	// Token works before revoke.
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Revoke current session.
	w = doJSON(t, srv, http.MethodGet, "/api/auth/sessions", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	sessions := parseSessionsResponse(t, w)
	testutil.Equal(t, 1, len(sessions))

	w = doJSON(t, srv, http.MethodDelete, "/api/auth/sessions/"+sessions[0].ID, nil, resp.Token)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	// Same access token should now be rejected.
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, resp.Token)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestSessionsRevokeAllExceptCurrent(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "sessions-revoke-all@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	first := parseAuthResp(t, w)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    "sessions-revoke-all@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	second := parseAuthResp(t, w)

	w = doJSON(t, srv, http.MethodDelete, "/api/auth/sessions?all_except_current=true", nil, first.Token)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/refresh", map[string]string{
		"refreshToken": first.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/refresh", map[string]string{
		"refreshToken": second.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestSessionRevocation_RevokeAllExceptCurrentDeniesOthers(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "sessions-revoke-deny-others@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	first := parseAuthResp(t, w)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    "sessions-revoke-deny-others@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	second := parseAuthResp(t, w)

	// Both access tokens work before revoke-all-except-current.
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, first.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, second.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	w = doJSON(t, srv, http.MethodDelete, "/api/auth/sessions?all_except_current=true", nil, first.Token)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	// Current token remains valid.
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, first.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Other session should now be denied immediately.
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, second.Token)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestSessionsRevokeOtherUserSessionForbidden(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "sessions-user-a@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	userA := parseAuthResp(t, w)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "sessions-user-b@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	userB := parseAuthResp(t, w)

	w = doJSON(t, srv, http.MethodGet, "/api/auth/sessions", nil, userB.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	sessionsB := parseSessionsResponse(t, w)
	testutil.Equal(t, 1, len(sessionsB))

	w = doJSON(t, srv, http.MethodDelete, "/api/auth/sessions/"+sessionsB[0].ID, nil, userA.Token)
	testutil.StatusCode(t, http.StatusForbidden, w.Code)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/refresh", map[string]string{
		"refreshToken": userB.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
}

func TestSessionsLastActiveAtUpdatedOnRefresh(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	firstHeaders := map[string]string{
		"User-Agent":      "SessionRefreshUA/1.0",
		"X-Forwarded-For": "203.0.113.50",
	}
	w := doJSONWithHeaders(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "sessions-last-active@example.com",
		"password": "password123",
	}, "", firstHeaders, "127.0.0.1:35555")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)

	var sessionID string
	var firstActive time.Time
	hash := hashRefreshToken(resp.RefreshToken)
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT id, last_active_at FROM _ayb_sessions WHERE token_hash = $1`,
		hash,
	).Scan(&sessionID, &firstActive)
	testutil.NoError(t, err)

	time.Sleep(20 * time.Millisecond)

	refreshHeaders := map[string]string{
		"User-Agent":      "SessionRefreshUA/2.0",
		"X-Forwarded-For": "203.0.113.99, 70.41.3.18",
	}
	w = doJSONWithHeaders(t, srv, http.MethodPost, "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "", refreshHeaders, "127.0.0.1:35555")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var secondActive time.Time
	var userAgent string
	var ipAddress string
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT last_active_at, COALESCE(user_agent, ''), COALESCE(ip_address, '')
		 FROM _ayb_sessions WHERE id = $1`,
		sessionID,
	).Scan(&secondActive, &userAgent, &ipAddress)
	testutil.NoError(t, err)
	testutil.True(t, secondActive.After(firstActive), "last_active_at should increase after refresh")
	testutil.Equal(t, "SessionRefreshUA/2.0", userAgent)
	testutil.Equal(t, "203.0.113.99", ipAddress)
}

func TestSessionActivity_DebouncedLastActiveAtOnAuthenticatedRequests(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "sessions-debounced-activity@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)

	var sessionID string
	var createdActiveAt time.Time
	hash := hashRefreshToken(resp.RefreshToken)
	err := sharedPG.Pool.QueryRow(ctx,
		`SELECT id, last_active_at FROM _ayb_sessions WHERE token_hash = $1`,
		hash,
	).Scan(&sessionID, &createdActiveAt)
	testutil.NoError(t, err)

	time.Sleep(20 * time.Millisecond)

	// First authenticated request should trigger a debounced write.
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var firstTouchActiveAt time.Time
	for i := 0; i < 30; i++ {
		err = sharedPG.Pool.QueryRow(ctx,
			`SELECT last_active_at FROM _ayb_sessions WHERE id = $1`,
			sessionID,
		).Scan(&firstTouchActiveAt)
		testutil.NoError(t, err)
		if firstTouchActiveAt.After(createdActiveAt) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	testutil.True(t, firstTouchActiveAt.After(createdActiveAt), "expected first authenticated request to update last_active_at")

	// Second request inside debounce window should not update again.
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	time.Sleep(50 * time.Millisecond)

	var secondTouchActiveAt time.Time
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT last_active_at FROM _ayb_sessions WHERE id = $1`,
		sessionID,
	).Scan(&secondTouchActiveAt)
	testutil.NoError(t, err)
	testutil.Equal(t, firstTouchActiveAt, secondTouchActiveAt)
}

func TestAdminSessionsListAndRevokeSingle(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServerWithAdmin(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "admin-sessions-list@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	user := parseAuthResp(t, w)

	adminToken := adminLoginForSessions(t, srv)
	userID := user.User["id"].(string)

	w = doJSON(t, srv, http.MethodGet, "/api/admin/users/"+userID+"/sessions", nil, adminToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	sessions := parseSessionsResponse(t, w)
	testutil.Equal(t, 1, len(sessions))
	testutil.False(t, sessions[0].Current, "admin view should not mark any session current")

	w = doJSON(t, srv, http.MethodDelete, "/api/admin/users/"+userID+"/sessions/"+sessions[0].ID, nil, adminToken)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/refresh", map[string]string{
		"refreshToken": user.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestAdminSessionsRevokeAll(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServerWithAdmin(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "admin-sessions-all@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	first := parseAuthResp(t, w)
	userID := first.User["id"].(string)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    "admin-sessions-all@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	second := parseAuthResp(t, w)

	adminToken := adminLoginForSessions(t, srv)
	w = doJSON(t, srv, http.MethodDelete, "/api/admin/users/"+userID+"/sessions", nil, adminToken)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/refresh", map[string]string{
		"refreshToken": first.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/refresh", map[string]string{
		"refreshToken": second.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestAdminSessionsRevokeAllImmediatelyDeniesAccessTokens(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServerWithAdmin(t, ctx)

	w := doJSON(t, srv, http.MethodPost, "/api/auth/register", map[string]string{
		"email":    "admin-sessions-all-access-deny@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	first := parseAuthResp(t, w)
	userID := first.User["id"].(string)

	w = doJSON(t, srv, http.MethodPost, "/api/auth/login", map[string]string{
		"email":    "admin-sessions-all-access-deny@example.com",
		"password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	second := parseAuthResp(t, w)

	// Both tokens are valid before admin revoke-all.
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, first.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, second.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	adminToken := adminLoginForSessions(t, srv)
	w = doJSON(t, srv, http.MethodDelete, "/api/admin/users/"+userID+"/sessions", nil, adminToken)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, first.Token)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	w = doJSON(t, srv, http.MethodGet, "/api/auth/me", nil, second.Token)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}
