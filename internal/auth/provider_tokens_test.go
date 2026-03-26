package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestProviderTokenEncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()

	vault := &fakeProviderTokenVault{
		nonce: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
	}
	store := &ProviderTokenStore{vault: vault}

	combined, err := store.encryptToken("provider-access-token")
	testutil.NoError(t, err)
	testutil.True(t, len(combined) > providerTokenNonceSize, "encrypted payload should include nonce and ciphertext")
	testutil.Equal(t, providerTokenNonceSize, len(combined[:providerTokenNonceSize]))
	testutil.True(t, bytes.Equal(vault.nonce, combined[:providerTokenNonceSize]), "payload should start with nonce")

	plain, err := store.decryptToken(combined)
	testutil.NoError(t, err)
	testutil.Equal(t, "provider-access-token", plain)
	testutil.True(t, bytes.Equal(vault.lastDecryptNonce, combined[:providerTokenNonceSize]), "decrypt should receive prefixed nonce")
	testutil.True(t, bytes.Equal(vault.lastDecryptCiphertext, combined[providerTokenNonceSize:]), "decrypt should receive payload ciphertext")
}

func TestProviderTokenDecryptRejectsShortPayload(t *testing.T) {
	t.Parallel()

	store := &ProviderTokenStore{vault: &fakeProviderTokenVault{nonce: []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}}}
	_, err := store.decryptToken([]byte{1, 2, 3, 4, 5})
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "invalid encrypted token payload")
}

func TestSetProviderTokenResolverPersistsUntilStoreIsSet(t *testing.T) {
	t.Parallel()

	svc := newTestService()
	resolver := func(provider string) (OAuthClientConfig, OAuthProviderConfig, bool) {
		return OAuthClientConfig{ClientID: provider}, OAuthProviderConfig{TokenURL: "https://example.test/token"}, true
	}

	svc.SetProviderTokenResolver(resolver)

	store := &fakeResolverAwareProviderTokenStore{}
	svc.SetProviderTokenStore(store)

	testutil.True(t, store.setResolverCalled, "resolver should be applied when store is later injected")
	testutil.NotNil(t, store.resolver)
	client, _, ok := store.resolver("google")
	testutil.True(t, ok, "resolver should remain callable")
	testutil.Equal(t, "google", client.ClientID)
}

type fakeProviderTokenVault struct {
	nonce                 []byte
	lastDecryptCiphertext []byte
	lastDecryptNonce      []byte
}

func (f *fakeProviderTokenVault) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	cipher := append([]byte("enc:"), plaintext...)
	return cipher, append([]byte(nil), f.nonce...), nil
}

func (f *fakeProviderTokenVault) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	f.lastDecryptCiphertext = append([]byte(nil), ciphertext...)
	f.lastDecryptNonce = append([]byte(nil), nonce...)
	return bytes.TrimPrefix(ciphertext, []byte("enc:")), nil
}

type fakeProviderTokenStore struct {
	lastUserID       string
	lastProvider     string
	lastAccessToken  string
	lastRefreshToken string
	lastTokenType    string
	lastScopes       string
	lastExpiresAtSet bool
	calls            int
}

func (f *fakeProviderTokenStore) StoreTokens(_ context.Context, userID, provider, accessToken, refreshToken, tokenType, scopes string, expiresAt *time.Time) error {
	f.calls++
	f.lastUserID = userID
	f.lastProvider = provider
	f.lastAccessToken = accessToken
	f.lastRefreshToken = refreshToken
	f.lastTokenType = tokenType
	f.lastScopes = scopes
	f.lastExpiresAtSet = expiresAt != nil
	return nil
}

func (f *fakeProviderTokenStore) GetProviderToken(context.Context, string, string) (string, error) {
	return "", nil
}

func (f *fakeProviderTokenStore) ListProviderTokens(context.Context, string) ([]ProviderTokenInfo, error) {
	return nil, nil
}

func (f *fakeProviderTokenStore) DeleteProviderToken(context.Context, string, string) error {
	return nil
}

func (f *fakeProviderTokenStore) RefreshExpiringProviderTokens(context.Context, time.Duration) error {
	return nil
}

type fakeResolverAwareProviderTokenStore struct {
	fakeProviderTokenStore
	setResolverCalled bool
	resolver          ProviderTokenOAuthResolver
}

func (f *fakeResolverAwareProviderTokenStore) SetOAuthResolver(resolver ProviderTokenOAuthResolver) {
	f.setResolverCalled = true
	f.resolver = resolver
}

type providerTokenRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f providerTokenRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRefreshProviderAccessTokenRejectsEmptyRefreshToken(t *testing.T) {
	t.Parallel()

	httpCalled := false
	store := &ProviderTokenStore{
		httpClient: &http.Client{
			Transport: providerTokenRoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				httpCalled = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"access_token":"new-token"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
		oauthResolver: func(provider string) (OAuthClientConfig, OAuthProviderConfig, bool) {
			if provider != "google" {
				return OAuthClientConfig{}, OAuthProviderConfig{}, false
			}
			return OAuthClientConfig{ClientID: "id", ClientSecret: "secret"}, OAuthProviderConfig{
				TokenURL: "https://oauth.example/token",
			}, true
		},
	}

	_, err := store.refreshProviderAccessToken(context.Background(), "google", "   ")
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "refresh token is required")
	testutil.True(t, !httpCalled, "token endpoint should not be called for empty refresh tokens")
}

func TestRefreshProviderAccessTokenBuildsRequestAndParsesResponse(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC)
	store := &ProviderTokenStore{
		httpClient: &http.Client{
			Transport: providerTokenRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				testutil.Equal(t, http.MethodPost, req.Method)
				testutil.Equal(t, "https://oauth.example/token", req.URL.String())

				gotAuth := req.Header.Get("Authorization")
				wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("client-id:client-secret"))
				testutil.Equal(t, wantAuth, gotAuth)
				testutil.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))
				testutil.Equal(t, "application/json", req.Header.Get("Accept"))
				testutil.Equal(t, "enabled", req.Header.Get("X-Test-Mutator"))

				body, err := io.ReadAll(req.Body)
				testutil.NoError(t, err)
				form, err := url.ParseQuery(string(body))
				testutil.NoError(t, err)

				testutil.Equal(t, "refresh_token", form.Get("grant_type"))
				testutil.Equal(t, "refresh-123", form.Get("refresh_token"))
				testutil.Equal(t, "https://api.example", form.Get("audience"))
				testutil.Equal(t, "", form.Get("client_id"))
				testutil.Equal(t, "", form.Get("client_secret"))

				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(
						`{"access_token":"new-access","refresh_token":"new-refresh","token_type":"bearer","scope":"read write","expires_in":120}`,
					)),
					Header: make(http.Header),
				}, nil
			}),
		},
		oauthResolver: func(provider string) (OAuthClientConfig, OAuthProviderConfig, bool) {
			if provider != "google" {
				return OAuthClientConfig{}, OAuthProviderConfig{}, false
			}
			return OAuthClientConfig{
					ClientID:     "client-id",
					ClientSecret: "client-secret",
				}, OAuthProviderConfig{
					TokenURL:        "https://oauth.example/token",
					TokenAuthMethod: OAuthTokenAuthMethodClientSecretBasic,
					TokenRequestMutator: func(_ context.Context, _ string, _ OAuthClientConfig, form url.Values, headers http.Header) error {
						form.Set("audience", "https://api.example")
						headers.Set("X-Test-Mutator", "enabled")
						return nil
					},
				}, true
		},
		now: func() time.Time { return now },
	}

	refreshed, err := store.refreshProviderAccessToken(context.Background(), "google", "refresh-123")
	testutil.NoError(t, err)
	testutil.Equal(t, "new-access", refreshed.AccessToken)
	testutil.Equal(t, "new-refresh", refreshed.RefreshToken)
	testutil.Equal(t, "bearer", refreshed.TokenType)
	testutil.Equal(t, "read write", refreshed.Scopes)
	testutil.NotNil(t, refreshed.ExpiresAt)
	testutil.True(t, refreshed.ExpiresAt.Equal(now.Add(120*time.Second)), "expires_at should be now+expires_in")
}

func TestProviderTokensGoUnder500Lines(t *testing.T) {
	t.Parallel()
	assertFileUnder500Lines(t, "provider_tokens.go")
}
