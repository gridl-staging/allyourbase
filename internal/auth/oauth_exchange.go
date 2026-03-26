package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func exchangeCode(ctx context.Context, provider string, client OAuthClientConfig, code, redirectURI string, pc OAuthProviderConfig, httpClient *http.Client) (*OAuthUserInfo, error) {
	info, _, err := exchangeCodeWithTokenResponse(ctx, provider, client, code, redirectURI, pc, httpClient)
	return info, err
}

// Exchanges an OAuth authorization code for provider tokens and fetches user information, returning both the normalized user data and the raw token response including access token and optional refresh token.
func exchangeCodeWithTokenResponse(ctx context.Context, provider string, client OAuthClientConfig, code, redirectURI string, pc OAuthProviderConfig, httpClient *http.Client) (*OAuthUserInfo, oauthTokenResponse, error) {
	pc = pc.withResolvedTemplates()

	// Exchange code for access token.
	data := url.Values{
		"code":         {code},
		"redirect_uri": {redirectURI},
		"grant_type":   {"authorization_code"},
	}
	headers := make(http.Header)
	applyTokenAuth(data, headers, client, pc)
	if pc.TokenRequestMutator != nil {
		if err := pc.TokenRequestMutator(ctx, provider, client, data, headers); err != nil {
			return nil, oauthTokenResponse{}, fmt.Errorf("building token request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pc.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, oauthTokenResponse{}, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, oauthTokenResponse{}, fmt.Errorf("%w: %v", ErrOAuthCodeExchange, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, oauthTokenResponse{}, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, oauthTokenResponse{}, fmt.Errorf("%w: token endpoint returned %d", ErrOAuthCodeExchange, resp.StatusCode)
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, oauthTokenResponse{}, fmt.Errorf("parsing token response: %w", err)
	}

	switch pc.userInfoSource() {
	case OAuthUserInfoSourceTokenResponse:
		if pc.TokenResponseUserInfoExtractor == nil {
			return nil, oauthTokenResponse{}, fmt.Errorf("%w: token response user info extractor not configured for %s", ErrOAuthNotConfigured, provider)
		}
		info, err := pc.TokenResponseUserInfoExtractor(body)
		if err != nil {
			return nil, oauthTokenResponse{}, fmt.Errorf("%w: %v", ErrOAuthProviderError, err)
		}
		if info == nil || info.ProviderUserID == "" {
			return nil, oauthTokenResponse{}, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
		}
		return info, tokenResp, nil
	case OAuthUserInfoSourceIDToken:
		if tokenResp.IDToken == "" {
			return nil, oauthTokenResponse{}, fmt.Errorf("%w: empty id_token", ErrOAuthCodeExchange)
		}
	case OAuthUserInfoSourceIDTokenWithEndpointFallback:
		if tokenResp.AccessToken == "" && tokenResp.IDToken == "" {
			return nil, oauthTokenResponse{}, fmt.Errorf("%w: no access_token or id_token", ErrOAuthCodeExchange)
		}
	default:
		if tokenResp.AccessToken == "" {
			return nil, oauthTokenResponse{}, fmt.Errorf("%w: empty access token", ErrOAuthCodeExchange)
		}
	}

	// Fetch user info according to the configured source.
	info, err := fetchUserInfoWithConfig(ctx, provider, pc, client, tokenResp, httpClient)
	if err != nil {
		return nil, oauthTokenResponse{}, err
	}
	return info, tokenResp, nil
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

func applyTokenAuth(form url.Values, headers http.Header, client OAuthClientConfig, pc OAuthProviderConfig) {
	switch pc.tokenAuthMethod() {
	case OAuthTokenAuthMethodClientSecretBasic:
		// RFC 6749 §2.3.1: when using HTTP Basic auth, client credentials go
		// in the Authorization header only — not duplicated in the request body.
		form.Del("client_id")
		form.Del("client_secret")
		credentials := client.ClientID + ":" + client.ClientSecret
		headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(credentials)))
	default:
		form.Set("client_id", client.ClientID)
		form.Set("client_secret", client.ClientSecret)
	}
}

func fetchUserInfo(ctx context.Context, provider, userInfoURL, accessToken string, httpClient *http.Client) (*OAuthUserInfo, error) {
	pc := OAuthProviderConfig{
		UserInfoURL: userInfoURL,
	}
	return fetchUserInfoWithConfig(ctx, provider, pc, OAuthClientConfig{}, oauthTokenResponse{
		AccessToken: accessToken,
	}, httpClient)
}

// Fetches and normalizes user information according to the provider's configured source strategy, which may retrieve from id_token, the token response directly, or the user info endpoint, with support for hybrid approaches that attempt multiple sources.
func fetchUserInfoWithConfig(ctx context.Context, provider string, pc OAuthProviderConfig, client OAuthClientConfig, tokenResp oauthTokenResponse, httpClient *http.Client) (*OAuthUserInfo, error) {
	pc = pc.withResolvedTemplates()

	switch pc.userInfoSource() {
	case OAuthUserInfoSourceIDToken:
		if pc.IDTokenUserInfoParser == nil {
			return nil, fmt.Errorf("%w: id_token parser not configured for %s", ErrOAuthNotConfigured, provider)
		}
		info, err := pc.IDTokenUserInfoParser(ctx, tokenResp.IDToken)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrOAuthProviderError, err)
		}
		if info == nil || info.ProviderUserID == "" {
			return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
		}
		return info, nil

	case OAuthUserInfoSourceIDTokenWithEndpointFallback:
		// Try id_token first.
		if pc.IDTokenUserInfoParser != nil && tokenResp.IDToken != "" {
			info, err := pc.IDTokenUserInfoParser(ctx, tokenResp.IDToken)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrOAuthProviderError, err)
			}
			if info == nil || info.ProviderUserID == "" {
				return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
			}
			// Got user from id_token. Fill missing email from endpoint if possible.
			if info.Email == "" && tokenResp.AccessToken != "" && pc.UserInfoURL != "" {
				if epInfo, epErr := fetchUserInfoFromEndpoint(ctx, provider, pc, client, tokenResp, httpClient); epErr == nil {
					if epInfo.Email != "" {
						info.Email = epInfo.Email
					}
					if info.Name == "" && epInfo.Name != "" {
						info.Name = epInfo.Name
					}
				}
			}
			return info, nil
		}
		// id_token unavailable (no parser or empty token) — try endpoint.
		if tokenResp.AccessToken != "" && pc.UserInfoURL != "" {
			return fetchUserInfoFromEndpoint(ctx, provider, pc, client, tokenResp, httpClient)
		}
		return nil, fmt.Errorf("%w: OIDC provider returned neither valid id_token nor usable access_token", ErrOAuthCodeExchange)
	}

	// Default: fetch from userinfo endpoint.
	return fetchUserInfoFromEndpoint(ctx, provider, pc, client, tokenResp, httpClient)
}

func withOAuthExpectedNonce(ctx context.Context, nonce string) context.Context {
	if strings.TrimSpace(nonce) == "" {
		return ctx
	}
	return context.WithValue(ctx, oauthExpectedNonceContextKey, nonce)
}

func oauthExpectedNonceFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	nonce, _ := ctx.Value(oauthExpectedNonceContextKey).(string)
	return nonce
}
