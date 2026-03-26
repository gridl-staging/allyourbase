//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

const testJWTSecret = "integration-test-secret-that-is-at-least-32-chars!!"

func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}

	logger := testutil.DiscardLogger()
	runner := migrations.NewRunner(sharedPG.Pool, logger)
	if err := runner.Bootstrap(ctx); err != nil {
		t.Fatalf("bootstrapping migrations: %v", err)
	}
	if _, err := runner.Run(ctx); err != nil {
		t.Fatalf("running migrations: %v", err)
	}
}

func newAuthService() *auth.Service {
	return auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, 7*24*time.Hour, 8, testutil.DiscardLogger())
}

func setupAuthServer(t *testing.T, ctx context.Context) *server.Server {
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

	authSvc := newAuthService()
	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
}

func doJSON(t *testing.T, srv *server.Server, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("doJSON: marshal body: %v", err)
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
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	return w
}

type authResp struct {
	Token        string         `json:"token"`
	RefreshToken string         `json:"refreshToken"`
	User         map[string]any `json:"user"`
}

func parseAuthResp(t *testing.T, w *httptest.ResponseRecorder) authResp {
	t.Helper()
	var resp authResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parsing auth response: %v (body: %s)", err, w.Body.String())
	}
	return resp
}

// --- Registration tests ---

func TestRegisterSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "alice@example.com", "password": "password123",
	}, "")

	testutil.StatusCode(t, http.StatusCreated, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return a token")
	testutil.True(t, resp.RefreshToken != "", "should return a refresh token")
	testutil.Equal(t, "alice@example.com", resp.User["email"].(string))
	testutil.True(t, resp.User["id"].(string) != "", "should have user id")

	claims, err := newAuthService().ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("validating register token: %v", err)
	}
	testutil.True(t, claims.TenantID != "", "register token should include tenant id")

	var role string
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT role
		   FROM _ayb_tenant_memberships
		  WHERE tenant_id = $1 AND user_id = $2`,
		claims.TenantID, resp.User["id"].(string),
	).Scan(&role)
	if err != nil {
		t.Fatalf("querying tenant membership: %v", err)
	}
	testutil.Equal(t, "owner", role)
}

func TestRegisterDuplicateEmail(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	body := map[string]string{"email": "dup@example.com", "password": "password123"}
	w := doJSON(t, srv, "POST", "/api/auth/register", body, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	// Same email again.
	w = doJSON(t, srv, "POST", "/api/auth/register", body, "")
	testutil.StatusCode(t, http.StatusConflict, w.Code)
}

func TestRegisterDuplicateEmailCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "User@Example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	// Same email, different case.
	w = doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "user@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusConflict, w.Code)
}

// --- Login tests ---

func TestLoginSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register first.
	doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "login@example.com", "password": "password123",
	}, "")

	// Login.
	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "login@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return a token")
	testutil.True(t, resp.RefreshToken != "", "should return a refresh token")
	testutil.Equal(t, "login@example.com", resp.User["email"].(string))

	claims, err := newAuthService().ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("validating login token: %v", err)
	}
	testutil.True(t, claims.TenantID != "", "login token should include tenant id")
}

func TestLoginBackfillsDefaultTenantForLegacyUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()
	user, err := auth.CreateUser(ctx, sharedPG.Pool, "legacy-no-tenant@example.com", "password123", 8)
	if err != nil {
		t.Fatalf("creating legacy user: %v", err)
	}

	returnedUser, accessToken, refreshToken, err := svc.Login(ctx, "legacy-no-tenant@example.com", "password123")
	if err != nil {
		t.Fatalf("logging in legacy user: %v", err)
	}
	testutil.Equal(t, user.ID, returnedUser.ID)
	testutil.True(t, accessToken != "", "login should return access token")
	testutil.True(t, refreshToken != "", "login should return refresh token")

	claims, err := svc.ValidateToken(accessToken)
	if err != nil {
		t.Fatalf("validating login token: %v", err)
	}
	testutil.True(t, claims.TenantID != "", "legacy login should backfill tenant id")

	var membershipCount int
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT COUNT(*)
		   FROM _ayb_tenant_memberships
		  WHERE tenant_id = $1 AND user_id = $2`,
		claims.TenantID, user.ID,
	).Scan(&membershipCount)
	if err != nil {
		t.Fatalf("counting tenant memberships: %v", err)
	}
	testutil.Equal(t, 1, membershipCount)
}

func TestRefreshTokenPreservesTenantID(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "refresh-tenant@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	initial := parseAuthResp(t, w)
	initialClaims, err := newAuthService().ValidateToken(initial.Token)
	if err != nil {
		t.Fatalf("validating initial token: %v", err)
	}

	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": initial.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	refreshed := parseAuthResp(t, w)
	refreshedClaims, err := newAuthService().ValidateToken(refreshed.Token)
	if err != nil {
		t.Fatalf("validating refreshed token: %v", err)
	}
	testutil.Equal(t, initialClaims.TenantID, refreshedClaims.TenantID)
}

func TestAuthMetricsRecordedForRegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "metrics-auth@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	w = doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "metrics-auth@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	metrics := doJSON(t, srv, "GET", "/metrics", nil, "")
	testutil.StatusCode(t, http.StatusOK, metrics.Code)
	body := metrics.Body.String()
	testutil.Contains(t, body, "ayb_auth_signups_total 1")
	testutil.Contains(t, body, "ayb_auth_logins_total 1")
}

func TestLoginWrongPassword(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "wrong@example.com", "password": "password123",
	}, "")

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "wrong@example.com", "password": "wrongpassword",
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestLoginNonexistentEmail(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "noone@example.com", "password": "password123",
	}, "")
	// Same status as wrong password — no enumeration.
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

// --- /me endpoint tests ---

func TestMeWithRegisterToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "me@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	w = doJSON(t, srv, "GET", "/api/auth/me", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var user map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatalf("parsing /me response: %v (body: %s)", err, w.Body.String())
	}
	testutil.Equal(t, "me@example.com", user["email"].(string))
}

func TestMeWithLoginToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "melogin@example.com", "password": "password123",
	}, "")

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "melogin@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	w = doJSON(t, srv, "GET", "/api/auth/me", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var user map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatalf("parsing /me response: %v (body: %s)", err, w.Body.String())
	}
	testutil.Equal(t, "melogin@example.com", user["email"].(string))
}

func TestMeWithoutToken(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "GET", "/api/auth/me", nil, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

// --- Protected collection endpoints ---

func TestCollectionEndpointRequiresAuth(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Create a test table.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL
		)
	`)
	testutil.NoError(t, err)

	// Reload schema.
	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	authSvc := newAuthService()
	srv = server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	// Without token → 401.
	w := doJSON(t, srv, "GET", "/api/collections/posts/", nil, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)

	// Register and get token.
	w = doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "authed@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// With token → 200.
	w = doJSON(t, srv, "GET", "/api/collections/posts/", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
}

// --- RLS enforcement ---

func TestRLSEnforcement(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	// Create a table with RLS.
	_, err := sharedPG.Pool.Exec(ctx, `
		CREATE TABLE notes (
			id SERIAL PRIMARY KEY,
			owner_id TEXT NOT NULL,
			content TEXT NOT NULL
		);
		ALTER TABLE notes ENABLE ROW LEVEL SECURITY;
		ALTER TABLE notes FORCE ROW LEVEL SECURITY;
		CREATE POLICY notes_owner ON notes
			USING (owner_id = current_setting('ayb.user_id', true));
	`)
	testutil.NoError(t, err)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	// Register two users.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "user1@example.com", "password": "password123",
	}, "")
	user1 := parseAuthResp(t, w)

	w = doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "user2@example.com", "password": "password123",
	}, "")
	user2 := parseAuthResp(t, w)

	user1ID := user1.User["id"].(string)
	user2ID := user2.User["id"].(string)

	// Insert notes owned by each user (bypass RLS with superuser pool).
	_, err = sharedPG.Pool.Exec(ctx,
		"INSERT INTO notes (owner_id, content) VALUES ($1, 'user1 note'), ($2, 'user2 note')",
		user1ID, user2ID)
	testutil.NoError(t, err)

	// User 1 should only see their note.
	w = doJSON(t, srv, "GET", "/api/collections/notes/", nil, user1.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var list1 struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list1); err != nil {
		t.Fatalf("parsing user1 notes response: %v (body: %s)", err, w.Body.String())
	}
	testutil.Equal(t, 1, len(list1.Items))
	testutil.Equal(t, "user1 note", list1.Items[0]["content"])

	// User 2 should only see their note.
	w = doJSON(t, srv, "GET", "/api/collections/notes/", nil, user2.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var list2 struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list2); err != nil {
		t.Fatalf("parsing user2 notes response: %v (body: %s)", err, w.Body.String())
	}
	testutil.Equal(t, 1, len(list2.Items))
	testutil.Equal(t, "user2 note", list2.Items[0]["content"])
}

// --- Refresh token tests ---

func setupAuthServerWithRefreshDur(t *testing.T, ctx context.Context, refreshDur time.Duration) *server.Server {
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

	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, refreshDur, 8, logger)
	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
}

func TestRefreshTokenFlow(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "refresh@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)
	testutil.True(t, resp.RefreshToken != "", "should return refresh token")

	// Use refresh token to get new tokens.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	refreshResp := parseAuthResp(t, w)
	testutil.True(t, refreshResp.Token != "", "should return new access token")
	testutil.True(t, refreshResp.RefreshToken != "", "should return new refresh token")

	// Verify the new access token works on /me.
	w = doJSON(t, srv, "GET", "/api/auth/me", nil, refreshResp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
}

func TestRefreshTokenExpired(t *testing.T) {
	ctx := context.Background()
	// Use a 1ms refresh duration so it expires immediately.
	srv := setupAuthServerWithRefreshDur(t, ctx, time.Millisecond)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "expired@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// Wait for the refresh token to expire.
	time.Sleep(50 * time.Millisecond)

	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestLogout(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "logout@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// Logout.
	w = doJSON(t, srv, "POST", "/api/auth/logout", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	// Refresh with the logged-out token should fail.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

// --- OAuth integration tests ---

func TestOAuthLoginNewUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()
	info := &auth.OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "oauth@example.com",
		Name:           "OAuth User",
	}

	user, token, refreshToken, err := svc.OAuthLogin(ctx, "google", info)
	testutil.NoError(t, err)
	testutil.True(t, user.ID != "", "should create user")
	testutil.Equal(t, "oauth@example.com", user.Email)
	testutil.True(t, token != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")

	// Verify the access token works.
	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, claims.Subject)
}

func TestOAuthLoginExistingIdentity(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()
	info := &auth.OAuthUserInfo{
		ProviderUserID: "google-456",
		Email:          "repeat@example.com",
		Name:           "Repeat User",
	}

	// First login creates user.
	user1, _, _, err := svc.OAuthLogin(ctx, "google", info)
	testutil.NoError(t, err)

	// Second login with same provider identity returns same user.
	user2, _, _, err := svc.OAuthLogin(ctx, "google", info)
	testutil.NoError(t, err)
	testutil.Equal(t, user1.ID, user2.ID)
}

func TestOAuthLoginLinksToExistingEmailUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()

	// Register a user with email/password first.
	emailUser, _, _, err := svc.Register(ctx, "linked@example.com", "password123")
	testutil.NoError(t, err)

	// Login via OAuth with the same email.
	info := &auth.OAuthUserInfo{
		ProviderUserID: "github-789",
		Email:          "linked@example.com",
		Name:           "Linked User",
	}
	oauthUser, _, _, err := svc.OAuthLogin(ctx, "github", info)
	testutil.NoError(t, err)

	// Should be the same user (linked, not a new account).
	testutil.Equal(t, emailUser.ID, oauthUser.ID)
}

func TestOAuthLoginMultipleProvidersSameUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()

	// Login via Google.
	googleInfo := &auth.OAuthUserInfo{
		ProviderUserID: "google-multi",
		Email:          "multi@example.com",
		Name:           "Multi User",
	}
	user1, _, _, err := svc.OAuthLogin(ctx, "google", googleInfo)
	testutil.NoError(t, err)

	// Login via GitHub with same email.
	githubInfo := &auth.OAuthUserInfo{
		ProviderUserID: "github-multi",
		Email:          "multi@example.com",
		Name:           "Multi User",
	}
	user2, _, _, err := svc.OAuthLogin(ctx, "github", githubInfo)
	testutil.NoError(t, err)

	// Should be the same user.
	testutil.Equal(t, user1.ID, user2.ID)
}

func TestOAuthLoginNoEmail(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	svc := newAuthService()
	info := &auth.OAuthUserInfo{
		ProviderUserID: "github-noemail",
		Email:          "",
		Name:           "No Email User",
	}

	user, _, _, err := svc.OAuthLogin(ctx, "github", info)
	testutil.NoError(t, err)
	testutil.True(t, user.ID != "", "should create user even without email")
	// Should have a placeholder email.
	testutil.True(t, user.Email != "", "should have placeholder email")
}

func TestOAuthHandlerFullFlowMocked(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	// Set up fake OAuth provider endpoints.
	fakeProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := json.NewEncoder(w).Encode(map[string]string{
				"access_token": "fake-access-token",
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		case "/userinfo":
			if err := json.NewEncoder(w).Encode(map[string]any{
				"id":    "12345",
				"email": "fakeuser@example.com",
				"name":  "Fake User",
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fakeProvider.Close()

	// Override Google's endpoints to point to our fake server.
	auth.SetProviderURLs("google", auth.OAuthProviderConfig{
		AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:    fakeProvider.URL + "/token",
		UserInfoURL: fakeProvider.URL + "/userinfo",
		Scopes:      []string{"openid", "email", "profile"},
	})
	defer auth.ResetProviderURLs("google")

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.OAuth = map[string]config.OAuthProvider{
		"google": {Enabled: true, ClientID: "test-id", ClientSecret: "test-secret"},
	}
	cfg.Auth.OAuthRedirectURL = "http://localhost:5173/callback"

	svc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, svc, nil)

	// Step 1: Initiate OAuth → should redirect to Google.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/google", nil)
	req.Host = "localhost:8090"
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	testutil.StatusCode(t, http.StatusTemporaryRedirect, w.Code)
	loc := w.Header().Get("Location")
	testutil.True(t, loc != "", "should redirect")

	// Extract state from the redirect URL.
	var state string
	if idx := len("state="); true {
		for _, part := range splitQuery(loc) {
			if len(part) > idx && part[:idx] == "state=" {
				state = part[idx:]
				break
			}
		}
	}
	testutil.True(t, state != "", "redirect should include state")

	// Step 2: Simulate callback from provider.
	callbackURL := fmt.Sprintf("/api/auth/oauth/google/callback?code=test-code&state=%s", state)
	req = httptest.NewRequest(http.MethodGet, callbackURL, nil)
	req.Host = "localhost:8090"
	w = httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	// Should redirect to the configured redirect URL with tokens.
	testutil.StatusCode(t, http.StatusTemporaryRedirect, w.Code)
	redirectLoc := w.Header().Get("Location")
	testutil.True(t, redirectLoc != "", "should redirect with tokens")
	testutil.True(t, len(redirectLoc) > len("http://localhost:5173/callback#"), "redirect should have fragment")

	// Verify the user was created.
	var count int
	err := sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_users WHERE email = 'fakeuser@example.com'",
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)

	// Verify the OAuth account was linked.
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_oauth_accounts WHERE provider = 'google' AND provider_user_id = '12345'",
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)
}

// splitQuery splits a URL's query string into key=value pairs.
func splitQuery(rawURL string) []string {
	idx := 0
	for i, c := range rawURL {
		if c == '?' {
			idx = i + 1
			break
		}
	}
	if idx == 0 {
		return nil
	}
	query := rawURL[idx:]
	var parts []string
	for _, p := range splitOn(query, '&') {
		parts = append(parts, p)
	}
	return parts
}

func splitOn(s string, sep byte) []string {
	var result []string
	start := 0
	for i := range len(s) {
		if s[i] == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// --- Refresh token rotation tests ---

func TestRefreshTokenRotation(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register and get initial tokens.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "refresh@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp1 := parseAuthResp(t, w)
	oldRefreshToken := resp1.RefreshToken

	// Use refresh token to get new tokens.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": oldRefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	resp2 := parseAuthResp(t, w)

	// Verify new tokens are different.
	testutil.NotEqual(t, resp1.Token, resp2.Token)
	testutil.NotEqual(t, resp1.RefreshToken, resp2.RefreshToken)

	// Old refresh token should no longer work (rotation invalidates it).
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": oldRefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)

	// New refresh token should work.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp2.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
}

func TestRefreshTokenCanOnlyBeUsedOnce(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "once@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)
	refreshToken := resp.RefreshToken

	// First refresh succeeds.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": refreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Second use of same token fails.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": refreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestRefreshTokenRejectedAfterExpiry(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	// Create auth service with very short refresh token expiry (1 second).
	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, 1*time.Second, 8, testutil.DiscardLogger())

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	// Register.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "expiry@example.com", "password": "password123",
	}, "")
	resp := parseAuthResp(t, w)

	// Wait for refresh token to expire.
	time.Sleep(1200 * time.Millisecond)

	// Refresh should fail.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestRefreshTokenPreservesAAL2(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register a user (gets AAL1 session).
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "aal2refresh@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)

	// Simulate MFA completion: update session to AAL2 with AMR.
	tokenHash := auth.HashTokenForTest(resp.RefreshToken)
	_, err := sharedPG.Pool.Exec(ctx,
		`UPDATE _ayb_sessions SET aal = 'aal2', amr = 'password,totp' WHERE token_hash = $1`,
		tokenHash,
	)
	testutil.NoError(t, err)

	// Refresh should produce a token with AAL2 preserved.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	refreshResp := parseAuthResp(t, w)

	// Parse the new access token and verify AAL2 + AMR are preserved.
	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, 7*24*time.Hour, 8, testutil.DiscardLogger())
	claims, err := authSvc.ValidateToken(refreshResp.Token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal2", claims.AAL)
	testutil.Equal(t, 2, len(claims.AMR))
	testutil.Equal(t, "password", claims.AMR[0])
	testutil.Equal(t, "totp", claims.AMR[1])
}

func TestRefreshTokenDoesNotElevateAAL(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register a user (gets AAL1 session by default).
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "aal1refresh@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)

	// Refresh — should stay AAL1.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	refreshResp := parseAuthResp(t, w)

	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, 7*24*time.Hour, 8, testutil.DiscardLogger())
	claims, err := authSvc.ValidateToken(refreshResp.Token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal1", claims.AAL)
}

func TestRefreshTokenPreservesAMRAtAAL1(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)

	// Register a user (gets AAL1 session by default).
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "aal1amrrefresh@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)

	// Simulate first-factor AMR tracking on the session.
	tokenHash := auth.HashTokenForTest(resp.RefreshToken)
	_, err := sharedPG.Pool.Exec(ctx,
		`UPDATE _ayb_sessions SET aal = 'aal1', amr = 'password' WHERE token_hash = $1`,
		tokenHash,
	)
	testutil.NoError(t, err)

	// Refresh should preserve AMR even when AAL remains aal1.
	w = doJSON(t, srv, "POST", "/api/auth/refresh", map[string]string{
		"refreshToken": resp.RefreshToken,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	refreshResp := parseAuthResp(t, w)

	authSvc := auth.NewService(sharedPG.Pool, testJWTSecret, time.Hour, 7*24*time.Hour, 8, testutil.DiscardLogger())
	claims, err := authSvc.ValidateToken(refreshResp.Token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal1", claims.AAL)
	testutil.Equal(t, 1, len(claims.AMR))
	testutil.Equal(t, "password", claims.AMR[0])
}

// --- Verification token tests ---

func TestVerificationTokenReuse(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()

	// Create a user.
	user, err := auth.CreateUser(ctx, sharedPG.Pool, "verify@example.com", "password123", 8)
	testutil.NoError(t, err)

	// Manually insert a verification token (simulating SendVerificationEmail).
	token := "test-verification-token-12345"
	hash := auth.HashTokenForTest(token)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_email_verifications (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		user.ID, hash, time.Now().Add(time.Hour),
	)
	testutil.NoError(t, err)

	// Verify email.
	err = authSvc.ConfirmEmail(ctx, token)
	testutil.NoError(t, err)

	// Check user is verified.
	var verified bool
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT email_verified FROM _ayb_users WHERE id = $1`, user.ID,
	).Scan(&verified)
	testutil.NoError(t, err)
	testutil.True(t, verified, "email should be verified")

	// Try to use same token again — should fail (token deleted after use).
	err = authSvc.ConfirmEmail(ctx, token)
	testutil.ErrorContains(t, err, "invalid or expired verification token")
}

func TestVerificationTokenExpiry(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()

	// Create a user.
	user, err := auth.CreateUser(ctx, sharedPG.Pool, "expired@example.com", "password123", 8)
	testutil.NoError(t, err)

	// Insert an expired verification token.
	token := "expired-token-12345"
	hash := auth.HashTokenForTest(token)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_email_verifications (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		user.ID, hash, time.Now().Add(-time.Hour), // expired
	)
	testutil.NoError(t, err)

	// Try to verify with expired token.
	err = authSvc.ConfirmEmail(ctx, token)
	testutil.ErrorContains(t, err, "invalid or expired verification token")
}

func TestVerificationTokenInvalidFormat(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()

	// Try to verify with invalid token.
	err := authSvc.ConfirmEmail(ctx, "not-a-real-token")
	testutil.ErrorContains(t, err, "invalid or expired verification token")
}

// --- API key management integration tests ---

func registerAndGetToken(t *testing.T, srv *server.Server, email string) string {
	t.Helper()
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": email, "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)
	return resp.Token
}

func TestAPIKeyCreateSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-create@example.com")

	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
		"name": "my-key",
	}, token)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	var resp struct {
		Key    string `json:"key"`
		APIKey struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"apiKey"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// API key should have realistic length (prefix + hash).
	testutil.True(t, len(resp.Key) >= 32, "apiKey should be at least 32 chars")
	testutil.Contains(t, resp.Key, "ayb_")
	testutil.Equal(t, "my-key", resp.APIKey.Name)
	// UUID should be exactly 36 chars (8-4-4-4-12 with hyphens).
	testutil.Equal(t, 36, len(resp.APIKey.ID))
}

func TestAPIKeyCreateWithScope(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-scope@example.com")

	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]any{
		"name":  "readonly-key",
		"scope": "readonly",
	}, token)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	var resp struct {
		Key    string `json:"key"`
		APIKey struct {
			Scope string `json:"scope"`
			Name  string `json:"name"`
		} `json:"apiKey"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, "readonly", resp.APIKey.Scope)
	testutil.Equal(t, "readonly-key", resp.APIKey.Name)
}

func TestAPIKeyCreateInvalidScope(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-badscope@example.com")

	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
		"name":  "bad-scope-key",
		"scope": "admin",
	}, token)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid scope")
}

func TestAPIKeyListSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-list@example.com")

	// Create two keys.
	for _, name := range []string{"key-1", "key-2"} {
		w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
			"name": name,
		}, token)
		testutil.StatusCode(t, http.StatusCreated, w.Code)
	}

	// List keys.
	w := doJSON(t, srv, "GET", "/api/auth/api-keys/", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var keys []json.RawMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &keys))
	testutil.Equal(t, 2, len(keys))
}

func TestAPIKeyListEmpty(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-empty@example.com")

	w := doJSON(t, srv, "GET", "/api/auth/api-keys/", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var keys []json.RawMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &keys))
	testutil.Equal(t, 0, len(keys))
}

func TestAPIKeyRevokeSuccess(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-revoke@example.com")

	// Create a key.
	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
		"name": "to-revoke",
	}, token)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	var createResp struct {
		APIKey struct {
			ID string `json:"id"`
		} `json:"apiKey"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	testutil.True(t, createResp.APIKey.ID != "", "should return key ID")

	// Revoke it.
	w = doJSON(t, srv, "DELETE", "/api/auth/api-keys/"+createResp.APIKey.ID, nil, token)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	// List should show the key with revokedAt set (key still exists, just revoked).
	w = doJSON(t, srv, "GET", "/api/auth/api-keys/", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var keys []struct {
		ID        string  `json:"id"`
		RevokedAt *string `json:"revokedAt"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &keys))
	testutil.Equal(t, 1, len(keys))
	testutil.NotNil(t, keys[0].RevokedAt)
}

func TestAPIKeyRevokeNotFound(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-notfound@example.com")

	w := doJSON(t, srv, "DELETE", "/api/auth/api-keys/00000000-0000-0000-0000-000000000000", nil, token)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "api key not found")
}

func TestAPIKeyRevokeInvalidUUID(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-baduuid@example.com")

	w := doJSON(t, srv, "DELETE", "/api/auth/api-keys/not-a-uuid", nil, token)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid api key id format")
}

func TestAPIKeyRevokeAlreadyRevoked(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-double-revoke@example.com")

	// Create and revoke a key.
	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
		"name": "double-revoke",
	}, token)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	var createResp struct {
		APIKey struct {
			ID string `json:"id"`
		} `json:"apiKey"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))

	// First revoke succeeds.
	w = doJSON(t, srv, "DELETE", "/api/auth/api-keys/"+createResp.APIKey.ID, nil, token)
	testutil.StatusCode(t, http.StatusNoContent, w.Code)

	// Second revoke returns 404 (revoked_at IS NULL clause fails).
	w = doJSON(t, srv, "DELETE", "/api/auth/api-keys/"+createResp.APIKey.ID, nil, token)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "api key not found")
}

func TestAPIKeyIsolationBetweenUsers(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token1 := registerAndGetToken(t, srv, "apikey-user1@example.com")
	token2 := registerAndGetToken(t, srv, "apikey-user2@example.com")

	// User 1 creates a key.
	w := doJSON(t, srv, "POST", "/api/auth/api-keys/", map[string]string{
		"name": "user1-key",
	}, token1)
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	var createResp struct {
		APIKey struct {
			ID string `json:"id"`
		} `json:"apiKey"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))

	// User 2 cannot see user 1's keys.
	w = doJSON(t, srv, "GET", "/api/auth/api-keys/", nil, token2)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var keys []json.RawMessage
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &keys))
	testutil.Equal(t, 0, len(keys))

	// User 2 cannot revoke user 1's key.
	w = doJSON(t, srv, "DELETE", "/api/auth/api-keys/"+createResp.APIKey.ID, nil, token2)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
}

// --- Magic link integration tests ---

func setupMagicLinkServer(t *testing.T, ctx context.Context) *server.Server {
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
	cfg.Auth.MagicLinkEnabled = true

	authSvc := newAuthService()
	authSvc.SetMagicLinkDuration(10 * time.Minute)
	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
}

func TestMagicLinkRequestReturns200(t *testing.T) {
	ctx := context.Background()
	srv := setupMagicLinkServer(t, ctx)

	// Request for nonexistent email should still return 200 (no enumeration).
	w := doJSON(t, srv, "POST", "/api/auth/magic-link", map[string]string{
		"email": "nobody@example.com",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "if valid, a login link has been sent")
}

func TestMagicLinkFullFlowNewUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()
	authSvc.SetMagicLinkDuration(10 * time.Minute)

	email := "newmagic@example.com"

	// Verify user doesn't exist yet.
	var count int
	err := sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_users WHERE LOWER(email) = $1", email,
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)

	// Insert a magic link token directly (simulating what RequestMagicLink does).
	token := "test-magic-token-new-user"
	hash := auth.HashTokenForTest(token)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		email, hash, time.Now().Add(10*time.Minute),
	)
	testutil.NoError(t, err)

	// Confirm the magic link.
	user, accessToken, refreshToken, err := authSvc.ConfirmMagicLink(ctx, token)
	testutil.NoError(t, err)
	testutil.True(t, user.ID != "", "should create user")
	testutil.Equal(t, email, user.Email)
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")

	// Verify the access token works.
	claims, err := authSvc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, claims.Subject)

	// Verify user was created in DB with email_verified = true.
	var verified bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT email_verified FROM _ayb_users WHERE id = $1", user.ID,
	).Scan(&verified)
	testutil.NoError(t, err)
	testutil.True(t, verified, "email should be verified after magic link login")
}

func TestMagicLinkFullFlowExistingUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()
	authSvc.SetMagicLinkDuration(10 * time.Minute)

	// Register a user first.
	existingUser, _, _, err := authSvc.Register(ctx, "existing@example.com", "password123")
	testutil.NoError(t, err)

	// Insert a magic link token for the existing user's email.
	token := "test-magic-token-existing"
	hash := auth.HashTokenForTest(token)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		existingUser.Email, hash, time.Now().Add(10*time.Minute),
	)
	testutil.NoError(t, err)

	// Confirm the magic link.
	user, accessToken, _, err := authSvc.ConfirmMagicLink(ctx, token)
	testutil.NoError(t, err)
	testutil.Equal(t, existingUser.ID, user.ID) // same user, not a new one
	testutil.True(t, accessToken != "", "should return access token")

	// Email should now be verified.
	var verified bool
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT email_verified FROM _ayb_users WHERE id = $1", user.ID,
	).Scan(&verified)
	testutil.NoError(t, err)
	testutil.True(t, verified, "email should be verified after magic link login")
}

func TestMagicLinkTokenConsumedAfterUse(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()
	authSvc.SetMagicLinkDuration(10 * time.Minute)

	email := "consumed@example.com"
	token := "test-magic-token-consumed"
	hash := auth.HashTokenForTest(token)
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		email, hash, time.Now().Add(10*time.Minute),
	)
	testutil.NoError(t, err)

	// First use succeeds.
	_, _, _, err = authSvc.ConfirmMagicLink(ctx, token)
	testutil.NoError(t, err)

	// Second use fails (token consumed).
	_, _, _, err = authSvc.ConfirmMagicLink(ctx, token)
	testutil.ErrorContains(t, err, "invalid or expired magic link token")
}

func TestMagicLinkTokenExpired(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()

	email := "expired-magic@example.com"
	token := "test-magic-token-expired"
	hash := auth.HashTokenForTest(token)
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		email, hash, time.Now().Add(-time.Hour), // already expired
	)
	testutil.NoError(t, err)

	_, _, _, err = authSvc.ConfirmMagicLink(ctx, token)
	testutil.ErrorContains(t, err, "invalid or expired magic link token")
}

func TestMagicLinkTokenInvalid(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()

	_, _, _, err := authSvc.ConfirmMagicLink(ctx, "not-a-real-token")
	testutil.ErrorContains(t, err, "invalid or expired magic link token")
}

func TestMagicLinkHandlerConfirmFullFlow(t *testing.T) {
	ctx := context.Background()
	srv := setupMagicLinkServer(t, ctx)

	// Insert a token directly.
	email := "handler-flow@example.com"
	token := "test-handler-magic-token"
	hash := auth.HashTokenForTest(token)
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		email, hash, time.Now().Add(10*time.Minute),
	)
	testutil.NoError(t, err)

	// Confirm via HTTP.
	w := doJSON(t, srv, "POST", "/api/auth/magic-link/confirm", map[string]string{
		"token": token,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return access token")
	testutil.True(t, resp.RefreshToken != "", "should return refresh token")
	testutil.Equal(t, email, resp.User["email"].(string))
}

func TestMagicLinkHandlerConfirmInvalidToken(t *testing.T) {
	ctx := context.Background()
	srv := setupMagicLinkServer(t, ctx)

	w := doJSON(t, srv, "POST", "/api/auth/magic-link/confirm", map[string]string{
		"token": "bogus-token",
	}, "")
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid or expired magic link token")
}

func TestMagicLinkDisabledReturns404(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	// MagicLinkEnabled defaults to false.

	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	w := doJSON(t, srv, "POST", "/api/auth/magic-link", map[string]string{
		"email": "test@example.com",
	}, "")
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "not enabled")
}

func TestMagicLinkRequestMagicLinkDeletesPreviousTokens(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	authSvc := newAuthService()
	authSvc.SetMagicLinkDuration(10 * time.Minute)
	// Wire up a log mailer so RequestMagicLink actually runs (it's a no-op without a mailer).
	authSvc.SetMailer(mailer.NewLogMailer(testutil.DiscardLogger()), "TestApp", "http://localhost:8090/api")

	email := "cleanup@example.com"

	// Insert two tokens for the same email.
	for _, tok := range []string{"old-token-1", "old-token-2"} {
		hash := auth.HashTokenForTest(tok)
		_, err := sharedPG.Pool.Exec(ctx,
			`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
			 VALUES ($1, $2, $3)`,
			email, hash, time.Now().Add(10*time.Minute),
		)
		testutil.NoError(t, err)
	}

	// Verify 2 tokens exist.
	var count int
	err := sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_magic_links WHERE email = $1", email,
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, count)

	// Call the actual RequestMagicLink method — this should delete old tokens and insert a new one.
	err = authSvc.RequestMagicLink(ctx, email)
	testutil.NoError(t, err)

	// After cleanup + insert, should be exactly 1 (the new token).
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_magic_links WHERE email = $1", email,
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)
}

// --- SMS OTP integration tests ---

func setupSMSService(t *testing.T) (*auth.Service, *sms.CaptureProvider) {
	t.Helper()
	resetAndMigrate(t, t.Context())
	capture := &sms.CaptureProvider{}
	svc := newAuthService()
	svc.SetSMSProvider(capture)
	svc.SetSMSConfig(sms.Config{
		CodeLength:       6,
		Expiry:           5 * time.Minute,
		MaxAttempts:      3,
		DailyLimit:       0, // unlimited in tests
		AllowedCountries: []string{"US", "CA"},
	})
	return svc, capture
}

func TestSMSFullFlow_NewUser(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	err := svc.RequestSMSCode(ctx, "+14155552671")
	testutil.NoError(t, err)
	testutil.SliceLen(t, capture.Calls, 1)
	testutil.Equal(t, "+14155552671", capture.Calls[0].To)

	code := capture.LastCode()
	testutil.True(t, code != "", "should have captured an OTP code")

	user, accessToken, refreshToken, err := svc.ConfirmSMSCode(ctx, "+14155552671", code)
	testutil.NoError(t, err)
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")
	testutil.Equal(t, "+14155552671", user.Phone)
	testutil.Equal(t, "+14155552671@sms.local", user.Email)
}

func TestSMSFullFlow_ExistingUser(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	// First login: create user.
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	first, _, _, err := svc.ConfirmSMSCode(ctx, "+14155552671", capture.LastCode())
	testutil.NoError(t, err)
	capture.Reset()

	// Second login: same phone → same user.
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	second, _, _, err := svc.ConfirmSMSCode(ctx, "+14155552671", capture.LastCode())
	testutil.NoError(t, err)
	testutil.Equal(t, first.ID, second.ID)
}

func TestSMSCode_ConsumedAfterUse(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	code := capture.LastCode()
	_, _, _, err := svc.ConfirmSMSCode(ctx, "+14155552671", code)
	testutil.NoError(t, err)

	// Second confirm with same code should fail.
	_, _, _, err = svc.ConfirmSMSCode(ctx, "+14155552671", code)
	testutil.True(t, err != nil, "expected error on reuse")
	testutil.True(t, errors.Is(err, auth.ErrInvalidSMSCode), "expected ErrInvalidSMSCode")
}

func TestSMSCode_InvalidCodeIncrementsAttempts(t *testing.T) {
	svc, _ := setupSMSService(t)
	ctx := t.Context()

	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	_, _, _, err := svc.ConfirmSMSCode(ctx, "+14155552671", "000000")
	testutil.True(t, err != nil, "expected error for wrong code")

	var attempts int
	err = svc.DB().QueryRow(ctx,
		`SELECT attempts FROM _ayb_sms_codes WHERE phone = $1`, "+14155552671",
	).Scan(&attempts)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, attempts)
}

func TestSMSCode_MaxAttemptsDeletesCode(t *testing.T) {
	svc, _ := setupSMSService(t)
	ctx := t.Context()

	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))

	// Each wrong attempt should return an error.
	for i := 0; i < 3; i++ {
		_, _, _, err := svc.ConfirmSMSCode(ctx, "+14155552671", "000000")
		testutil.True(t, errors.Is(err, auth.ErrInvalidSMSCode),
			"attempt %d: expected ErrInvalidSMSCode, got %v", i+1, err)
	}

	// After max attempts, the code row should be deleted.
	var count int
	err := svc.DB().QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_sms_codes WHERE phone = $1`, "+14155552671",
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}

func TestSMSCode_WrongThenCorrectSucceeds(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	code := capture.LastCode()

	// One wrong attempt — should fail but not exhaust the code.
	_, _, _, err := svc.ConfirmSMSCode(ctx, "+14155552671", "000000")
	testutil.True(t, errors.Is(err, auth.ErrInvalidSMSCode), "wrong code should fail")

	// Correct code should still work.
	user, accessToken, refreshToken, err := svc.ConfirmSMSCode(ctx, "+14155552671", code)
	testutil.NoError(t, err)
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")
	testutil.Equal(t, "+14155552671", user.Phone)
}

func TestSMSCode_NewRequestDeletesOldCode(t *testing.T) {
	svc, _ := setupSMSService(t)
	ctx := t.Context()

	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))

	var count int
	err := svc.DB().QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_sms_codes WHERE phone = $1`, "+14155552671",
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)
}

func TestSMS_GeoBlock(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	// UK number — outside allowed ["US","CA"].
	err := svc.RequestSMSCode(ctx, "+442079460958")
	testutil.NoError(t, err) // no error returned (anti-enumeration)
	testutil.SliceLen(t, capture.Calls, 0)

	// Verify no code was stored in the database either.
	var count int
	err = svc.DB().QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_sms_codes WHERE phone = $1`, "+442079460958",
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}

func TestSMS_DailyLimitCircuitBreaker(t *testing.T) {
	svc, _ := setupSMSService(t)
	ctx := t.Context()

	svc.SetSMSConfig(sms.Config{
		CodeLength:       6,
		Expiry:           5 * time.Minute,
		MaxAttempts:      3,
		DailyLimit:       2,
		AllowedCountries: []string{"US", "CA"},
	})

	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552672"))
	err := svc.RequestSMSCode(ctx, "+14155552673")
	testutil.True(t, errors.Is(err, auth.ErrDailyLimitExceeded), "expected ErrDailyLimitExceeded")
}

// --- Test phone numbers ---

func TestRequestSMSCode_TestPhoneNumber(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	// Configure a test phone number with predetermined code.
	svc.SetSMSConfig(sms.Config{
		CodeLength:       6,
		Expiry:           5 * time.Minute,
		MaxAttempts:      3,
		DailyLimit:       0,
		AllowedCountries: []string{"US"},
		TestPhoneNumbers: map[string]string{"+15550001234": "000000"},
	})

	err := svc.RequestSMSCode(ctx, "+15550001234")
	testutil.NoError(t, err)

	// Provider.Send() must NOT be called for test phones.
	testutil.SliceLen(t, capture.Calls, 0)

	// The predetermined code should still be stored and work for confirmation.
	user, accessToken, refreshToken, err := svc.ConfirmSMSCode(ctx, "+15550001234", "000000")
	testutil.NoError(t, err)
	testutil.True(t, user != nil, "should return user")
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")
}

func TestConfirmSMSCode_TestPhoneNumber(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	svc.SetSMSConfig(sms.Config{
		CodeLength:       6,
		Expiry:           5 * time.Minute,
		MaxAttempts:      3,
		DailyLimit:       0,
		AllowedCountries: []string{"US"},
		TestPhoneNumbers: map[string]string{"+15550001234": "000000"},
	})

	// Request code for test phone.
	err := svc.RequestSMSCode(ctx, "+15550001234")
	testutil.NoError(t, err)
	testutil.SliceLen(t, capture.Calls, 0)

	// Wrong code should fail.
	_, _, _, err = svc.ConfirmSMSCode(ctx, "+15550001234", "999999")
	testutil.True(t, err != nil, "wrong code should fail")

	// Re-request (wrong attempt consumed the code attempt).
	err = svc.RequestSMSCode(ctx, "+15550001234")
	testutil.NoError(t, err)

	// Correct predetermined code should succeed.
	user, accessToken, refreshToken, err := svc.ConfirmSMSCode(ctx, "+15550001234", "000000")
	testutil.NoError(t, err)
	testutil.True(t, user != nil, "should return user")
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")
}

func TestRequestSMSCode_TestPhoneNumber_BypassesDailyCount(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	svc.SetSMSConfig(sms.Config{
		CodeLength:       6,
		Expiry:           5 * time.Minute,
		MaxAttempts:      3,
		DailyLimit:       2, // low limit
		AllowedCountries: []string{"US"},
		TestPhoneNumbers: map[string]string{"+15550001234": "000000"},
	})

	// Send 5 requests to the test phone number — should all succeed
	// even though DailyLimit is 2, because test phones bypass the count.
	for i := 0; i < 5; i++ {
		err := svc.RequestSMSCode(ctx, "+15550001234")
		testutil.NoError(t, err)
	}
	testutil.SliceLen(t, capture.Calls, 0)

	// Verify no daily count was incremented.
	var count int
	err := svc.DB().QueryRow(ctx,
		`SELECT COALESCE((SELECT count FROM _ayb_sms_daily_counts WHERE date = CURRENT_DATE), 0)`,
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)

	// Now send a real SMS to prove the daily limit still works for non-test phones.
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552672"))
	err = svc.RequestSMSCode(ctx, "+14155552673")
	testutil.True(t, errors.Is(err, auth.ErrDailyLimitExceeded), "expected ErrDailyLimitExceeded for real phones")
}

func TestRequestSMSCode_TestPhoneNumber_NotConfigured(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	// No TestPhoneNumbers configured — normal flow.
	// Use a valid US number (555 numbers fail libphonenumber validation).
	err := svc.RequestSMSCode(ctx, "+14155552671")
	testutil.NoError(t, err)

	// Provider.Send() should be called normally.
	testutil.SliceLen(t, capture.Calls, 1)
	testutil.Equal(t, "+14155552671", capture.Calls[0].To)
}

// --- Server-level SMS smoke test ---

func setupSMSServer(t *testing.T) (*server.Server, *sms.CaptureProvider) {
	t.Helper()
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.SMSEnabled = true

	authSvc := newAuthService()
	capture := &sms.CaptureProvider{}
	authSvc.SetSMSProvider(capture)
	authSvc.SetSMSConfig(sms.Config{
		CodeLength:       6,
		Expiry:           5 * time.Minute,
		MaxAttempts:      3,
		DailyLimit:       0,
		AllowedCountries: []string{"US", "CA"},
	})

	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
	return srv, capture
}

func TestSMSEndpoints_ServerLevel(t *testing.T) {
	srv, capture := setupSMSServer(t)

	// POST /api/auth/sms → 200.
	w := doJSON(t, srv, "POST", "/api/auth/sms", map[string]string{
		"phone": "+14155552671",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "verification code")

	// Capture the OTP and confirm.
	code := capture.LastCode()
	testutil.True(t, code != "", "should have captured an OTP code")

	w = doJSON(t, srv, "POST", "/api/auth/sms/confirm", map[string]string{
		"phone": "+14155552671",
		"code":  code,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return access token")
	testutil.True(t, resp.RefreshToken != "", "should return refresh token")

	// Verify the returned token actually works on a protected endpoint.
	w = doJSON(t, srv, "GET", "/api/auth/me", nil, resp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var user map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &user))
	testutil.Equal(t, "+14155552671", user["phone"].(string))
}

func TestSMSEndpoints_ServerLevel_WrongCode(t *testing.T) {
	srv, capture := setupSMSServer(t)

	// Request code.
	w := doJSON(t, srv, "POST", "/api/auth/sms", map[string]string{
		"phone": "+14155552671",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.True(t, capture.LastCode() != "", "should have captured an OTP code")

	// Confirm with wrong code → 401.
	w = doJSON(t, srv, "POST", "/api/auth/sms/confirm", map[string]string{
		"phone": "+14155552671",
		"code":  "000000",
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid or expired SMS code")
}

// --- MFA enrollment integration tests (Step 4) ---

func setupMFAService(t *testing.T) (*auth.Service, *sms.CaptureProvider) {
	t.Helper()
	resetAndMigrate(t, t.Context())
	capture := &sms.CaptureProvider{}
	svc := newAuthService()
	svc.SetSMSProvider(capture)
	svc.SetSMSConfig(sms.Config{
		CodeLength:       6,
		Expiry:           5 * time.Minute,
		MaxAttempts:      3,
		DailyLimit:       0,
		AllowedCountries: []string{"US", "CA"},
	})
	return svc, capture
}

func registerTestUser(t *testing.T, svc *auth.Service) *auth.User {
	t.Helper()
	user, _, _, err := svc.Register(t.Context(), "mfa-test@example.com", "password123")
	testutil.NoError(t, err)
	return user
}

func TestEnrollSMSMFA_Success(t *testing.T) {
	svc, capture := setupMFAService(t)
	ctx := t.Context()
	user := registerTestUser(t, svc)

	err := svc.EnrollSMSMFA(ctx, user.ID, "+14155552671")
	testutil.NoError(t, err)

	// Should have sent an OTP.
	testutil.SliceLen(t, capture.Calls, 1)
	testutil.Equal(t, "+14155552671", capture.Calls[0].To)

	code := capture.LastCode()
	testutil.True(t, code != "", "should have captured an OTP code")

	// Verify enrollment row exists with enabled=false.
	var enabled bool
	err = svc.DB().QueryRow(ctx,
		`SELECT enabled FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'sms'`,
		user.ID,
	).Scan(&enabled)
	testutil.NoError(t, err)
	testutil.False(t, enabled, "enrollment should be disabled before confirmation")
}

func TestEnrollSMSMFA_InvalidPhone(t *testing.T) {
	svc, _ := setupMFAService(t)
	ctx := t.Context()
	user := registerTestUser(t, svc)

	err := svc.EnrollSMSMFA(ctx, user.ID, "not-a-phone")
	testutil.True(t, err != nil, "expected error for invalid phone")
	testutil.True(t, errors.Is(err, auth.ErrInvalidPhoneNumber),
		"expected ErrInvalidPhoneNumber, got %v", err)
}

func TestEnrollSMSMFA_AlreadyEnrolled(t *testing.T) {
	svc, capture := setupMFAService(t)
	ctx := t.Context()
	user := registerTestUser(t, svc)

	// Enroll and confirm.
	testutil.NoError(t, svc.EnrollSMSMFA(ctx, user.ID, "+14155552671"))
	code := capture.LastCode()
	testutil.NoError(t, svc.ConfirmSMSMFAEnrollment(ctx, user.ID, "+14155552671", code))

	// Try to enroll again — should fail.
	err := svc.EnrollSMSMFA(ctx, user.ID, "+14155552672")
	testutil.True(t, err != nil, "expected error for already enrolled")
	testutil.True(t, errors.Is(err, auth.ErrMFAAlreadyEnrolled),
		"expected ErrMFAAlreadyEnrolled, got %v", err)
}

func TestConfirmSMSMFAEnrollment_Success(t *testing.T) {
	svc, capture := setupMFAService(t)
	ctx := t.Context()
	user := registerTestUser(t, svc)

	testutil.NoError(t, svc.EnrollSMSMFA(ctx, user.ID, "+14155552671"))
	code := capture.LastCode()

	err := svc.ConfirmSMSMFAEnrollment(ctx, user.ID, "+14155552671", code)
	testutil.NoError(t, err)

	// Verify enrollment is now enabled with enrolled_at set.
	var enabled bool
	var enrolledAt *time.Time
	err = svc.DB().QueryRow(ctx,
		`SELECT enabled, enrolled_at FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'sms'`,
		user.ID,
	).Scan(&enabled, &enrolledAt)
	testutil.NoError(t, err)
	testutil.True(t, enabled, "enrollment should be enabled after confirmation")
	testutil.NotNil(t, enrolledAt)

	// HasSMSMFA should return true.
	has, err := svc.HasSMSMFA(ctx, user.ID)
	testutil.NoError(t, err)
	testutil.True(t, has, "HasSMSMFA should return true after enrollment")
}

func TestConfirmSMSMFAEnrollment_WrongCode(t *testing.T) {
	svc, _ := setupMFAService(t)
	ctx := t.Context()
	user := registerTestUser(t, svc)

	testutil.NoError(t, svc.EnrollSMSMFA(ctx, user.ID, "+14155552671"))

	err := svc.ConfirmSMSMFAEnrollment(ctx, user.ID, "+14155552671", "000000")
	testutil.True(t, err != nil, "expected error for wrong code")
	testutil.True(t, errors.Is(err, auth.ErrInvalidSMSCode),
		"expected ErrInvalidSMSCode, got %v", err)

	// Enrollment should still be disabled.
	var enabled bool
	err = svc.DB().QueryRow(ctx,
		`SELECT enabled FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'sms'`,
		user.ID,
	).Scan(&enabled)
	testutil.NoError(t, err)
	testutil.False(t, enabled, "enrollment should stay disabled after wrong code")
}

// --- MFA challenge/verify integration tests (Step 6) ---

func enrollMFA(t *testing.T, svc *auth.Service, capture *sms.CaptureProvider, userID string) {
	t.Helper()
	ctx := t.Context()
	testutil.NoError(t, svc.EnrollSMSMFA(ctx, userID, "+14155552671"))
	code := capture.LastCode()
	testutil.NoError(t, svc.ConfirmSMSMFAEnrollment(ctx, userID, "+14155552671", code))
	capture.Reset()
}

func TestChallengeSMSMFA_Success(t *testing.T) {
	svc, capture := setupMFAService(t)
	ctx := t.Context()
	user := registerTestUser(t, svc)
	enrollMFA(t, svc, capture, user.ID)

	// Challenge should send an OTP to the enrolled phone.
	err := svc.ChallengeSMSMFA(ctx, user.ID)
	testutil.NoError(t, err)

	testutil.SliceLen(t, capture.Calls, 1)
	testutil.Equal(t, "+14155552671", capture.Calls[0].To)
	testutil.True(t, capture.LastCode() != "", "should have captured an OTP code")
}

func TestVerifySMSMFA_Success(t *testing.T) {
	svc, capture := setupMFAService(t)
	ctx := t.Context()
	user := registerTestUser(t, svc)
	enrollMFA(t, svc, capture, user.ID)

	// Challenge to get OTP.
	testutil.NoError(t, svc.ChallengeSMSMFA(ctx, user.ID))
	code := capture.LastCode()

	// Verify with correct code should issue full tokens.
	returnedUser, accessToken, refreshToken, err := svc.VerifySMSMFA(ctx, user.ID, code, "password")
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, returnedUser.ID)
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")

	// The access token should be a normal (non-MFA-pending) token.
	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.False(t, claims.MFAPending, "verified token should not have MFAPending")
	testutil.Equal(t, user.ID, claims.Subject)
	testutil.Equal(t, "aal2", claims.AAL)
	testutil.Equal(t, 2, len(claims.AMR))
	testutil.Equal(t, "password", claims.AMR[0])
	testutil.Equal(t, "sms_otp", claims.AMR[1])
}

func TestVerifySMSMFA_WrongCode(t *testing.T) {
	svc, capture := setupMFAService(t)
	ctx := t.Context()
	user := registerTestUser(t, svc)
	enrollMFA(t, svc, capture, user.ID)

	testutil.NoError(t, svc.ChallengeSMSMFA(ctx, user.ID))

	_, _, _, err := svc.VerifySMSMFA(ctx, user.ID, "000000", "password")
	testutil.True(t, err != nil, "expected error for wrong code")
	testutil.True(t, errors.Is(err, auth.ErrInvalidSMSCode),
		"expected ErrInvalidSMSCode, got %v", err)
}

func TestHasSMSMFA_NotEnrolled(t *testing.T) {
	svc, _ := setupMFAService(t)
	ctx := t.Context()
	user := registerTestUser(t, svc)

	has, err := svc.HasSMSMFA(ctx, user.ID)
	testutil.NoError(t, err)
	testutil.False(t, has, "user without MFA enrollment should return false")
}

func TestEnrollSMSMFA_ReEnrollAfterDisabledReset(t *testing.T) {
	svc, capture := setupMFAService(t)
	ctx := t.Context()
	user := registerTestUser(t, svc)

	// First enrollment attempt (don't confirm — stays disabled).
	testutil.NoError(t, svc.EnrollSMSMFA(ctx, user.ID, "+14155552671"))
	capture.Reset()

	// Second enrollment should succeed (upserts the disabled row).
	testutil.NoError(t, svc.EnrollSMSMFA(ctx, user.ID, "+14155552672"))
	code := capture.LastCode()
	testutil.True(t, code != "", "should send OTP for re-enrollment")

	// Confirm with the new phone.
	testutil.NoError(t, svc.ConfirmSMSMFAEnrollment(ctx, user.ID, "+14155552672", code))

	has, err := svc.HasSMSMFA(ctx, user.ID)
	testutil.NoError(t, err)
	testutil.True(t, has, "should be enrolled after re-enrollment")
}

// --- MFA login gating tests (Step 6/7) ---

func TestLogin_WithMFA_ReturnsPendingToken(t *testing.T) {
	svc, capture := setupMFAService(t)
	ctx := t.Context()

	// Register user with password.
	user, _, _, err := svc.Register(ctx, "mfa-login@example.com", "password123")
	testutil.NoError(t, err)

	// Enroll and confirm MFA.
	enrollMFA(t, svc, capture, user.ID)

	// Login should return a pending token, not a full token.
	returnedUser, accessToken, refreshToken, err := svc.Login(ctx, "mfa-login@example.com", "password123")
	testutil.NoError(t, err)

	// The returned user should still be present.
	testutil.Equal(t, user.ID, returnedUser.ID)

	// The access token should have MFAPending=true.
	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.True(t, claims.MFAPending, "Login with MFA enrolled should return MFA pending token")

	// No refresh token should be issued for MFA pending login.
	testutil.True(t, refreshToken == "", "Login with MFA should not return refresh token")
}

func TestLogin_WithMFA_FullFlowEndToEnd(t *testing.T) {
	svc, capture := setupMFAService(t)
	ctx := t.Context()

	// Register -> enroll MFA.
	user, _, _, err := svc.Register(ctx, "mfa-e2e@example.com", "password123")
	testutil.NoError(t, err)
	enrollMFA(t, svc, capture, user.ID)

	// Login -> get pending token.
	_, pendingToken, _, err := svc.Login(ctx, "mfa-e2e@example.com", "password123")
	testutil.NoError(t, err)

	pendingClaims, err := svc.ValidateToken(pendingToken)
	testutil.NoError(t, err)
	testutil.True(t, pendingClaims.MFAPending, "should be MFA pending")

	// Challenge -> get OTP.
	testutil.NoError(t, svc.ChallengeSMSMFA(ctx, user.ID))
	code := capture.LastCode()

	// Verify -> get full tokens.
	verifiedUser, fullToken, fullRefresh, err := svc.VerifySMSMFA(ctx, user.ID, code, "password")
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, verifiedUser.ID)
	testutil.True(t, fullToken != "", "should return full access token")
	testutil.True(t, fullRefresh != "", "should return full refresh token")

	// Full token should NOT be MFA pending.
	fullClaims, err := svc.ValidateToken(fullToken)
	testutil.NoError(t, err)
	testutil.False(t, fullClaims.MFAPending, "full token should not be MFA pending")
	testutil.Equal(t, "aal2", fullClaims.AAL)
}

func TestLogin_WithoutMFA_ReturnsNormalTokens(t *testing.T) {
	svc, _ := setupMFAService(t)
	ctx := t.Context()

	// Register user without MFA.
	_, _, _, err := svc.Register(ctx, "no-mfa@example.com", "password123")
	testutil.NoError(t, err)

	// Login should return normal tokens (no MFA pending).
	_, accessToken, refreshToken, err := svc.Login(ctx, "no-mfa@example.com", "password123")
	testutil.NoError(t, err)
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")

	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.False(t, claims.MFAPending, "non-MFA user should get normal token")
}

// --- MFA gating on alternative login methods (Step 7 remaining) ---

func TestConfirmMagicLink_WithMFA_ReturnsPendingToken(t *testing.T) {
	ctx := t.Context()
	svc, capture := setupMFAService(t)
	svc.SetMailer(mailer.NewLogMailer(testutil.DiscardLogger()), "TestApp", "http://localhost:8090/api")
	svc.SetMagicLinkDuration(10 * time.Minute)

	// Register user and enroll MFA.
	user, _, _, err := svc.Register(ctx, "mfa-magic@example.com", "password123")
	testutil.NoError(t, err)
	enrollMFA(t, svc, capture, user.ID)

	// Insert a magic link token directly.
	token := "test-mfa-magic-token"
	hash := auth.HashTokenForTest(token)
	_, err = sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_magic_links (email, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		"mfa-magic@example.com", hash, time.Now().Add(10*time.Minute),
	)
	testutil.NoError(t, err)

	// Confirm magic link — should return MFA pending token.
	returnedUser, accessToken, refreshToken, err := svc.ConfirmMagicLink(ctx, token)
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, returnedUser.ID)

	// Access token should have MFAPending=true.
	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.True(t, claims.MFAPending, "ConfirmMagicLink with MFA enrolled should return MFA pending token")

	// No refresh token should be issued.
	testutil.True(t, refreshToken == "", "ConfirmMagicLink with MFA should not return refresh token")
}

func TestConfirmSMSCode_WithMFA_ReturnsPendingToken(t *testing.T) {
	svc, capture := setupMFAService(t)
	ctx := t.Context()

	// Create user via SMS first-factor, then enroll MFA.
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	user, _, _, err := svc.ConfirmSMSCode(ctx, "+14155552671", capture.LastCode())
	testutil.NoError(t, err)
	capture.Reset()

	enrollMFA(t, svc, capture, user.ID)

	// Login via SMS first-factor again.
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	code := capture.LastCode()

	// Confirm SMS code — should return MFA pending token.
	returnedUser, accessToken, refreshToken, err := svc.ConfirmSMSCode(ctx, "+14155552671", code)
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, returnedUser.ID)

	// Access token should have MFAPending=true.
	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.True(t, claims.MFAPending, "ConfirmSMSCode with MFA enrolled should return MFA pending token")

	// No refresh token should be issued.
	testutil.True(t, refreshToken == "", "ConfirmSMSCode with MFA should not return refresh token")
}

func TestSMSEndpoints_DisabledReturns404(t *testing.T) {
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	// SMSEnabled defaults to false.

	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	w := doJSON(t, srv, "POST", "/api/auth/sms", map[string]string{
		"phone": "+14155552671",
	}, "")
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "not enabled")

	// Confirm endpoint should also return 404.
	w = doJSON(t, srv, "POST", "/api/auth/sms/confirm", map[string]string{
		"phone": "+14155552671", "code": "123456",
	}, "")
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "not enabled")
}

// --- MFA Handler Tests (Steps 8-10) ---

func setupMFAServer(t *testing.T) (*server.Server, *auth.Service, *sms.CaptureProvider) {
	t.Helper()
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.SMSEnabled = true

	authSvc := newAuthService()
	capture := &sms.CaptureProvider{}
	authSvc.SetSMSProvider(capture)
	authSvc.SetSMSConfig(sms.Config{
		CodeLength:       6,
		Expiry:           5 * time.Minute,
		MaxAttempts:      3,
		DailyLimit:       0,
		AllowedCountries: []string{"US", "CA"},
	})

	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
	return srv, authSvc, capture
}

// registerForMFA registers a user and returns their JWT token and user ID.
func registerForMFA(t *testing.T, srv *server.Server, email string) (token string, userID string) {
	t.Helper()
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": email, "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	resp := parseAuthResp(t, w)
	return resp.Token, resp.User["id"].(string)
}

// enrollMFAViaHTTP enrolls and confirms MFA for a user via HTTP endpoints.
func enrollMFAViaHTTP(t *testing.T, srv *server.Server, capture *sms.CaptureProvider, token string) {
	t.Helper()
	doJSON(t, srv, "POST", "/api/auth/mfa/sms/enroll", map[string]string{
		"phone": "+14155552671",
	}, token)
	code := capture.LastCode()
	doJSON(t, srv, "POST", "/api/auth/mfa/sms/enroll/confirm", map[string]string{
		"phone": "+14155552671", "code": code,
	}, token)
	capture.Reset()
}

// loginAndGetPendingToken logs in an MFA-enrolled user and returns the pending token.
func loginAndGetPendingToken(t *testing.T, srv *server.Server, email string) string {
	t.Helper()
	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": email, "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var resp map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp["mfa_token"].(string)
}

func TestHandleMFAEnroll_Success(t *testing.T) {
	srv, _, capture := setupMFAServer(t)
	token, _ := registerForMFA(t, srv, "mfa-enroll@example.com")

	w := doJSON(t, srv, "POST", "/api/auth/mfa/sms/enroll", map[string]string{
		"phone": "+14155552671",
	}, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.SliceLen(t, capture.Calls, 1)
	testutil.Equal(t, "+14155552671", capture.Calls[0].To)
}

func TestHandleMFAEnroll_Unauthenticated(t *testing.T) {
	srv, _, _ := setupMFAServer(t)
	w := doJSON(t, srv, "POST", "/api/auth/mfa/sms/enroll", map[string]string{
		"phone": "+14155552671",
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestHandleMFAEnrollConfirm_Success(t *testing.T) {
	srv, _, capture := setupMFAServer(t)
	token, _ := registerForMFA(t, srv, "mfa-confirm@example.com")

	doJSON(t, srv, "POST", "/api/auth/mfa/sms/enroll", map[string]string{
		"phone": "+14155552671",
	}, token)
	code := capture.LastCode()

	w := doJSON(t, srv, "POST", "/api/auth/mfa/sms/enroll/confirm", map[string]string{
		"phone": "+14155552671", "code": code,
	}, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
}

func TestHandleMFAChallenge_Success(t *testing.T) {
	srv, _, capture := setupMFAServer(t)
	token, _ := registerForMFA(t, srv, "mfa-challenge@example.com")
	enrollMFAViaHTTP(t, srv, capture, token)

	pendingToken := loginAndGetPendingToken(t, srv, "mfa-challenge@example.com")

	w := doJSON(t, srv, "POST", "/api/auth/mfa/sms/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	testutil.SliceLen(t, capture.Calls, 1)
}

func TestHandleMFAChallenge_NotPendingToken(t *testing.T) {
	srv, _, capture := setupMFAServer(t)
	token, _ := registerForMFA(t, srv, "mfa-notpending@example.com")
	enrollMFAViaHTTP(t, srv, capture, token)

	// Regular token (not MFA pending) on challenge should fail.
	w := doJSON(t, srv, "POST", "/api/auth/mfa/sms/challenge", nil, token)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "no MFA challenge pending")
}

func TestHandleMFAVerify_Success(t *testing.T) {
	srv, _, capture := setupMFAServer(t)
	token, _ := registerForMFA(t, srv, "mfa-verify@example.com")
	enrollMFAViaHTTP(t, srv, capture, token)

	pendingToken := loginAndGetPendingToken(t, srv, "mfa-verify@example.com")
	doJSON(t, srv, "POST", "/api/auth/mfa/sms/challenge", nil, pendingToken)
	code := capture.LastCode()

	w := doJSON(t, srv, "POST", "/api/auth/mfa/sms/verify", map[string]string{
		"code": code,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return access token")
	testutil.True(t, resp.RefreshToken != "", "should return refresh token")
	testutil.NotNil(t, resp.User)
}

func TestHandleMFAVerify_WrongCode(t *testing.T) {
	srv, _, capture := setupMFAServer(t)
	token, _ := registerForMFA(t, srv, "mfa-wrong@example.com")
	enrollMFAViaHTTP(t, srv, capture, token)

	pendingToken := loginAndGetPendingToken(t, srv, "mfa-wrong@example.com")
	doJSON(t, srv, "POST", "/api/auth/mfa/sms/challenge", nil, pendingToken)

	w := doJSON(t, srv, "POST", "/api/auth/mfa/sms/verify", map[string]string{
		"code": "000000",
	}, pendingToken)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestHandleMFA_DisabledReturns404(t *testing.T) {
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	// SMSEnabled defaults to false — MFA endpoints should 404.

	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
	token := registerAndGetToken(t, srv, "mfa-disabled@example.com")

	for _, ep := range []string{
		"/api/auth/mfa/sms/enroll",
		"/api/auth/mfa/sms/enroll/confirm",
		"/api/auth/mfa/sms/challenge",
		"/api/auth/mfa/sms/verify",
	} {
		w := doJSON(t, srv, "POST", ep, map[string]string{}, token)
		testutil.StatusCode(t, http.StatusNotFound, w.Code)
	}
}

// --- Step 10: Login response shape ---

func TestHandleLogin_WithMFA_ReturnsMFAResponse(t *testing.T) {
	srv, _, capture := setupMFAServer(t)
	token, _ := registerForMFA(t, srv, "mfa-shape@example.com")
	enrollMFAViaHTTP(t, srv, capture, token)

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "mfa-shape@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var resp map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, true, resp["mfa_pending"].(bool))
	testutil.True(t, resp["mfa_token"].(string) != "", "should have mfa_token")

	// Should NOT have normal auth fields.
	_, hasUser := resp["user"]
	_, hasToken := resp["token"]
	_, hasRefresh := resp["refreshToken"]
	testutil.False(t, hasUser, "MFA response should not include user")
	testutil.False(t, hasToken, "MFA response should not include token")
	testutil.False(t, hasRefresh, "MFA response should not include refreshToken")
}

func TestHandleLogin_WithoutMFA_ReturnsNormalResponse(t *testing.T) {
	srv, _, _ := setupMFAServer(t)
	doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "no-mfa@example.com", "password": "password123",
	}, "")

	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "no-mfa@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return access token")
	testutil.True(t, resp.RefreshToken != "", "should return refresh token")
	testutil.NotNil(t, resp.User)
}

// --- SMS Stats: confirm_count / fail_count tracking (Step 5) ---

func TestSMSStats_ConfirmIncrementsCount(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	// Request and confirm an SMS code successfully.
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))
	code := capture.LastCode()
	testutil.True(t, code != "", "should have captured an OTP code")

	_, _, _, err := svc.ConfirmSMSCode(ctx, "+14155552671", code)
	testutil.NoError(t, err)

	// confirm_count for today should be 1.
	var confirmCount int
	err = svc.DB().QueryRow(ctx,
		`SELECT COALESCE(SUM(confirm_count), 0) FROM _ayb_sms_daily_counts WHERE date = CURRENT_DATE`,
	).Scan(&confirmCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, confirmCount)
}

func TestSMSStats_FailedConfirmIncrementsFailCount(t *testing.T) {
	svc, _ := setupSMSService(t)
	ctx := t.Context()

	// Request an SMS code, then confirm with the wrong code.
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552671"))

	_, _, _, err := svc.ConfirmSMSCode(ctx, "+14155552671", "000000")
	testutil.True(t, errors.Is(err, auth.ErrInvalidSMSCode), "expected ErrInvalidSMSCode")

	// fail_count for today should be 1.
	var failCount int
	err = svc.DB().QueryRow(ctx,
		`SELECT COALESCE(SUM(fail_count), 0) FROM _ayb_sms_daily_counts WHERE date = CURRENT_DATE`,
	).Scan(&failCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, failCount)
}

func TestSMSStats_MultipleConfirmsAccumulate(t *testing.T) {
	svc, capture := setupSMSService(t)
	ctx := t.Context()

	// Two successful confirmations with different phone numbers.
	for _, phone := range []string{"+14155552671", "+14155552672"} {
		testutil.NoError(t, svc.RequestSMSCode(ctx, phone))
		code := capture.LastCode()
		_, _, _, err := svc.ConfirmSMSCode(ctx, phone, code)
		testutil.NoError(t, err)
	}

	// One failed confirmation.
	testutil.NoError(t, svc.RequestSMSCode(ctx, "+14155552673"))
	_, _, _, err := svc.ConfirmSMSCode(ctx, "+14155552673", "000000")
	testutil.True(t, errors.Is(err, auth.ErrInvalidSMSCode), "expected ErrInvalidSMSCode")

	var confirmCount, failCount int
	err = svc.DB().QueryRow(ctx,
		`SELECT COALESCE(SUM(confirm_count), 0), COALESCE(SUM(fail_count), 0) FROM _ayb_sms_daily_counts WHERE date = CURRENT_DATE`,
	).Scan(&confirmCount, &failCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, confirmCount)
	testutil.Equal(t, 1, failCount)
}

// --- Admin SMS Health endpoint integration tests (Step 6) ---

const testAdminPassword = "test-admin-password"

func setupSMSHealthServer(t *testing.T) (*server.Server, *sms.CaptureProvider) {
	t.Helper()
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.SMSEnabled = true
	cfg.Admin.Password = testAdminPassword

	authSvc := newAuthService()
	capture := &sms.CaptureProvider{}
	authSvc.SetSMSProvider(capture)
	authSvc.SetSMSConfig(sms.Config{
		CodeLength:       6,
		Expiry:           5 * time.Minute,
		MaxAttempts:      3,
		DailyLimit:       0,
		AllowedCountries: []string{"US", "CA"},
	})

	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
	return srv, capture
}

func adminLoginForSMS(t *testing.T, srv *server.Server) string {
	t.Helper()
	w := doJSON(t, srv, "POST", "/api/admin/auth", map[string]string{
		"password": testAdminPassword,
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var resp map[string]string
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	token := resp["token"]
	testutil.True(t, token != "", "admin login should return token")
	return token
}

func TestAdminSMSHealth_ReturnsStats(t *testing.T) {
	srv, capture := setupSMSHealthServer(t)
	ctx := t.Context()
	token := adminLoginForSMS(t, srv)

	// Create some known data: 2 sends, 1 confirm, 1 fail.
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_sms_daily_counts (date, count, confirm_count, fail_count)
		 VALUES (CURRENT_DATE, 10, 5, 2)`)
	testutil.NoError(t, err)

	_ = capture // SMS provider not needed for health check

	w := doJSON(t, srv, "GET", "/api/admin/sms/health", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var resp map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Verify "today" section.
	today := resp["today"].(map[string]any)
	testutil.Equal(t, float64(10), today["sent"].(float64))
	testutil.Equal(t, float64(5), today["confirmed"].(float64))
	testutil.Equal(t, float64(2), today["failed"].(float64))
	testutil.Equal(t, float64(50), today["conversion_rate"].(float64))

	// Should also have last_7d and last_30d sections.
	testutil.NotNil(t, resp["last_7d"])
	testutil.NotNil(t, resp["last_30d"])
}

func TestAdminSMSHealth_WarnsLowConversion(t *testing.T) {
	srv, _ := setupSMSHealthServer(t)
	ctx := t.Context()
	token := adminLoginForSMS(t, srv)

	// Create data with very low conversion rate (1/100 = 1%).
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_sms_daily_counts (date, count, confirm_count, fail_count)
		 VALUES (CURRENT_DATE, 100, 1, 50)`)
	testutil.NoError(t, err)

	w := doJSON(t, srv, "GET", "/api/admin/sms/health", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var resp map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	warning, ok := resp["warning"].(string)
	testutil.True(t, ok, "should include a warning field")
	testutil.Contains(t, warning, "low conversion rate")
}

func TestAdminSMSHealth_NoData(t *testing.T) {
	srv, _ := setupSMSHealthServer(t)
	token := adminLoginForSMS(t, srv)

	// No data inserted — all counts should be zero.
	w := doJSON(t, srv, "GET", "/api/admin/sms/health", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var resp map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	today := resp["today"].(map[string]any)
	testutil.Equal(t, float64(0), today["sent"].(float64))
	testutil.Equal(t, float64(0), today["confirmed"].(float64))
	testutil.Equal(t, float64(0), today["failed"].(float64))
	testutil.Equal(t, float64(0), today["conversion_rate"].(float64))
}

func TestAdminSMSHealth_RequiresAdminAuth(t *testing.T) {
	srv, _ := setupSMSHealthServer(t)

	// No auth → 401.
	w := doJSON(t, srv, "GET", "/api/admin/sms/health", nil, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

// --- TOTP MFA Integration Tests ---

func setupTOTPService(t *testing.T) *auth.Service {
	t.Helper()
	resetAndMigrate(t, t.Context())
	svc := newAuthService()
	// Set up AES-256-GCM encryption key for TOTP secrets.
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	if err := svc.SetEncryptionKey(key); err != nil {
		t.Fatalf("setting encryption key: %v", err)
	}
	return svc
}

// registerNamedUser registers a user with a custom email and returns the user ID.
func registerNamedUser(t *testing.T, svc *auth.Service, email string) string {
	t.Helper()
	user, _, _, err := svc.Register(t.Context(), email, "password123")
	if err != nil {
		t.Fatalf("registering user: %v", err)
	}
	return user.ID
}

func TestTOTP_EnrollAndConfirm(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "totp@example.com")

	// Enroll TOTP.
	enrollment, err := svc.EnrollTOTP(ctx, userID, "totp@example.com", "TestApp")
	testutil.NoError(t, err)
	testutil.True(t, enrollment.FactorID != "", "should return factor ID")
	testutil.True(t, enrollment.URI != "", "should return otpauth URI")
	testutil.True(t, enrollment.Secret != "", "should return base32 secret")
	testutil.Contains(t, enrollment.URI, "otpauth://totp/")
	testutil.Contains(t, enrollment.URI, "TestApp")

	// Decode secret and generate a valid TOTP code.
	secret, err := base32Decode(enrollment.Secret)
	testutil.NoError(t, err)
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)

	// Confirm with wrong code should fail.
	err = svc.ConfirmTOTPEnrollment(ctx, userID, "000000")
	testutil.True(t, errors.Is(err, auth.ErrTOTPInvalidCode), "wrong code should fail")

	// Confirm with valid code should succeed.
	err = svc.ConfirmTOTPEnrollment(ctx, userID, code)
	testutil.NoError(t, err)

	// Factor should now be enabled.
	hasTOTP, err := svc.HasTOTPMFA(ctx, userID)
	testutil.NoError(t, err)
	testutil.True(t, hasTOTP, "user should have TOTP enabled after confirm")
}

func TestTOTP_EnrollRejectsAlreadyEnrolled(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "totp-dupe@example.com")

	// Enroll and confirm TOTP.
	enrollment, err := svc.EnrollTOTP(ctx, userID, "totp-dupe@example.com", "TestApp")
	testutil.NoError(t, err)
	secret, err := base32Decode(enrollment.Secret)
	testutil.NoError(t, err)
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)
	testutil.NoError(t, svc.ConfirmTOTPEnrollment(ctx, userID, code))

	// Second enrollment should fail.
	_, err = svc.EnrollTOTP(ctx, userID, "totp-dupe@example.com", "TestApp")
	testutil.True(t, errors.Is(err, auth.ErrTOTPAlreadyEnrolled), "duplicate enroll should be rejected")
}

func TestTOTP_ChallengeAndVerify(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "totp-cv@example.com")

	// Enroll + confirm TOTP.
	enrollment, err := svc.EnrollTOTP(ctx, userID, "totp-cv@example.com", "TestApp")
	testutil.NoError(t, err)
	secret, err := base32Decode(enrollment.Secret)
	testutil.NoError(t, err)
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)
	testutil.NoError(t, svc.ConfirmTOTPEnrollment(ctx, userID, code))

	// Create challenge.
	challengeID, err := svc.CreateTOTPChallenge(ctx, userID, "127.0.0.1")
	testutil.NoError(t, err)
	testutil.True(t, challengeID != "", "should return challenge ID")

	// Verify with wrong code.
	_, _, _, err = svc.VerifyTOTPChallenge(ctx, userID, challengeID, "000000", "password")
	testutil.True(t, errors.Is(err, auth.ErrTOTPInvalidCode), "wrong code should fail")

	// Verify with valid code (generate fresh for current step).
	step = time.Now().Unix() / auth.TOTPPeriodForTest
	validCode := auth.GenerateTOTPCodeForTest(secret, step)

	// The challenge was already used for a failed attempt but that's fine — it's single-use only on success.
	// Actually, per the code, wrong code doesn't mark challenge as used. Let me verify.
	user, accessToken, refreshToken, err := svc.VerifyTOTPChallenge(ctx, userID, challengeID, validCode, "password")
	testutil.NoError(t, err)
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")
	testutil.NotNil(t, user)
	testutil.Equal(t, userID, user.ID)
}

func TestTOTP_ChallengeRejectsSingleUse(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "totp-single@example.com")

	// Enroll + confirm TOTP.
	enrollment, err := svc.EnrollTOTP(ctx, userID, "totp-single@example.com", "TestApp")
	testutil.NoError(t, err)
	secret, err := base32Decode(enrollment.Secret)
	testutil.NoError(t, err)
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)
	testutil.NoError(t, svc.ConfirmTOTPEnrollment(ctx, userID, code))

	// Create and verify challenge.
	challengeID, err := svc.CreateTOTPChallenge(ctx, userID, "127.0.0.1")
	testutil.NoError(t, err)
	step = time.Now().Unix() / auth.TOTPPeriodForTest
	validCode := auth.GenerateTOTPCodeForTest(secret, step)
	_, _, _, err = svc.VerifyTOTPChallenge(ctx, userID, challengeID, validCode, "password")
	testutil.NoError(t, err)

	// Re-using the same challenge should fail.
	step = time.Now().Unix() / auth.TOTPPeriodForTest
	anotherCode := auth.GenerateTOTPCodeForTest(secret, step)
	_, _, _, err = svc.VerifyTOTPChallenge(ctx, userID, challengeID, anotherCode, "password")
	testutil.True(t, errors.Is(err, auth.ErrTOTPChallengeUsed), "reused challenge should be rejected")
}

func TestTOTP_ReplayProtection(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "totp-replay@example.com")

	// Enroll + confirm TOTP.
	enrollment, err := svc.EnrollTOTP(ctx, userID, "totp-replay@example.com", "TestApp")
	testutil.NoError(t, err)
	secret, err := base32Decode(enrollment.Secret)
	testutil.NoError(t, err)
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)
	testutil.NoError(t, svc.ConfirmTOTPEnrollment(ctx, userID, code))

	// First challenge + verify succeeds.
	chal1, err := svc.CreateTOTPChallenge(ctx, userID, "127.0.0.1")
	testutil.NoError(t, err)
	step = time.Now().Unix() / auth.TOTPPeriodForTest
	code1 := auth.GenerateTOTPCodeForTest(secret, step)
	_, _, _, err = svc.VerifyTOTPChallenge(ctx, userID, chal1, code1, "password")
	testutil.NoError(t, err)

	// Second challenge with same code (same time step) should be rejected as replay.
	chal2, err := svc.CreateTOTPChallenge(ctx, userID, "127.0.0.1")
	testutil.NoError(t, err)
	_, _, _, err = svc.VerifyTOTPChallenge(ctx, userID, chal2, code1, "password")
	testutil.True(t, errors.Is(err, auth.ErrTOTPReplay), "replayed code should be rejected")
}

func TestTOTP_ChallengeRequiresEnrolledFactor(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "totp-nonenrolled@example.com")

	// No TOTP enrolled — challenge should fail.
	_, err := svc.CreateTOTPChallenge(ctx, userID, "127.0.0.1")
	testutil.True(t, errors.Is(err, auth.ErrTOTPNotEnrolled), "challenge without enrollment should fail")
}

func TestTOTP_ConfirmWithoutEnrollment(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "totp-noconfirm@example.com")

	// Confirm without enrollment should fail.
	err := svc.ConfirmTOTPEnrollment(ctx, userID, "123456")
	testutil.True(t, errors.Is(err, auth.ErrTOTPNotEnrolled), "confirm without enrollment should fail")
}

func TestTOTP_CleanupUnverifiedEnrollments(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "totp-cleanup@example.com")

	// Start enrollment but don't confirm.
	_, err := svc.EnrollTOTP(ctx, userID, "totp-cleanup@example.com", "TestApp")
	testutil.NoError(t, err)

	// Cleanup with 0 TTL should delete it.
	err = svc.CleanupUnverifiedTOTPEnrollments(ctx, 0)
	testutil.NoError(t, err)

	// Factor should be gone — enrollment should succeed again (not get "already enrolled").
	_, err = svc.EnrollTOTP(ctx, userID, "totp-cleanup@example.com", "TestApp")
	testutil.NoError(t, err)
}

func TestTOTP_MFAFactors(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "totp-factors@example.com")

	// No factors initially.
	factors, err := svc.GetUserMFAFactors(ctx, userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 0)

	// Enroll + confirm TOTP.
	enrollment, err := svc.EnrollTOTP(ctx, userID, "totp-factors@example.com", "TestApp")
	testutil.NoError(t, err)
	secret, err := base32Decode(enrollment.Secret)
	testutil.NoError(t, err)
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)
	testutil.NoError(t, svc.ConfirmTOTPEnrollment(ctx, userID, code))

	// Should now have 1 TOTP factor.
	factors, err = svc.GetUserMFAFactors(ctx, userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, factors, 1)
	testutil.Equal(t, "totp", factors[0].Method)
}

// base32Decode decodes a base32 string (no padding) to bytes.
func base32Decode(s string) ([]byte, error) {
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
}

// --- Backup Code Integration Tests ---

func TestBackupCodes_GenerateAndVerify(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "backup@example.com")

	// Generate codes.
	codes, err := svc.GenerateBackupCodes(ctx, userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, codes, 10)

	// All codes should be in xxxxx-xxxxx format.
	for _, code := range codes {
		testutil.Equal(t, 11, len(code))
		testutil.Contains(t, code, "-")
	}

	// Count should be 10.
	count, err := svc.GetBackupCodeCount(ctx, userID)
	testutil.NoError(t, err)
	testutil.Equal(t, 10, count)

	// Verify the first code.
	err = svc.VerifyBackupCode(ctx, userID, codes[0])
	testutil.NoError(t, err)

	// Count should now be 9.
	count, err = svc.GetBackupCodeCount(ctx, userID)
	testutil.NoError(t, err)
	testutil.Equal(t, 9, count)
}

func TestBackupCodes_SingleUse(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "backup-single@example.com")

	codes, err := svc.GenerateBackupCodes(ctx, userID)
	testutil.NoError(t, err)

	// Use code once — should work.
	err = svc.VerifyBackupCode(ctx, userID, codes[0])
	testutil.NoError(t, err)

	// Use same code again — should fail.
	err = svc.VerifyBackupCode(ctx, userID, codes[0])
	testutil.True(t, errors.Is(err, auth.ErrBackupCodeInvalid), "reused backup code should be rejected")
}

func TestBackupCodes_InvalidCode(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "backup-invalid@example.com")

	_, err := svc.GenerateBackupCodes(ctx, userID)
	testutil.NoError(t, err)

	// Invalid code should fail.
	err = svc.VerifyBackupCode(ctx, userID, "zzzzz-zzzzz")
	testutil.True(t, errors.Is(err, auth.ErrBackupCodeInvalid), "invalid code should be rejected")
}

func TestBackupCodes_RegenerateInvalidatesOld(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "backup-regen@example.com")

	// Generate first set.
	oldCodes, err := svc.GenerateBackupCodes(ctx, userID)
	testutil.NoError(t, err)

	// Regenerate (GenerateBackupCodes deletes old codes first).
	newCodes, err := svc.GenerateBackupCodes(ctx, userID)
	testutil.NoError(t, err)
	testutil.SliceLen(t, newCodes, 10)

	// Old codes should no longer work.
	err = svc.VerifyBackupCode(ctx, userID, oldCodes[0])
	testutil.True(t, errors.Is(err, auth.ErrBackupCodeInvalid), "old codes should be invalidated")

	// New codes should work.
	err = svc.VerifyBackupCode(ctx, userID, newCodes[0])
	testutil.NoError(t, err)
}

func TestBackupCodes_CaseInsensitive(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "backup-case@example.com")

	codes, err := svc.GenerateBackupCodes(ctx, userID)
	testutil.NoError(t, err)

	// Verify with uppercase version.
	upper := strings.ToUpper(codes[0])
	err = svc.VerifyBackupCode(ctx, userID, upper)
	testutil.NoError(t, err)
}

func TestBackupCodes_VerifyMFA_IssuesAAL2Tokens(t *testing.T) {
	svc := setupTOTPService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "backup-mfa@example.com")

	codes, err := svc.GenerateBackupCodes(ctx, userID)
	testutil.NoError(t, err)

	// VerifyBackupCodeMFA should issue AAL2 tokens.
	user, accessToken, refreshToken, err := svc.VerifyBackupCodeMFA(ctx, userID, codes[0], "password")
	testutil.NoError(t, err)
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")
	testutil.NotNil(t, user)
	testutil.Equal(t, userID, user.ID)

	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.False(t, claims.MFAPending, "verified token should not be MFA pending")
	testutil.Equal(t, "aal2", claims.AAL)
	testutil.Equal(t, 2, len(claims.AMR))
	testutil.Equal(t, "password", claims.AMR[0])
	testutil.Equal(t, "backup_code", claims.AMR[1])
}

// --- Email MFA Integration Tests ---

func setupEmailMFAService(t *testing.T) *auth.Service {
	t.Helper()
	resetAndMigrate(t, t.Context())
	svc := newAuthService()
	// No mailer — email sending is a no-op, but challenge/verify flow still works.
	return svc
}

type captureEmailMailer struct {
	mu    sync.Mutex
	calls []mailer.Message
}

func (m *captureEmailMailer) Send(_ context.Context, msg *mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if msg == nil {
		return nil
	}
	m.calls = append(m.calls, *msg)
	return nil
}

func (m *captureEmailMailer) LastCode() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return ""
	}
	return strings.TrimSpace(m.calls[len(m.calls)-1].Text)
}

type codeOnlyTemplateRenderer struct{}

func (codeOnlyTemplateRenderer) RenderWithFallback(_ context.Context, _ string, vars map[string]string) (string, string, string, error) {
	code := strings.TrimSpace(vars["Code"])
	return "Test MFA code", code, code, nil
}

func setupEmailMFAServiceWithCapture(t *testing.T) (*auth.Service, *captureEmailMailer) {
	t.Helper()
	resetAndMigrate(t, t.Context())
	svc := newAuthService()
	capture := &captureEmailMailer{}
	svc.SetMailer(capture, "TestApp", "http://localhost:8090/api")
	svc.SetEmailTemplateService(codeOnlyTemplateRenderer{})
	return svc, capture
}

func TestEmailMFA_EnrollAndConfirm(t *testing.T) {
	svc, capture := setupEmailMFAServiceWithCapture(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "emailmfa@example.com")

	// Enroll email MFA.
	err := svc.EnrollEmailMFA(ctx, userID, "emailmfa@example.com")
	testutil.NoError(t, err)

	// Retrieve the code from the challenge row directly (no mailer in test).
	var challengeID string
	var codeHash string
	err = svc.DB().QueryRow(ctx,
		`SELECT c.id, c.otp_code_hash FROM _ayb_mfa_challenges c
		 JOIN _ayb_user_mfa f ON f.id = c.factor_id
		 WHERE f.user_id = $1 AND f.method = 'email'
		 ORDER BY c.created_at DESC LIMIT 1`, userID,
	).Scan(&challengeID, &codeHash)
	testutil.NoError(t, err)
	testutil.True(t, codeHash != "", "should have stored code hash")

	// Wrong code should fail.
	err = svc.ConfirmEmailMFAEnrollment(ctx, userID, "000000")
	testutil.True(t, errors.Is(err, auth.ErrEmailMFAInvalidCode), "wrong code should fail")

	// Verify wrong-code attempt incremented attempt_count.
	var attemptCount int
	err = svc.DB().QueryRow(ctx,
		`SELECT attempt_count FROM _ayb_mfa_challenges WHERE id = $1`, challengeID,
	).Scan(&attemptCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, attemptCount)

	// Confirm with the real code captured from email.
	code := capture.LastCode()
	testutil.True(t, code != "", "should capture enrollment code")
	err = svc.ConfirmEmailMFAEnrollment(ctx, userID, code)
	testutil.NoError(t, err)

	// Factor should now be enabled.
	var enabled bool
	err = svc.DB().QueryRow(ctx,
		`SELECT enabled FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'email'`, userID,
	).Scan(&enabled)
	testutil.NoError(t, err)
	testutil.True(t, enabled, "email MFA factor should be enabled after confirmation")
}

func TestEmailMFA_AttemptCountProgression(t *testing.T) {
	svc := setupEmailMFAService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "emailmfa-attempts@example.com")

	err := svc.EnrollEmailMFA(ctx, userID, "emailmfa-attempts@example.com")
	testutil.NoError(t, err)

	// Make 5 wrong attempts — should increment attempt_count to 5.
	for i := 0; i < 5; i++ {
		err = svc.ConfirmEmailMFAEnrollment(ctx, userID, "000000")
		testutil.True(t, errors.Is(err, auth.ErrEmailMFAInvalidCode), "wrong code should fail")
	}

	// 6th attempt should still fail (code invalidated after 5 failures).
	err = svc.ConfirmEmailMFAEnrollment(ctx, userID, "000000")
	testutil.True(t, errors.Is(err, auth.ErrEmailMFAInvalidCode), "should reject after max attempts")

	// Verify attempt_count in DB is 5 (should stop incrementing at max).
	var challengeID string
	var attemptCount int
	err = svc.DB().QueryRow(ctx,
		`SELECT c.id, c.attempt_count FROM _ayb_mfa_challenges c
		 JOIN _ayb_user_mfa f ON f.id = c.factor_id
		 WHERE f.user_id = $1 AND f.method = 'email'
		 ORDER BY c.created_at DESC LIMIT 1`, userID,
	).Scan(&challengeID, &attemptCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 5, attemptCount)
}

func TestEmailMFA_ChallengeRateLimit(t *testing.T) {
	svc := setupEmailMFAService(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "emailmfa-rate@example.com")

	// Enroll and confirm email MFA by directly inserting an enabled factor.
	_, err := svc.DB().Exec(ctx,
		`INSERT INTO _ayb_user_mfa (user_id, method, enabled, enrolled_at)
		 VALUES ($1, 'email', true, NOW())`, userID)
	testutil.NoError(t, err)

	// Create 3 challenges (the max per 10 min).
	for i := 0; i < 3; i++ {
		_, err = svc.ChallengeEmailMFA(ctx, userID, "emailmfa-rate@example.com")
		testutil.NoError(t, err)
	}

	// 4th challenge should be rate-limited.
	_, err = svc.ChallengeEmailMFA(ctx, userID, "emailmfa-rate@example.com")
	testutil.True(t, errors.Is(err, auth.ErrEmailMFARateLimit), "4th challenge should be rate limited")
}

func TestEmailMFA_VerifyRejectsUnverifiedEnrollmentChallenge(t *testing.T) {
	svc, capture := setupEmailMFAServiceWithCapture(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "emailmfa-unverified@example.com")

	// Enrollment creates an unverified factor plus challenge code.
	err := svc.EnrollEmailMFA(ctx, userID, "emailmfa-unverified@example.com")
	testutil.NoError(t, err)

	code := capture.LastCode()
	testutil.True(t, code != "", "should capture enrollment code")

	var challengeID string
	err = svc.DB().QueryRow(ctx,
		`SELECT c.id FROM _ayb_mfa_challenges c
		 JOIN _ayb_user_mfa f ON f.id = c.factor_id
		 WHERE f.user_id = $1 AND f.method = 'email'
		 ORDER BY c.created_at DESC LIMIT 1`,
		userID,
	).Scan(&challengeID)
	testutil.NoError(t, err)

	user, accessToken, refreshToken, err := svc.VerifyEmailMFA(ctx, userID, challengeID, code, "password")
	testutil.True(t, errors.Is(err, auth.ErrTOTPChallengeNotFound), "verify should reject enrollment challenge from unverified factor")
	testutil.Nil(t, user)
	testutil.Equal(t, "", accessToken)
	testutil.Equal(t, "", refreshToken)
}

func TestEmailMFA_ChallengeAndVerifyIssuesAAL2Tokens(t *testing.T) {
	svc, capture := setupEmailMFAServiceWithCapture(t)
	ctx := t.Context()

	email := "emailmfa-verify@example.com"
	userID := registerNamedUser(t, svc, email)

	// Enroll + confirm factor first.
	err := svc.EnrollEmailMFA(ctx, userID, email)
	testutil.NoError(t, err)
	enrollCode := capture.LastCode()
	testutil.True(t, enrollCode != "", "should capture enrollment code")
	err = svc.ConfirmEmailMFAEnrollment(ctx, userID, enrollCode)
	testutil.NoError(t, err)

	// Create verify challenge for enabled factor.
	challengeID, err := svc.ChallengeEmailMFA(ctx, userID, email)
	testutil.NoError(t, err)
	testutil.True(t, challengeID != "", "should return challenge ID")
	verifyCode := capture.LastCode()
	testutil.True(t, verifyCode != "", "should capture challenge code")

	user, accessToken, refreshToken, err := svc.VerifyEmailMFA(ctx, userID, challengeID, verifyCode, "password")
	testutil.NoError(t, err)
	testutil.NotNil(t, user)
	testutil.Equal(t, userID, user.ID)
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")

	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.False(t, claims.MFAPending, "verified token should not be MFA pending")
	testutil.Equal(t, "aal2", claims.AAL)
	testutil.Equal(t, 2, len(claims.AMR))
	testutil.Equal(t, "password", claims.AMR[0])
	testutil.Equal(t, "email_otp", claims.AMR[1])
}

// --- Anonymous Auth Integration Tests ---

func TestAnonymous_CreateUser(t *testing.T) {
	svc := setupAnonymousService(t)
	ctx := t.Context()

	user, accessToken, refreshToken, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)
	testutil.NotNil(t, user)
	testutil.True(t, user.IsAnonymous, "user should be anonymous")
	testutil.Equal(t, "", user.Email)
	testutil.True(t, accessToken != "", "should return access token")
	testutil.True(t, refreshToken != "", "should return refresh token")

	// Token should have is_anonymous claim.
	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.True(t, claims.IsAnonymous, "JWT should have is_anonymous=true")
	testutil.Equal(t, "aal1", claims.AAL)
}

func TestAnonymous_LinkEmail_PreservesUserID(t *testing.T) {
	svc := setupAnonymousService(t)
	ctx := t.Context()

	// Create anonymous user.
	anonUser, _, _, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)
	originalID := anonUser.ID

	// Link email.
	linkedUser, accessToken, _, err := svc.LinkEmail(ctx, originalID, "linked@example.com", "password123")
	testutil.NoError(t, err)
	testutil.Equal(t, originalID, linkedUser.ID)
	testutil.False(t, linkedUser.IsAnonymous, "user should no longer be anonymous")
	testutil.Equal(t, "linked@example.com", linkedUser.Email)
	testutil.NotNil(t, linkedUser.LinkedAt)

	// New token should not have is_anonymous.
	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.False(t, claims.IsAnonymous, "linked user JWT should have is_anonymous=false")
}

func TestAnonymous_LinkEmail_Conflict(t *testing.T) {
	svc := setupAnonymousService(t)
	ctx := t.Context()

	// Register a normal user with this email first.
	_, _, _, err := svc.Register(ctx, "taken@example.com", "password123")
	testutil.NoError(t, err)

	// Create anonymous user and try to link with the same email.
	anonUser, _, _, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)

	_, _, _, err = svc.LinkEmail(ctx, anonUser.ID, "taken@example.com", "password123")
	testutil.True(t, errors.Is(err, auth.ErrLinkConflict), "should fail with link conflict")
}

func TestAnonymous_LinkEmail_NotAnonymous(t *testing.T) {
	svc := setupAnonymousService(t)
	ctx := t.Context()

	// Register a normal user.
	user, _, _, err := svc.Register(ctx, "normal@example.com", "password123")
	testutil.NoError(t, err)

	// Try to link — should fail because user is not anonymous.
	_, _, _, err = svc.LinkEmail(ctx, user.ID, "other@example.com", "password123")
	testutil.True(t, errors.Is(err, auth.ErrNotAnonymous), "should reject non-anonymous user")
}

func TestAnonymous_LinkEmail_DoubleLink(t *testing.T) {
	svc := setupAnonymousService(t)
	ctx := t.Context()

	anonUser, _, _, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)

	// First link succeeds.
	_, _, _, err = svc.LinkEmail(ctx, anonUser.ID, "first@example.com", "password123")
	testutil.NoError(t, err)

	// Second link fails — user is no longer anonymous.
	_, _, _, err = svc.LinkEmail(ctx, anonUser.ID, "second@example.com", "password123")
	testutil.True(t, errors.Is(err, auth.ErrNotAnonymous), "double-link should fail")
}

func TestAnonymous_LinkOAuth(t *testing.T) {
	svc := setupAnonymousService(t)
	ctx := t.Context()

	anonUser, _, _, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)
	originalID := anonUser.ID

	info := &auth.OAuthUserInfo{
		ProviderUserID: "github-12345",
		Email:          "oauth@example.com",
		Name:           "OAuth User",
	}
	linkedUser, accessToken, _, err := svc.LinkOAuth(ctx, originalID, "github", info)
	testutil.NoError(t, err)
	testutil.Equal(t, originalID, linkedUser.ID)
	testutil.False(t, linkedUser.IsAnonymous, "should no longer be anonymous")
	testutil.Equal(t, "oauth@example.com", linkedUser.Email)

	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.False(t, claims.IsAnonymous, "linked user JWT should not be anonymous")
}

func TestAnonymous_LinkOAuth_Conflict(t *testing.T) {
	svc := setupAnonymousService(t)
	ctx := t.Context()

	// Create first anonymous user and link with a GitHub identity.
	anon1, _, _, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)
	_, _, _, err = svc.LinkOAuth(ctx, anon1.ID, "github", &auth.OAuthUserInfo{
		ProviderUserID: "github-99",
		Email:          "first-oauth@example.com",
		Name:           "First",
	})
	testutil.NoError(t, err)

	// Create second anonymous user and try to link with the same GitHub identity.
	anon2, _, _, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)
	_, _, _, err = svc.LinkOAuth(ctx, anon2.ID, "github", &auth.OAuthUserInfo{
		ProviderUserID: "github-99",
		Email:          "second-oauth@example.com",
		Name:           "Second",
	})
	testutil.True(t, errors.Is(err, auth.ErrOAuthLinkConflict), "duplicate provider identity should conflict")
}

func TestAnonymous_Cleanup(t *testing.T) {
	svc := setupAnonymousService(t)
	ctx := t.Context()

	// Create two anonymous users.
	_, _, _, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)
	_, _, _, err = svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)

	// Cleanup with zero TTL should delete all (they were just created, but 0 TTL means cutoff is now).
	// Actually, zero TTL means cutoff = now, so users created_at < now should be caught.
	// Wait briefly to ensure created_at is before the cutoff.
	time.Sleep(10 * time.Millisecond)

	count, err := svc.CleanupAnonymousUsers(ctx, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(2), count)

	// Create one more, link it, then cleanup — linked user should survive.
	anon, _, _, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)
	_, _, _, err = svc.LinkEmail(ctx, anon.ID, "survivor@example.com", "password123")
	testutil.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	count, err = svc.CleanupAnonymousUsers(ctx, 0)
	testutil.NoError(t, err)
	testutil.Equal(t, int64(0), count)
}

func TestAnonymous_LoginAfterLink(t *testing.T) {
	svc := setupAnonymousService(t)
	ctx := t.Context()

	// Create anonymous user and link with email.
	anonUser, _, _, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)
	_, _, _, err = svc.LinkEmail(ctx, anonUser.ID, "logintest@example.com", "password123")
	testutil.NoError(t, err)

	// Should be able to login with email/password now.
	user, accessToken, _, err := svc.Login(ctx, "logintest@example.com", "password123")
	testutil.NoError(t, err)
	testutil.Equal(t, anonUser.ID, user.ID)
	testutil.False(t, user.IsAnonymous, "logged-in linked user should not be anonymous")

	claims, err := svc.ValidateToken(accessToken)
	testutil.NoError(t, err)
	testutil.False(t, claims.IsAnonymous, "login token should not be anonymous")
	testutil.Equal(t, "aal1", claims.AAL)
}

func setupAnonymousService(t *testing.T) *auth.Service {
	t.Helper()
	resetAndMigrate(t, t.Context())
	return newAuthService()
}

// --- Anonymous Auth API (HTTP Handler) Tests ---

func setupAnonymousServer(t *testing.T) *server.Server {
	t.Helper()
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.AnonymousAuthEnabled = true
	cfg.Auth.TOTPEnabled = true
	cfg.Auth.EmailMFAEnabled = true

	authSvc := newAuthService()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	if err := authSvc.SetEncryptionKey(key); err != nil {
		t.Fatalf("setting encryption key: %v", err)
	}
	return server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
}

func TestAnonymousAPI_SignIn(t *testing.T) {
	srv := setupAnonymousServer(t)

	w := doJSON(t, srv, "POST", "/api/auth/anonymous", nil, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return access token")
	testutil.True(t, resp.RefreshToken != "", "should return refresh token")
	testutil.True(t, resp.User["is_anonymous"] == true, "user should be anonymous")
}

func TestAnonymousAPI_Disabled(t *testing.T) {
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	// AnonymousAuthEnabled defaults to false.

	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)

	w := doJSON(t, srv, "POST", "/api/auth/anonymous", nil, "")
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "not enabled")
}

func TestAnonymousAPI_LinkEmail_Success(t *testing.T) {
	srv := setupAnonymousServer(t)

	// Create anonymous user.
	w := doJSON(t, srv, "POST", "/api/auth/anonymous", nil, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	anonResp := parseAuthResp(t, w)
	anonID := anonResp.User["id"].(string)

	// Link email.
	w = doJSON(t, srv, "POST", "/api/auth/link/email", map[string]string{
		"email": "apilink@example.com", "password": "password123",
	}, anonResp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	linkResp := parseAuthResp(t, w)
	testutil.Equal(t, anonID, linkResp.User["id"].(string))
	testutil.True(t, linkResp.User["is_anonymous"] == false || linkResp.User["is_anonymous"] == nil,
		"linked user should not be anonymous")
}

func TestAnonymousAPI_LinkEmail_Conflict(t *testing.T) {
	srv := setupAnonymousServer(t)

	// Register a normal user first.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "existing@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)

	// Create anonymous user.
	w = doJSON(t, srv, "POST", "/api/auth/anonymous", nil, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	anonResp := parseAuthResp(t, w)

	// Link with existing email — should get 409.
	w = doJSON(t, srv, "POST", "/api/auth/link/email", map[string]string{
		"email": "existing@example.com", "password": "password123",
	}, anonResp.Token)
	testutil.StatusCode(t, http.StatusConflict, w.Code)
	testutil.Contains(t, w.Body.String(), "already belongs")
}

func TestAnonymousAPI_MFAEnrollBlocked(t *testing.T) {
	srv := setupAnonymousServer(t)

	// Create anonymous user.
	w := doJSON(t, srv, "POST", "/api/auth/anonymous", nil, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	anonResp := parseAuthResp(t, w)

	// Try TOTP enroll — should be blocked for anonymous user.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/enroll", nil, anonResp.Token)
	testutil.StatusCode(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "link your account")

	// Try email MFA enroll — should also be blocked.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll", nil, anonResp.Token)
	testutil.StatusCode(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "link your account")
}

func TestAnonymousAPI_LinkThenLogin(t *testing.T) {
	srv := setupAnonymousServer(t)

	// Create anonymous user and link email.
	w := doJSON(t, srv, "POST", "/api/auth/anonymous", nil, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	anonResp := parseAuthResp(t, w)
	originalID := anonResp.User["id"].(string)

	w = doJSON(t, srv, "POST", "/api/auth/link/email", map[string]string{
		"email": "linklogin@example.com", "password": "password123",
	}, anonResp.Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Login with the linked credentials.
	w = doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "linklogin@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)

	loginResp := parseAuthResp(t, w)
	testutil.Equal(t, originalID, loginResp.User["id"].(string))
}

func TestAnonymousAPI_LinkEmail_MissingFields(t *testing.T) {
	srv := setupAnonymousServer(t)

	w := doJSON(t, srv, "POST", "/api/auth/anonymous", nil, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	anonResp := parseAuthResp(t, w)

	// Missing email.
	w = doJSON(t, srv, "POST", "/api/auth/link/email", map[string]string{
		"password": "password123",
	}, anonResp.Token)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)

	// Missing password.
	w = doJSON(t, srv, "POST", "/api/auth/link/email", map[string]string{
		"email": "test@example.com",
	}, anonResp.Token)
	testutil.StatusCode(t, http.StatusBadRequest, w.Code)
}

func TestAnonymousAPI_LinkEmail_NotAuthenticated(t *testing.T) {
	srv := setupAnonymousServer(t)

	w := doJSON(t, srv, "POST", "/api/auth/link/email", map[string]string{
		"email": "test@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
}

func TestAnonymousAPI_LinkEmail_NotAnonymous(t *testing.T) {
	srv := setupAnonymousServer(t)

	// Register a normal user.
	w := doJSON(t, srv, "POST", "/api/auth/register", map[string]string{
		"email": "normal@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusCreated, w.Code)
	normalResp := parseAuthResp(t, w)

	// Try to link as a non-anonymous user — should get 403.
	w = doJSON(t, srv, "POST", "/api/auth/link/email", map[string]string{
		"email": "other@example.com", "password": "password123",
	}, normalResp.Token)
	testutil.StatusCode(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "only anonymous")
}

// =======================================================================
// TOTP MFA API Tests (HTTP-level)
// =======================================================================

// setupMFAAPIServer creates a server with TOTP, email MFA, anonymous auth, and a
// capture mailer for email MFA tests.
func setupMFAAPIServer(t *testing.T) (*server.Server, *auth.Service, *captureEmailMailer) {
	t.Helper()
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	if err := ch.Load(ctx); err != nil {
		t.Fatalf("loading schema cache: %v", err)
	}

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.Auth.TOTPEnabled = true
	cfg.Auth.EmailMFAEnabled = true
	cfg.Auth.AnonymousAuthEnabled = true
	cfg.Auth.RateLimit = 500 // high limit for tests

	authSvc := newAuthService()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	if err := authSvc.SetEncryptionKey(key); err != nil {
		t.Fatalf("setting encryption key: %v", err)
	}

	capture := &captureEmailMailer{}
	authSvc.SetMailer(capture, "TestApp", "http://localhost:8090/api")
	authSvc.SetEmailTemplateService(codeOnlyTemplateRenderer{})

	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
	return srv, authSvc, capture
}

// enrollTOTPViaAPI enrolls and confirms TOTP for a user via HTTP. Returns
// the decoded TOTP secret bytes so the caller can generate codes.
func enrollTOTPViaAPI(t *testing.T, srv *server.Server, authSvc *auth.Service, token string) []byte {
	t.Helper()

	w := doJSON(t, srv, "POST", "/api/auth/mfa/totp/enroll", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	var enrollResp struct {
		FactorID string `json:"factor_id"`
		URI      string `json:"uri"`
		Secret   string `json:"secret"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &enrollResp))
	testutil.True(t, enrollResp.Secret != "", "should return base32 secret")

	secret, err := base32Decode(enrollResp.Secret)
	testutil.NoError(t, err)

	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)

	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/enroll/confirm", map[string]string{
		"code": code,
	}, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	return secret
}

func TestTOTPAPI_EnrollConfirmChallengeVerify_HappyPath(t *testing.T) {
	srv, authSvc, _ := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "totp-api-happy@example.com")

	// Enroll + confirm TOTP via API.
	secret := enrollTOTPViaAPI(t, srv, authSvc, token)

	// Login to get MFA pending token.
	w := doJSON(t, srv, "POST", "/api/auth/login", map[string]string{
		"email": "totp-api-happy@example.com", "password": "password123",
	}, "")
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var loginResp map[string]any
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &loginResp))
	testutil.Equal(t, true, loginResp["mfa_pending"].(bool))
	pendingToken := loginResp["mfa_token"].(string)

	// Get factors.
	w = doJSON(t, srv, "GET", "/api/auth/mfa/factors", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var factorsResp struct {
		Factors []map[string]any `json:"factors"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &factorsResp))
	testutil.True(t, len(factorsResp.Factors) >= 1, "should have at least one factor")
	testutil.Equal(t, "totp", factorsResp.Factors[0]["method"])

	// Challenge.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))
	testutil.True(t, chalResp.ChallengeID != "", "should return challenge ID")

	// Verify with valid code.
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)

	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         code,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return access token")
	testutil.True(t, resp.RefreshToken != "", "should return refresh token")

	// Verify the token has AAL2.
	claims, err := authSvc.ValidateToken(resp.Token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal2", claims.AAL)
	testutil.Equal(t, 2, len(claims.AMR))
	testutil.Equal(t, "password", claims.AMR[0])
	testutil.Equal(t, "totp", claims.AMR[1])
}

func TestTOTPAPI_ReplayRejection(t *testing.T) {
	srv, authSvc, _ := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "totp-api-replay@example.com")
	secret := enrollTOTPViaAPI(t, srv, authSvc, token)

	// Login to get MFA pending token.
	pendingToken := loginAndGetPendingToken(t, srv, "totp-api-replay@example.com")

	// Challenge and verify.
	w := doJSON(t, srv, "POST", "/api/auth/mfa/totp/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))

	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)

	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         code,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Second login — get a new pending token.
	pendingToken2 := loginAndGetPendingToken(t, srv, "totp-api-replay@example.com")

	// New challenge, but same TOTP code (same time step) — should be rejected as replay.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/challenge", nil, pendingToken2)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp2 struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp2))

	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/verify", map[string]string{
		"challenge_id": chalResp2.ChallengeID,
		"code":         code, // same code — replay
	}, pendingToken2)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "already used")
}

func TestTOTPAPI_ChallengeAlreadyVerified(t *testing.T) {
	srv, authSvc, _ := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "totp-api-used@example.com")
	secret := enrollTOTPViaAPI(t, srv, authSvc, token)

	pendingToken := loginAndGetPendingToken(t, srv, "totp-api-used@example.com")

	// Challenge.
	w := doJSON(t, srv, "POST", "/api/auth/mfa/totp/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))

	// Verify — success.
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         code,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Re-use same challenge — should fail with 409 Conflict.
	step2 := time.Now().Unix()/auth.TOTPPeriodForTest + 1
	code2 := auth.GenerateTOTPCodeForTest(secret, step2)
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         code2,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusConflict, w.Code)
	testutil.Contains(t, w.Body.String(), "already verified")
}

func TestTOTPAPI_WrongCode(t *testing.T) {
	srv, authSvc, _ := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "totp-api-wrong@example.com")
	enrollTOTPViaAPI(t, srv, authSvc, token)

	pendingToken := loginAndGetPendingToken(t, srv, "totp-api-wrong@example.com")

	w := doJSON(t, srv, "POST", "/api/auth/mfa/totp/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))

	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         "000000",
	}, pendingToken)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid TOTP code")
}

func TestTOTPAPI_EnrollAlreadyEnrolled(t *testing.T) {
	srv, authSvc, _ := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "totp-api-dupe@example.com")
	secret := enrollTOTPViaAPI(t, srv, authSvc, token)

	// After enrollment, user has an existing MFA factor. AAL2 is required
	// before the duplicate-enrollment check runs. AAL1 token → 403 (need AAL2).
	w := doJSON(t, srv, "POST", "/api/auth/mfa/totp/enroll", nil, token)
	testutil.StatusCode(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "AAL2")

	// Get an AAL2 token and re-try — now should get 409 Conflict.
	pendingToken := loginAndGetPendingToken(t, srv, "totp-api-dupe@example.com")
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         code,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	aal2Token := parseAuthResp(t, w).Token

	// With AAL2 token, should get 409 for duplicate enrollment.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/enroll", nil, aal2Token)
	testutil.StatusCode(t, http.StatusConflict, w.Code)
	testutil.Contains(t, w.Body.String(), "already enrolled")
}

func TestTOTPAPI_Disabled(t *testing.T) {
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	// TOTPEnabled defaults to false.

	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
	token := registerAndGetToken(t, srv, "totp-disabled@example.com")

	for _, ep := range []string{
		"/api/auth/mfa/totp/enroll",
		"/api/auth/mfa/totp/enroll/confirm",
		"/api/auth/mfa/totp/challenge",
		"/api/auth/mfa/totp/verify",
	} {
		w := doJSON(t, srv, "POST", ep, map[string]string{}, token)
		testutil.StatusCode(t, http.StatusNotFound, w.Code)
	}
}

// =======================================================================
// Backup Code API Tests (HTTP-level)
// =======================================================================

func TestBackupCodeAPI_GenerateVerifyReuse(t *testing.T) {
	srv, authSvc, _ := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "backup-api@example.com")

	// Enroll TOTP first (need MFA to get AAL2 for backup generation).
	secret := enrollTOTPViaAPI(t, srv, authSvc, token)

	// Login -> MFA -> get AAL2 token.
	pendingToken := loginAndGetPendingToken(t, srv, "backup-api@example.com")
	w := doJSON(t, srv, "POST", "/api/auth/mfa/totp/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))

	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         code,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	aal2Resp := parseAuthResp(t, w)
	aal2Token := aal2Resp.Token

	// Generate backup codes (requires AAL2).
	w = doJSON(t, srv, "POST", "/api/auth/mfa/backup/generate", nil, aal2Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var genResp struct {
		Codes []string `json:"codes"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &genResp))
	testutil.Equal(t, 10, len(genResp.Codes))

	// Check count.
	w = doJSON(t, srv, "GET", "/api/auth/mfa/backup/count", nil, aal2Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var countResp struct {
		Remaining int `json:"remaining"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &countResp))
	testutil.Equal(t, 10, countResp.Remaining)

	// Login again to get fresh pending token for backup verify.
	pendingToken2 := loginAndGetPendingToken(t, srv, "backup-api@example.com")

	// Verify with backup code.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/backup/verify", map[string]string{
		"code": genResp.Codes[0],
	}, pendingToken2)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	backupResp := parseAuthResp(t, w)
	testutil.True(t, backupResp.Token != "", "should return access token")

	claims, err := authSvc.ValidateToken(backupResp.Token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal2", claims.AAL)
	testutil.Equal(t, "backup_code", claims.AMR[1])

	// Reuse same backup code — should fail.
	pendingToken3 := loginAndGetPendingToken(t, srv, "backup-api@example.com")
	w = doJSON(t, srv, "POST", "/api/auth/mfa/backup/verify", map[string]string{
		"code": genResp.Codes[0],
	}, pendingToken3)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid or already used")
}

func TestBackupCodeAPI_GenerateRequiresAAL2(t *testing.T) {
	srv, _, _ := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "backup-api-aal1@example.com")

	// AAL1 token → should be rejected.
	w := doJSON(t, srv, "POST", "/api/auth/mfa/backup/generate", nil, token)
	testutil.StatusCode(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "insufficient_aal")
}

func TestBackupCodeAPI_RegenerateInvalidatesOld(t *testing.T) {
	srv, authSvc, _ := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "backup-api-regen@example.com")
	secret := enrollTOTPViaAPI(t, srv, authSvc, token)

	// Get AAL2 token.
	pendingToken := loginAndGetPendingToken(t, srv, "backup-api-regen@example.com")
	w := doJSON(t, srv, "POST", "/api/auth/mfa/totp/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         code,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	aal2Token := parseAuthResp(t, w).Token

	// Generate first set.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/backup/generate", nil, aal2Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var firstCodes struct {
		Codes []string `json:"codes"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &firstCodes))

	// Regenerate.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/backup/regenerate", nil, aal2Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var newCodes struct {
		Codes []string `json:"codes"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &newCodes))
	testutil.Equal(t, 10, len(newCodes.Codes))

	// Old code should no longer work.
	pendingToken2 := loginAndGetPendingToken(t, srv, "backup-api-regen@example.com")
	w = doJSON(t, srv, "POST", "/api/auth/mfa/backup/verify", map[string]string{
		"code": firstCodes.Codes[0],
	}, pendingToken2)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)

	// New code should work.
	pendingToken3 := loginAndGetPendingToken(t, srv, "backup-api-regen@example.com")
	w = doJSON(t, srv, "POST", "/api/auth/mfa/backup/verify", map[string]string{
		"code": newCodes.Codes[0],
	}, pendingToken3)
	testutil.StatusCode(t, http.StatusOK, w.Code)
}

// =======================================================================
// Email MFA API Tests (HTTP-level)
// =======================================================================

func TestEmailMFAAPI_EnrollConfirmChallengeVerify(t *testing.T) {
	srv, authSvc, capture := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "emailmfa-api@example.com")

	// Enroll email MFA.
	w := doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll", nil, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	enrollCode := capture.LastCode()
	testutil.True(t, enrollCode != "", "should send enrollment code via email")

	// Confirm enrollment.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll/confirm", map[string]string{
		"code": enrollCode,
	}, token)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	// Login to get MFA pending token.
	pendingToken := loginAndGetPendingToken(t, srv, "emailmfa-api@example.com")

	// Challenge.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/email/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))
	testutil.True(t, chalResp.ChallengeID != "", "should return challenge ID")

	verifyCode := capture.LastCode()
	testutil.True(t, verifyCode != "", "should send challenge code via email")

	// Verify.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/email/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         verifyCode,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)

	resp := parseAuthResp(t, w)
	testutil.True(t, resp.Token != "", "should return access token")

	claims, err := authSvc.ValidateToken(resp.Token)
	testutil.NoError(t, err)
	testutil.Equal(t, "aal2", claims.AAL)
	testutil.Equal(t, "email_otp", claims.AMR[1])
}

func TestEmailMFAAPI_WrongCode(t *testing.T) {
	srv, _, capture := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "emailmfa-api-wrong@example.com")

	// Enroll + confirm.
	doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll", nil, token)
	enrollCode := capture.LastCode()
	doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll/confirm", map[string]string{
		"code": enrollCode,
	}, token)

	// Login -> challenge.
	pendingToken := loginAndGetPendingToken(t, srv, "emailmfa-api-wrong@example.com")
	w := doJSON(t, srv, "POST", "/api/auth/mfa/email/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))

	// Verify with wrong code.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/email/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         "000000",
	}, pendingToken)
	testutil.StatusCode(t, http.StatusUnauthorized, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid email MFA code")
}

func TestEmailMFAAPI_ChallengeRateLimit(t *testing.T) {
	srv, _, capture := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "emailmfa-api-ratelimit@example.com")

	// Enroll + confirm. Enrollment creates 1 challenge (the enrollment code).
	doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll", nil, token)
	enrollCode := capture.LastCode()
	doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll/confirm", map[string]string{
		"code": enrollCode,
	}, token)

	pendingToken := loginAndGetPendingToken(t, srv, "emailmfa-api-ratelimit@example.com")

	// Enrollment already consumed 1 of the 3 challenges in the rate-limit window.
	// Issue 2 more challenges (reaching the max of 3).
	for i := 0; i < 2; i++ {
		w := doJSON(t, srv, "POST", "/api/auth/mfa/email/challenge", nil, pendingToken)
		testutil.StatusCode(t, http.StatusOK, w.Code)
	}

	// Next challenge should be rate-limited.
	w := doJSON(t, srv, "POST", "/api/auth/mfa/email/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusTooManyRequests, w.Code)
	testutil.Contains(t, w.Body.String(), "too many email challenges")
}

func TestEmailMFAAPI_Disabled(t *testing.T) {
	ctx := t.Context()
	resetAndMigrate(t, ctx)

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = testJWTSecret
	// EmailMFAEnabled defaults to false.

	authSvc := newAuthService()
	srv := server.New(cfg, logger, ch, sharedPG.Pool, authSvc, nil)
	token := registerAndGetToken(t, srv, "emailmfa-disabled@example.com")

	for _, ep := range []string{
		"/api/auth/mfa/email/enroll",
		"/api/auth/mfa/email/enroll/confirm",
		"/api/auth/mfa/email/challenge",
		"/api/auth/mfa/email/verify",
	} {
		w := doJSON(t, srv, "POST", ep, map[string]string{}, token)
		testutil.StatusCode(t, http.StatusNotFound, w.Code)
	}
}

// =======================================================================
// AAL Enforcement API Tests
// =======================================================================

func TestAALEnforcement_ProtectedRouteRejectsAAL1(t *testing.T) {
	srv, authSvc, _ := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "aal-enforce@example.com")
	secret := enrollTOTPViaAPI(t, srv, authSvc, token)

	// AAL1 token should be rejected on backup/generate (AAL2-protected).
	w := doJSON(t, srv, "POST", "/api/auth/mfa/backup/generate", nil, token)
	testutil.StatusCode(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "insufficient_aal")

	// AAL1 token should be rejected on backup/regenerate (AAL2-protected).
	w = doJSON(t, srv, "POST", "/api/auth/mfa/backup/regenerate", nil, token)
	testutil.StatusCode(t, http.StatusForbidden, w.Code)
	testutil.Contains(t, w.Body.String(), "insufficient_aal")

	// Get AAL2 token.
	pendingToken := loginAndGetPendingToken(t, srv, "aal-enforce@example.com")
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))
	step := time.Now().Unix() / auth.TOTPPeriodForTest
	code := auth.GenerateTOTPCodeForTest(secret, step)
	w = doJSON(t, srv, "POST", "/api/auth/mfa/totp/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         code,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	aal2Token := parseAuthResp(t, w).Token

	// AAL2 token should be accepted on backup/generate.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/backup/generate", nil, aal2Token)
	testutil.StatusCode(t, http.StatusOK, w.Code)
}

// =======================================================================
// Email MFA Code Expiry Test
// =======================================================================

func TestEmailMFA_CodeExpiry_EnrollConfirmExpired(t *testing.T) {
	svc, capture := setupEmailMFAServiceWithCapture(t)
	ctx := t.Context()

	userID := registerNamedUser(t, svc, "emailmfa-expiry@example.com")

	// Enroll email MFA.
	err := svc.EnrollEmailMFA(ctx, userID, "emailmfa-expiry@example.com")
	testutil.NoError(t, err)

	code := capture.LastCode()
	testutil.True(t, code != "", "should capture enrollment code")

	// Expire the challenge by setting expires_at to the past.
	_, err = svc.DB().Exec(ctx,
		`UPDATE _ayb_mfa_challenges SET expires_at = NOW() - INTERVAL '1 hour'
		 WHERE factor_id IN (SELECT id FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'email')`,
		userID,
	)
	testutil.NoError(t, err)

	// Confirm with valid code should fail with expired error.
	err = svc.ConfirmEmailMFAEnrollment(ctx, userID, code)
	testutil.True(t, errors.Is(err, auth.ErrEmailMFAExpired), "expired code should return ErrEmailMFAExpired")
}

func TestEmailMFA_CodeExpiry_VerifyChallengeExpired(t *testing.T) {
	svc, capture := setupEmailMFAServiceWithCapture(t)
	ctx := t.Context()

	email := "emailmfa-verify-expiry@example.com"
	userID := registerNamedUser(t, svc, email)

	// Enroll + confirm factor.
	err := svc.EnrollEmailMFA(ctx, userID, email)
	testutil.NoError(t, err)
	enrollCode := capture.LastCode()
	err = svc.ConfirmEmailMFAEnrollment(ctx, userID, enrollCode)
	testutil.NoError(t, err)

	// Create verification challenge.
	challengeID, err := svc.ChallengeEmailMFA(ctx, userID, email)
	testutil.NoError(t, err)
	verifyCode := capture.LastCode()

	// Expire the challenge.
	_, err = svc.DB().Exec(ctx,
		`UPDATE _ayb_mfa_challenges SET expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1`,
		challengeID,
	)
	testutil.NoError(t, err)

	// Verify should fail with expired error.
	_, _, _, err = svc.VerifyEmailMFA(ctx, userID, challengeID, verifyCode, "password")
	testutil.True(t, errors.Is(err, auth.ErrEmailMFAExpired), "expired challenge should return ErrEmailMFAExpired")
}

func TestEmailMFAAPI_ExpiredChallenge(t *testing.T) {
	srv, _, capture := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "emailmfa-api-expiry@example.com")

	// Enroll + confirm.
	doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll", nil, token)
	enrollCode := capture.LastCode()
	doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll/confirm", map[string]string{
		"code": enrollCode,
	}, token)

	// Login → challenge.
	pendingToken := loginAndGetPendingToken(t, srv, "emailmfa-api-expiry@example.com")
	w := doJSON(t, srv, "POST", "/api/auth/mfa/email/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusOK, w.Code)
	var chalResp struct {
		ChallengeID string `json:"challenge_id"`
	}
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &chalResp))
	verifyCode := capture.LastCode()

	// Expire the challenge directly in DB.
	_, err := sharedPG.Pool.Exec(t.Context(),
		`UPDATE _ayb_mfa_challenges SET expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1`,
		chalResp.ChallengeID,
	)
	testutil.NoError(t, err)

	// Verify should return 410 Gone.
	w = doJSON(t, srv, "POST", "/api/auth/mfa/email/verify", map[string]string{
		"challenge_id": chalResp.ChallengeID,
		"code":         verifyCode,
	}, pendingToken)
	testutil.StatusCode(t, http.StatusGone, w.Code)
	testutil.Contains(t, w.Body.String(), "expired")
}

// =======================================================================
// Wrong Factor Type Error Tests
// =======================================================================

func TestWrongFactorType_TOTPChallengeWithoutEnrollment(t *testing.T) {
	srv, _, capture := setupMFAAPIServer(t)
	token, _ := registerForMFA(t, srv, "wrong-factor@example.com")

	// Enroll email MFA (not TOTP).
	doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll", nil, token)
	enrollCode := capture.LastCode()
	doJSON(t, srv, "POST", "/api/auth/mfa/email/enroll/confirm", map[string]string{
		"code": enrollCode,
	}, token)

	// Login → pending token.
	pendingToken := loginAndGetPendingToken(t, srv, "wrong-factor@example.com")

	// Try TOTP challenge with no TOTP enrolled → should fail.
	w := doJSON(t, srv, "POST", "/api/auth/mfa/totp/challenge", nil, pendingToken)
	testutil.StatusCode(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "no TOTP factor enrolled")
}

// --- ListUsers Integration Tests ---

func TestListUsers_IncludesAnonymousUsersWithNullEmail(t *testing.T) {
	svc := setupAnonymousService(t)
	ctx := t.Context()

	// Create a regular user with email.
	_, _, _, err := svc.Register(ctx, "regular@example.com", "password123!")
	testutil.NoError(t, err)

	// Create an anonymous user (email will be NULL in the DB).
	anonUser, _, _, err := svc.CreateAnonymousUser(ctx)
	testutil.NoError(t, err)
	testutil.True(t, anonUser.IsAnonymous, "user should be anonymous")

	// ListUsers must succeed even when anonymous users with NULL email exist.
	result, err := svc.ListUsers(ctx, 1, 20, "")
	testutil.NoError(t, err)
	testutil.True(t, result.TotalItems == 2, fmt.Sprintf("expected 2 users, got %d", result.TotalItems))
	testutil.True(t, len(result.Items) == 2, fmt.Sprintf("expected 2 items, got %d", len(result.Items)))

	// Verify we can find both users in the results.
	var foundAnon, foundRegular bool
	for _, u := range result.Items {
		if u.ID == anonUser.ID {
			foundAnon = true
			testutil.True(t, u.Email == "", "anonymous user email should be empty string")
		}
		if u.Email == "regular@example.com" {
			foundRegular = true
		}
	}
	testutil.True(t, foundAnon, "anonymous user should appear in list")
	testutil.True(t, foundRegular, "regular user should appear in list")
}
