//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/testutil"
)

type testProviderTokenVault struct {
	nonce []byte
}

func (v *testProviderTokenVault) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	cipher := append([]byte("enc:"), plaintext...)
	return cipher, append([]byte(nil), v.nonce...), nil
}

func (v *testProviderTokenVault) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	if !bytes.Equal(v.nonce, nonce) {
		return nil, auth.ErrValidation
	}
	return bytes.TrimPrefix(ciphertext, []byte("enc:")), nil
}

func TestProviderTokenGetProviderTokenAutoRefresh(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	vault := &testProviderTokenVault{nonce: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}}
	store := auth.NewProviderTokenStore(sharedPG.Pool, vault, testutil.DiscardLogger())

	called := false
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("unexpected grant_type: %s", got)
		}
		if got := r.Form.Get("refresh_token"); got != "old-refresh" {
			t.Fatalf("unexpected refresh_token: %s", got)
		}
		resp := map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"token_type":    "bearer",
			"scope":         "read write",
			"expires_in":    1200,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer tokenServer.Close()

	store.SetOAuthResolver(func(provider string) (auth.OAuthClientConfig, auth.OAuthProviderConfig, bool) {
		if provider != "google" {
			return auth.OAuthClientConfig{}, auth.OAuthProviderConfig{}, false
		}
		return auth.OAuthClientConfig{ClientID: "client-id", ClientSecret: "client-secret"}, auth.OAuthProviderConfig{TokenURL: tokenServer.URL}, true
	})

	var userID string
	err := sharedPG.Pool.QueryRow(ctx, "INSERT INTO _ayb_users (email, password_hash) VALUES ('refresh-success@example.com', 'hash') RETURNING id").Scan(&userID)
	testutil.NoError(t, err)

	expiredAt := time.Now().Add(-time.Hour)
	err = store.StoreTokens(ctx, userID, "google", "stale-access", "old-refresh", "bearer", "read", &expiredAt)
	testutil.NoError(t, err)

	token, err := store.GetProviderToken(ctx, userID, "google")
	testutil.NoError(t, err)
	testutil.Equal(t, "new-access", token)
	testutil.True(t, called)

	var accessEnc, refreshEnc []byte
	var tokenType, scopes string
	var refreshFailureCount int
	var lastRefreshErr string
	var updatedExpiry time.Time
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT access_token_enc, refresh_token_enc, COALESCE(token_type, ''), COALESCE(scopes, ''), COALESCE(refresh_failure_count, 0), COALESCE(last_refresh_error, ''), expires_at
		 FROM _ayb_oauth_provider_tokens
		 WHERE user_id = $1 AND provider = $2`,
		userID, "google").Scan(&accessEnc, &refreshEnc, &tokenType, &scopes, &refreshFailureCount, &lastRefreshErr, &updatedExpiry)
	testutil.NoError(t, err)
	testutil.Equal(t, "bearer", tokenType)
	testutil.Equal(t, "read write", scopes)
	testutil.Equal(t, 0, refreshFailureCount)
	testutil.Equal(t, "", lastRefreshErr)
	plainAccess, err := vault.Decrypt(accessEnc[len(vault.nonce):], accessEnc[:len(vault.nonce)])
	testutil.NoError(t, err)
	testutil.Equal(t, "new-access", string(plainAccess))
	plainRefresh, err := vault.Decrypt(refreshEnc[len(vault.nonce):], refreshEnc[:len(vault.nonce)])
	testutil.NoError(t, err)
	testutil.Equal(t, "new-refresh", string(plainRefresh))
	testutil.True(t, updatedExpiry.After(time.Now()))
}

func TestProviderTokenGetProviderTokenStaleAfterFiveFailures(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	vault := &testProviderTokenVault{nonce: []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}}
	store := auth.NewProviderTokenStore(sharedPG.Pool, vault, testutil.DiscardLogger())

	calls := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = r.ParseForm()
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer tokenServer.Close()

	store.SetOAuthResolver(func(provider string) (auth.OAuthClientConfig, auth.OAuthProviderConfig, bool) {
		if provider != "google" {
			return auth.OAuthClientConfig{}, auth.OAuthProviderConfig{}, false
		}
		return auth.OAuthClientConfig{ClientID: "client-id", ClientSecret: "client-secret"}, auth.OAuthProviderConfig{TokenURL: tokenServer.URL}, true
	})

	var userID string
	err := sharedPG.Pool.QueryRow(ctx, "INSERT INTO _ayb_users (email, password_hash) VALUES ('refresh-stale@example.com', 'hash') RETURNING id").Scan(&userID)
	testutil.NoError(t, err)

	expiredAt := time.Now().Add(-time.Hour)
	err = store.StoreTokens(ctx, userID, "google", "stale-access", "old-refresh", "bearer", "read", &expiredAt)
	testutil.NoError(t, err)

	for i := 0; i < 4; i++ {
		token, err := store.GetProviderToken(ctx, userID, "google")
		testutil.Equal(t, "", strings.TrimSpace(token))
		testutil.NotNil(t, err)
		testutil.True(t, !errors.Is(err, auth.ErrProviderTokenStale))

		var refreshFailureCount int
		err = sharedPG.Pool.QueryRow(ctx, "SELECT COALESCE(refresh_failure_count, 0) FROM _ayb_oauth_provider_tokens WHERE user_id = $1 AND provider = $2", userID, "google").Scan(&refreshFailureCount)
		testutil.NoError(t, err)
		testutil.Equal(t, i+1, refreshFailureCount)
	}

	token, err := store.GetProviderToken(ctx, userID, "google")
	testutil.Equal(t, "", strings.TrimSpace(token))
	testutil.True(t, errors.Is(err, auth.ErrProviderTokenStale))
	testutil.Equal(t, 5, calls)

	// After stale, token refresh is no longer attempted.
	token, err = store.GetProviderToken(ctx, userID, "google")
	testutil.Equal(t, "", strings.TrimSpace(token))
	testutil.True(t, errors.Is(err, auth.ErrProviderTokenStale))
	testutil.Equal(t, 5, calls)
}

func TestProviderTokenCascadeOnUserDeletion(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	vault := &testProviderTokenVault{nonce: []byte{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4}}
	store := auth.NewProviderTokenStore(sharedPG.Pool, vault, testutil.DiscardLogger())

	// Create user and store provider tokens for two providers.
	var userID string
	err := sharedPG.Pool.QueryRow(ctx,
		"INSERT INTO _ayb_users (email, password_hash) VALUES ('cascade-delete@example.com', 'hash') RETURNING id",
	).Scan(&userID)
	testutil.NoError(t, err)

	exp := time.Now().Add(time.Hour)
	err = store.StoreTokens(ctx, userID, "google", "g-access", "g-refresh", "bearer", "openid", &exp)
	testutil.NoError(t, err)
	err = store.StoreTokens(ctx, userID, "github", "gh-access", "gh-refresh", "bearer", "repo", &exp)
	testutil.NoError(t, err)

	// Verify tokens exist.
	var count int
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_oauth_provider_tokens WHERE user_id = $1", userID,
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, count)

	// Delete the user — FK CASCADE should remove all provider tokens.
	_, err = sharedPG.Pool.Exec(ctx, "DELETE FROM _ayb_users WHERE id = $1", userID)
	testutil.NoError(t, err)

	// Verify provider tokens were cascaded.
	err = sharedPG.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM _ayb_oauth_provider_tokens WHERE user_id = $1", userID,
	).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}

func TestProviderTokenRefreshExpiringWindow(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)

	vault := &testProviderTokenVault{nonce: []byte{3, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}}
	store := auth.NewProviderTokenStore(sharedPG.Pool, vault, testutil.DiscardLogger())

	called := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("unexpected grant_type: %s", got)
		}
		resp := map[string]any{
			"access_token":  "fresh-access",
			"refresh_token": "fresh-refresh",
			"token_type":    "bearer",
			"scope":         "read",
			"expires_in":    1800,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer tokenServer.Close()

	store.SetOAuthResolver(func(provider string) (auth.OAuthClientConfig, auth.OAuthProviderConfig, bool) {
		if provider != "google" && provider != "github" {
			return auth.OAuthClientConfig{}, auth.OAuthProviderConfig{}, false
		}
		return auth.OAuthClientConfig{ClientID: "client-id", ClientSecret: "client-secret"}, auth.OAuthProviderConfig{TokenURL: tokenServer.URL}, true
	})

	var userID string
	err := sharedPG.Pool.QueryRow(ctx, "INSERT INTO _ayb_users (email, password_hash) VALUES ('refresh-window@example.com', 'hash') RETURNING id").Scan(&userID)
	testutil.NoError(t, err)

	expiresSoon := time.Now().Add(8 * time.Minute)
	err = store.StoreTokens(ctx, userID, "google", "soon-access", "old-refresh", "bearer", "read", &expiresSoon)
	testutil.NoError(t, err)

	expiresLater := time.Now().Add(30 * time.Minute)
	err = store.StoreTokens(ctx, userID, "github", "later-access", "later-refresh", "bearer", "read", &expiresLater)
	testutil.NoError(t, err)

	err = store.RefreshExpiringProviderTokens(ctx, 10*time.Minute)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, called)

	var googleAccessEnc []byte
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT access_token_enc FROM _ayb_oauth_provider_tokens WHERE user_id = $1 AND provider = $2`,
		userID, "google").Scan(&googleAccessEnc)
	testutil.NoError(t, err)
	googleAccess, err := vault.Decrypt(googleAccessEnc[len(vault.nonce):], googleAccessEnc[:len(vault.nonce)])
	testutil.NoError(t, err)
	testutil.Equal(t, "fresh-access", string(googleAccess))

	var githubAccessEnc []byte
	err = sharedPG.Pool.QueryRow(ctx,
		`SELECT access_token_enc FROM _ayb_oauth_provider_tokens WHERE user_id = $1 AND provider = $2`,
		userID, "github").Scan(&githubAccessEnc)
	testutil.NoError(t, err)
	githubAccess, err := vault.Decrypt(githubAccessEnc[len(vault.nonce):], githubAccessEnc[:len(vault.nonce)])
	testutil.NoError(t, err)
	testutil.Equal(t, "later-access", string(githubAccess))
}

func TestAPIKeyCreateRejectsUnsupportedScopeFields(t *testing.T) {
	ctx := context.Background()
	srv := setupAuthServer(t, ctx)
	token := registerAndGetToken(t, srv, "apikey-unsupported-scope@example.com")

	tests := []struct {
		name    string
		payload map[string]any
		msg     string
	}{
		{
			name: "app scope field",
			payload: map[string]any{
				"name":  "my-key",
				"appId": "7c254770-bf4c-4cc2-8f56-9f24df29deba",
			},
			msg: "appId is not supported",
		},
		{
			name: "org scope field",
			payload: map[string]any{
				"name":  "my-key",
				"orgId": "7c254770-bf4c-4cc2-8f56-9f24df29deba",
			},
			msg: "orgId is not supported",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, srv, "POST", "/api/auth/api-keys/", tc.payload, token)
			testutil.StatusCode(t, http.StatusBadRequest, w.Code)
			testutil.Contains(t, w.Body.String(), tc.msg)
		})
	}
}
