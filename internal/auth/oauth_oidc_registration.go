package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

func jwksFetcherFromDiscovery(doc *OIDCDiscoveryDocument, cache *OIDCDiscoveryCache) *JWKSFetcher {
	if doc == nil || doc.JWKSURI == "" {
		return nil
	}

	fetcher := NewJWKSFetcher(doc.JWKSURI, 24*time.Hour)
	if cache != nil && cache.httpClient != nil {
		fetcher.SetHTTPClient(cache.httpClient)
	}
	return fetcher
}

// OIDCProviderRegistration holds the parameters needed to register a custom OIDC provider.
type OIDCProviderRegistration struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	Scopes       []string
	DisplayName  string
}

// RegisterOIDCProvider registers a custom OIDC provider using auto-discovery.
// It fetches the discovery document, creates a JWKS fetcher, and registers the
// provider in the OAuth provider map. The provider name must not conflict with
// built-in providers.
func RegisterOIDCProvider(name string, reg OIDCProviderRegistration, cache *OIDCDiscoveryCache) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("OIDC provider name is required")
	}
	if cache == nil {
		return fmt.Errorf("OIDC discovery cache is required")
	}

	if isBuiltInOAuthProviderName(name) {
		return fmt.Errorf("OIDC provider name %q conflicts with built-in provider", name)
	}

	doc, err := cache.Get(reg.IssuerURL)
	if err != nil {
		return fmt.Errorf("OIDC discovery for %q: %w", name, err)
	}
	for _, warning := range DiscoveryDocumentWarnings(doc) {
		slog.Warn("OIDC discovery warning", "provider", name, "issuer", doc.Issuer, "warning", warning)
	}

	var verifierMu sync.RWMutex
	currentDoc := doc
	currentFetcher := jwksFetcherFromDiscovery(doc, cache)
	clientID := reg.ClientID

	idTokenParser := func(ctx context.Context, idToken string) (*OAuthUserInfo, error) {
		verifierMu.RLock()
		docSnapshot := currentDoc
		fetcherSnapshot := currentFetcher
		verifierMu.RUnlock()

		if fetcherSnapshot == nil {
			return nil, fmt.Errorf("OIDC provider %q: no JWKS URI in discovery document", name)
		}

		info, err := VerifyOIDCIDToken(idToken, docSnapshot.Issuer, clientID, oauthExpectedNonceFromContext(ctx), fetcherSnapshot)
		if err == nil || !errors.Is(err, ErrOIDCIDTokenVerificationFailed) {
			return info, err
		}

		cache.Invalidate(reg.IssuerURL)
		refreshedDoc, refreshErr := cache.Get(reg.IssuerURL)
		if refreshErr != nil {
			return nil, err
		}
		refreshedFetcher := jwksFetcherFromDiscovery(refreshedDoc, cache)
		if refreshedFetcher == nil {
			return nil, fmt.Errorf("OIDC provider %q: no JWKS URI in discovery document", name)
		}

		verifierMu.Lock()
		currentDoc = refreshedDoc
		currentFetcher = refreshedFetcher
		verifierMu.Unlock()

		return VerifyOIDCIDToken(idToken, refreshedDoc.Issuer, clientID, oauthExpectedNonceFromContext(ctx), refreshedFetcher)
	}

	scopes := reg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}

	userInfoSource := OAuthUserInfoSourceIDTokenWithEndpointFallback
	if strings.TrimSpace(doc.UserInfoEndpoint) == "" {
		userInfoSource = OAuthUserInfoSourceIDToken
	}

	providerConfig := OAuthProviderConfig{
		AuthURL:               doc.AuthorizationEndpoint,
		TokenURL:              doc.TokenEndpoint,
		UserInfoURL:           doc.UserInfoEndpoint,
		Scopes:                scopes,
		DiscoveryURL:          reg.IssuerURL,
		UserInfoSource:        userInfoSource,
		IDTokenUserInfoParser: idTokenParser,
	}

	oauthMu.Lock()
	defer oauthMu.Unlock()
	oauthProviders[name] = providerConfig
	oauthUserInfoParsers[name] = parseOIDCUserInfo
	return nil
}

// UnregisterOIDCProvider removes a dynamically-registered OIDC provider.
func UnregisterOIDCProvider(name string) {
	name = strings.TrimSpace(name)
	if name == "" || isBuiltInOAuthProviderName(name) {
		return
	}

	oauthMu.Lock()
	defer oauthMu.Unlock()
	delete(oauthProviders, name)
	delete(oauthUserInfoParsers, name)
}

// fetchUserInfoFromEndpoint fetches user info from the provider's userinfo
// endpoint using the access token.
func fetchUserInfoFromEndpoint(ctx context.Context, provider string, pc OAuthProviderConfig, client OAuthClientConfig, tokenResp oauthTokenResponse, httpClient *http.Client) (*OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pc.UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	req.Header.Set("Accept", "application/json")
	applyUserInfoHeaders(req, pc.UserInfoHeaders, client)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOAuthProviderError, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading userinfo response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: userinfo endpoint returned %d", ErrOAuthProviderError, resp.StatusCode)
	}

	info, err := parseUserInfo(provider, body)
	if err != nil {
		return nil, err
	}

	if provider == "github" && info.Email == "" {
		email, fetchErr := fetchGitHubPrimaryEmail(ctx, tokenResp.AccessToken, httpClient)
		if fetchErr == nil {
			info.Email = email
		}
	}
	if provider == "bitbucket" && info.Email == "" {
		email, fetchErr := fetchBitbucketPrimaryEmail(ctx, tokenResp.AccessToken, httpClient)
		if fetchErr == nil {
			info.Email = email
		}
	}
	return info, nil
}
