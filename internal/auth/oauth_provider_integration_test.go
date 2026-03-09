//go:build integration

package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Helper: create app + OAuth client for test ---

func setupOAuthClient(t *testing.T, ctx context.Context, svc *auth.Service, clientType string) (*auth.OAuthClient, string) {
	t.Helper()

	// Create a user to own the app.
	user, _, _, err := svc.Register(ctx, "oauth-test@example.com", "password123")
	testutil.NoError(t, err)

	// Create an app.
	app, err := svc.CreateApp(ctx, "test-app", "Test application", user.ID)
	testutil.NoError(t, err)

	// Register an OAuth client.
	secret, client, err := svc.RegisterOAuthClient(ctx, app.ID, "test-client", clientType,
		[]string{"https://example.com/callback"}, []string{"readonly", "readwrite"})
	testutil.NoError(t, err)

	return client, secret
}

func TestOAuthListClientsIncludesTokenStats(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "stats@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "stats_verifier_for_list_clients"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "state-stats")
	testutil.NoError(t, err)

	authCodeResp, err := svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)

	clientCredsResp, err := svc.ClientCredentialsGrant(ctx, client.ClientID, "readwrite", nil)
	testutil.NoError(t, err)

	// Keep these references used and ensure both grants are valid before list aggregation.
	_, err = svc.ValidateOAuthToken(ctx, authCodeResp.AccessToken)
	testutil.NoError(t, err)
	_, err = svc.ValidateOAuthToken(ctx, clientCredsResp.AccessToken)
	testutil.NoError(t, err)

	list, err := svc.ListOAuthClients(ctx, 1, 20)
	testutil.NoError(t, err)

	var got *auth.OAuthClient
	for i := range list.Items {
		if list.Items[i].ClientID == client.ClientID {
			got = &list.Items[i]
			break
		}
	}
	testutil.NotNil(t, got)
	testutil.Equal(t, 2, got.ActiveAccessTokenCount)
	testutil.Equal(t, 1, got.ActiveRefreshTokenCount)
	testutil.Equal(t, 2, got.TotalGrants)
	testutil.NotNil(t, got.LastTokenIssuedAt)
}

// --- Authorization Code Flow (end-to-end) ---

func TestOAuthAuthCodeFlowE2E(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, clientSecret := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)

	// Register a user to authorize.
	user, _, _, err := svc.Register(ctx, "enduser@example.com", "password456")
	testutil.NoError(t, err)

	// Generate PKCE verifier + challenge.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := auth.GeneratePKCEChallenge(verifier)

	// Create authorization code.
	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "random-state-123")
	testutil.NoError(t, err)
	testutil.True(t, len(code) == 64, "code should be 64 hex chars (32 bytes)")

	// Validate client credentials.
	_, err = svc.ValidateOAuthClientCredentials(ctx, client.ClientID, clientSecret)
	testutil.NoError(t, err)

	// Exchange code for tokens.
	resp, err := svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)
	testutil.Equal(t, "Bearer", resp.TokenType)
	testutil.Equal(t, 3600, resp.ExpiresIn)
	testutil.Equal(t, "readonly", resp.Scope)
	testutil.True(t, auth.IsOAuthAccessToken(resp.AccessToken), "should be access token")
	testutil.True(t, auth.IsOAuthRefreshToken(resp.RefreshToken), "should be refresh token")

	// Validate the access token.
	info, err := svc.ValidateOAuthToken(ctx, resp.AccessToken)
	testutil.NoError(t, err)
	testutil.True(t, info.UserID != nil, "should have user_id")
	testutil.Equal(t, user.ID, *info.UserID)
	testutil.Equal(t, client.ClientID, info.ClientID)
	testutil.Equal(t, "readonly", info.Scope)
}

// --- Code Replay Rejection ---

func TestOAuthCodeReplayRejected(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "replay@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "test_verifier_for_replay_detection"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "state-replay")
	testutil.NoError(t, err)

	// First exchange succeeds.
	_, err = svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)

	// Second exchange with same code fails.
	_, err = svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.True(t, err != nil, "replay should fail")
	oauthErr, ok := err.(*auth.OAuthError)
	testutil.True(t, ok, "expected OAuthError")
	testutil.Equal(t, auth.OAuthErrInvalidGrant, oauthErr.Code)
}

// --- PKCE Verification ---

func TestOAuthPKCES256Verification(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "pkce@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "correct_verifier_string_for_pkce"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "state-pkce")
	testutil.NoError(t, err)

	// Wrong verifier should fail.
	_, err = svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", "wrong_verifier")
	testutil.True(t, err != nil, "wrong verifier should fail")
	oauthErr, ok := err.(*auth.OAuthError)
	testutil.True(t, ok, "expected OAuthError")
	testutil.Equal(t, auth.OAuthErrInvalidGrant, oauthErr.Code)
	testutil.True(t, oauthErr.Description == "PKCE verification failed",
		"expected PKCE error, got: %s", oauthErr.Description)
}

func TestOAuthPKCEFailureDoesNotConsumeAuthorizationCode(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "pkce-retry@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "pkce_retry_verifier"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "state-pkce-retry")
	testutil.NoError(t, err)

	// First exchange attempt fails due to wrong verifier.
	_, err = svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", "wrong_verifier")
	testutil.True(t, err != nil, "wrong verifier should fail")

	// Correct verifier should still work after failed PKCE attempt.
	resp, err := svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)
	testutil.True(t, auth.IsOAuthAccessToken(resp.AccessToken), "should return access token")
}

// --- Client Credentials Grant ---

func TestOAuthClientCredentialsGrant(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)

	resp, err := svc.ClientCredentialsGrant(ctx, client.ClientID, "readwrite", nil)
	testutil.NoError(t, err)
	testutil.Equal(t, "Bearer", resp.TokenType)
	testutil.Equal(t, "readwrite", resp.Scope)
	testutil.True(t, auth.IsOAuthAccessToken(resp.AccessToken), "should be access token")
	testutil.Equal(t, "", resp.RefreshToken) // no refresh token for client_credentials

	// Validate the access token.
	info, err := svc.ValidateOAuthToken(ctx, resp.AccessToken)
	testutil.NoError(t, err)
	testutil.True(t, info.UserID == nil, "client_credentials should have no user_id")
	testutil.Equal(t, client.ClientID, info.ClientID)
	testutil.Equal(t, "readwrite", info.Scope)
}

// --- Refresh Token Rotation ---

func TestOAuthRefreshTokenRotation(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "refresh@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "refresh_verifier_string"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "state-refresh")
	testutil.NoError(t, err)

	resp1, err := svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)

	// Refresh should return new token pair.
	resp2, err := svc.RefreshOAuthToken(ctx, resp1.RefreshToken, client.ClientID)
	testutil.NoError(t, err)
	testutil.True(t, resp2.AccessToken != resp1.AccessToken, "should get new access token")
	testutil.True(t, resp2.RefreshToken != resp1.RefreshToken, "should get new refresh token")
	testutil.Equal(t, "readonly", resp2.Scope)

	// Old refresh token should now fail (rotated out).
	_, err = svc.RefreshOAuthToken(ctx, resp1.RefreshToken, client.ClientID)
	testutil.True(t, err != nil, "old refresh token should be rejected")
}

// --- Refresh Token Reuse Detection ---

func TestOAuthRefreshTokenReuseDetection(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "reuse@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "reuse_verifier_string"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "state-reuse")
	testutil.NoError(t, err)

	resp1, err := svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)

	// First refresh succeeds (rotates to resp2).
	resp2, err := svc.RefreshOAuthToken(ctx, resp1.RefreshToken, client.ClientID)
	testutil.NoError(t, err)

	// Reuse old refresh token (resp1) — this is a compromise indicator.
	_, err = svc.RefreshOAuthToken(ctx, resp1.RefreshToken, client.ClientID)
	testutil.True(t, err != nil, "reuse should fail")
	oauthErr, ok := err.(*auth.OAuthError)
	testutil.True(t, ok, "expected OAuthError")
	testutil.Equal(t, auth.OAuthErrInvalidGrant, oauthErr.Code)

	// The new tokens (resp2) should also be revoked due to reuse detection.
	_, err = svc.ValidateOAuthToken(ctx, resp2.AccessToken)
	testutil.True(t, err != nil, "resp2 access token should be revoked after reuse detection")

	_, err = svc.RefreshOAuthToken(ctx, resp2.RefreshToken, client.ClientID)
	testutil.True(t, err != nil, "resp2 refresh token should be revoked after reuse detection")
}

// --- Token Revocation ---

func TestOAuthTokenRevocation(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)

	resp, err := svc.ClientCredentialsGrant(ctx, client.ClientID, "readonly", nil)
	testutil.NoError(t, err)

	// Token should be valid before revocation.
	_, err = svc.ValidateOAuthToken(ctx, resp.AccessToken)
	testutil.NoError(t, err)

	// Revoke access token.
	err = svc.RevokeOAuthToken(ctx, resp.AccessToken)
	testutil.NoError(t, err)

	// Token should be invalid after revocation.
	_, err = svc.ValidateOAuthToken(ctx, resp.AccessToken)
	testutil.True(t, err != nil, "revoked token should fail validation")
}

func TestOAuthRefreshTokenRevocationCascades(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "cascade@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "cascade_verifier"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "state-cascade")
	testutil.NoError(t, err)

	resp, err := svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)

	// Revoking refresh token should cascade to access token.
	err = svc.RevokeOAuthToken(ctx, resp.RefreshToken)
	testutil.NoError(t, err)

	_, err = svc.ValidateOAuthToken(ctx, resp.AccessToken)
	testutil.True(t, err != nil, "access token should be revoked after refresh revocation")
}

func TestOAuthRevocationUnknownToken(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	// Per RFC 7009: unknown token revocation should return success.
	err := svc.RevokeOAuthToken(ctx, "ayb_at_nonexistent_token_here")
	testutil.NoError(t, err)
}

// --- Consent ---

func TestOAuthConsent(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "consent@example.com", "password456")
	testutil.NoError(t, err)

	// No consent initially.
	has, err := svc.HasConsent(ctx, user.ID, client.ClientID, "readonly", nil)
	testutil.NoError(t, err)
	testutil.False(t, has, "should have no consent initially")

	// Save consent.
	err = svc.SaveConsent(ctx, user.ID, client.ClientID, "readonly", nil)
	testutil.NoError(t, err)

	// Now consent exists.
	has, err = svc.HasConsent(ctx, user.ID, client.ClientID, "readonly", nil)
	testutil.NoError(t, err)
	testutil.True(t, has, "should have consent after saving")

	// Different scope — no consent.
	has, err = svc.HasConsent(ctx, user.ID, client.ClientID, "readwrite", nil)
	testutil.NoError(t, err)
	testutil.False(t, has, "different scope should not match")

	// Upsert consent to new scope.
	err = svc.SaveConsent(ctx, user.ID, client.ClientID, "readwrite", nil)
	testutil.NoError(t, err)

	has, err = svc.HasConsent(ctx, user.ID, client.ClientID, "readwrite", nil)
	testutil.NoError(t, err)
	testutil.True(t, has, "upserted scope should match")
}

func TestOAuthConsentAllowedTablesSubset(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "consent-tables@example.com", "password456")
	testutil.NoError(t, err)

	// Save consent restricted to one table.
	err = svc.SaveConsent(ctx, user.ID, client.ClientID, "readonly", []string{"users"})
	testutil.NoError(t, err)

	// Exact allowed table request should match.
	has, err := svc.HasConsent(ctx, user.ID, client.ClientID, "readonly", []string{"users"})
	testutil.NoError(t, err)
	testutil.True(t, has, "exact allowed_tables should match")

	// Expanded table request must not match prior consent.
	has, err = svc.HasConsent(ctx, user.ID, client.ClientID, "readonly", []string{"users", "profiles"})
	testutil.NoError(t, err)
	testutil.False(t, has, "expanded allowed_tables must require re-consent")

	// Unrestricted request must not match previously restricted consent.
	has, err = svc.HasConsent(ctx, user.ID, client.ClientID, "readonly", nil)
	testutil.NoError(t, err)
	testutil.False(t, has, "unrestricted request must require re-consent when consent was restricted")
}

// --- Redirect URI Mismatch ---

func TestOAuthRedirectURIMismatch(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "redirect@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "redirect_verifier"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "state-redir")
	testutil.NoError(t, err)

	// Exchange with different redirect_uri.
	_, err = svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://evil.com/callback", verifier)
	testutil.True(t, err != nil, "mismatched redirect_uri should fail")
	oauthErr, ok := err.(*auth.OAuthError)
	testutil.True(t, ok, "expected OAuthError")
	testutil.Equal(t, auth.OAuthErrInvalidGrant, oauthErr.Code)
}

// --- Client ID Mismatch ---

func TestOAuthClientIDMismatch(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "clientmismatch@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "mismatch_verifier"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "state-mismatch")
	testutil.NoError(t, err)

	// Exchange with different client_id.
	_, err = svc.ExchangeAuthorizationCode(ctx, code, "ayb_cid_different_client",
		"https://example.com/callback", verifier)
	testutil.True(t, err != nil, "mismatched client_id should fail")
}

// --- Allowed Tables ---

func TestOAuthTokenWithAllowedTables(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "tables@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "tables_verifier"
	challenge := auth.GeneratePKCEChallenge(verifier)
	tables := []string{"users", "posts"}

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", tables, challenge, "S256", "state-tables")
	testutil.NoError(t, err)

	resp, err := svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)

	info, err := svc.ValidateOAuthToken(ctx, resp.AccessToken)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(info.AllowedTables))
	testutil.Equal(t, "users", info.AllowedTables[0])
	testutil.Equal(t, "posts", info.AllowedTables[1])
}

// --- Expired Token Rejection ---

func TestOAuthExpiredTokenRejected(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	// Set very short token duration.
	svc.SetOAuthProviderModeConfig(auth.OAuthProviderModeConfig{
		AccessTokenDuration: 1 * time.Millisecond,
	})

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)

	resp, err := svc.ClientCredentialsGrant(ctx, client.ClientID, "readonly", nil)
	testutil.NoError(t, err)

	// Wait for token to expire.
	time.Sleep(10 * time.Millisecond)

	_, err = svc.ValidateOAuthToken(ctx, resp.AccessToken)
	testutil.True(t, err != nil, "expired token should fail validation")
}

// --- Refresh Token Client Mismatch ---

func TestOAuthRefreshTokenClientMismatch(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "clientmismatch2@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "clientmismatch_verifier"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", nil, challenge, "S256", "state-cm")
	testutil.NoError(t, err)

	resp, err := svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)

	// Try refreshing with different client_id.
	_, err = svc.RefreshOAuthToken(ctx, resp.RefreshToken, "ayb_cid_other_client")
	testutil.True(t, err != nil, "refresh with wrong client should fail")
}

// --- Middleware: OAuth access token accepted by RequireAuth ---

func TestOAuthTokenMiddlewareAcceptsValidToken(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "mw-valid@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "mw_valid_verifier"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readonly", []string{"posts"}, challenge, "S256", "state-mw")
	testutil.NoError(t, err)

	resp, err := svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)

	var gotClaims *auth.Claims
	handler := auth.RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = auth.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.NotNil(t, gotClaims)
	testutil.Equal(t, user.ID, gotClaims.Subject)
	testutil.Equal(t, "readonly", gotClaims.APIKeyScope)
	testutil.Equal(t, 1, len(gotClaims.AllowedTables))
	testutil.Equal(t, "posts", gotClaims.AllowedTables[0])
	testutil.True(t, gotClaims.AppID != "", "should have app_id from OAuth client")
}

func TestOAuthTokenMiddlewareRejectsRevokedToken(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)

	resp, err := svc.ClientCredentialsGrant(ctx, client.ClientID, "readonly", nil)
	testutil.NoError(t, err)

	err = svc.RevokeOAuthToken(ctx, resp.AccessToken)
	testutil.NoError(t, err)

	handler := auth.RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOAuthTokenMiddlewareReadonlyDeniesWrite(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)

	resp, err := svc.ClientCredentialsGrant(ctx, client.ClientID, "readonly", nil)
	testutil.NoError(t, err)

	var gotClaims *auth.Claims
	handler := auth.RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = auth.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.NotNil(t, gotClaims)
	testutil.Equal(t, "readonly", gotClaims.APIKeyScope)
	testutil.False(t, gotClaims.IsWriteAllowed(), "readonly scope should deny writes")
	testutil.True(t, gotClaims.IsReadAllowed(), "readonly scope should allow reads")
}

func TestOAuthTokenMiddlewareClientCredentialsNoUser(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)

	resp, err := svc.ClientCredentialsGrant(ctx, client.ClientID, "readwrite", nil)
	testutil.NoError(t, err)

	var gotClaims *auth.Claims
	handler := auth.RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = auth.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.NotNil(t, gotClaims)
	// Client credentials grants have no user context.
	testutil.Equal(t, "", gotClaims.Subject)
	testutil.Equal(t, "readwrite", gotClaims.APIKeyScope)
}

func TestOAuthTokenMiddlewareCoexistsWithJWT(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	// Test JWT token still works.
	user, jwtToken, _, err := svc.Register(ctx, "jwt-coexist@example.com", "password456")
	testutil.NoError(t, err)

	var gotClaims *auth.Claims
	handler := auth.RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = auth.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.NotNil(t, gotClaims)
	testutil.Equal(t, user.ID, gotClaims.Subject)
	testutil.Equal(t, "", gotClaims.APIKeyScope) // JWT tokens have no API key scope
}

// --- App Rate Limit Propagation ---

func TestOAuthTokenCarriesAppRateLimits(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	// Create user + app.
	user, _, _, err := svc.Register(ctx, "ratelimit@example.com", "password456")
	testutil.NoError(t, err)

	app, err := svc.CreateApp(ctx, "rate-limited-app", "App with rate limits", user.ID)
	testutil.NoError(t, err)

	// Set rate limits on the app.
	app, err = svc.UpdateApp(ctx, app.ID, app.Name, app.Description, 50, 60)
	testutil.NoError(t, err)
	testutil.Equal(t, 50, app.RateLimitRPS)
	testutil.Equal(t, 60, app.RateLimitWindowSeconds)

	// Register OAuth client on that app.
	_, client, err := svc.RegisterOAuthClient(ctx, app.ID, "rate-limited-client",
		auth.OAuthClientTypeConfidential, []string{"https://example.com/callback"}, []string{"readonly"})
	testutil.NoError(t, err)

	// Issue a token via client_credentials.
	resp, err := svc.ClientCredentialsGrant(ctx, client.ClientID, "readonly", nil)
	testutil.NoError(t, err)

	// Validate the token — should carry rate limit info.
	info, err := svc.ValidateOAuthToken(ctx, resp.AccessToken)
	testutil.NoError(t, err)
	testutil.Equal(t, app.ID, info.AppID)
	testutil.Equal(t, 50, info.AppRateLimitRPS)
	testutil.Equal(t, 60, info.AppRateLimitWindowSeconds)
}

func TestOAuthTokenMiddlewareCarriesAppRateLimits(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	// Create user + app with rate limits.
	user, _, _, err := svc.Register(ctx, "ratelimit-mw@example.com", "password456")
	testutil.NoError(t, err)

	app, err := svc.CreateApp(ctx, "rate-limited-app-mw", "App with rate limits", user.ID)
	testutil.NoError(t, err)

	app, err = svc.UpdateApp(ctx, app.ID, app.Name, app.Description, 25, 120)
	testutil.NoError(t, err)

	// Register OAuth client.
	_, client, err := svc.RegisterOAuthClient(ctx, app.ID, "rl-mw-client",
		auth.OAuthClientTypeConfidential, []string{"https://example.com/callback"}, []string{"readwrite"})
	testutil.NoError(t, err)

	// Issue token.
	resp, err := svc.ClientCredentialsGrant(ctx, client.ClientID, "readwrite", nil)
	testutil.NoError(t, err)

	// Use middleware to extract claims.
	var gotClaims *auth.Claims
	handler := auth.RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = auth.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.NotNil(t, gotClaims)
	testutil.Equal(t, app.ID, gotClaims.AppID)
	testutil.Equal(t, 25, gotClaims.AppRateLimitRPS)
	testutil.Equal(t, 120, gotClaims.AppRateLimitWindow)
}

func TestOAuthTokenMiddlewareAllowedTableEnforcement(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	svc := newAuthService()

	client, _ := setupOAuthClient(t, ctx, svc, auth.OAuthClientTypeConfidential)
	user, _, _, err := svc.Register(ctx, "tables-mw@example.com", "password456")
	testutil.NoError(t, err)

	verifier := "tables_mw_verifier"
	challenge := auth.GeneratePKCEChallenge(verifier)

	code, err := svc.CreateAuthorizationCode(ctx, client.ClientID, user.ID,
		"https://example.com/callback", "readwrite", []string{"posts", "comments"}, challenge, "S256", "state-tbls")
	testutil.NoError(t, err)

	resp, err := svc.ExchangeAuthorizationCode(ctx, code, client.ClientID,
		"https://example.com/callback", verifier)
	testutil.NoError(t, err)

	var gotClaims *auth.Claims
	handler := auth.RequireAuth(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = auth.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.NotNil(t, gotClaims)
	testutil.True(t, gotClaims.IsTableAllowed("posts"), "posts should be allowed")
	testutil.True(t, gotClaims.IsTableAllowed("comments"), "comments should be allowed")
	testutil.False(t, gotClaims.IsTableAllowed("users"), "users should not be allowed")
}
