// Package auth Handles OAuth provider token refresh operations, including calling the token endpoint and updating stored token metadata.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// refreshProviderToken refreshes the access token for a provider using its stored refresh token, updating the database with the new credentials or recording a refresh failure.
func (s *ProviderTokenStore) refreshProviderToken(ctx context.Context, userID, provider string, refreshEnc []byte, tokenType, scopes string) (string, error) {
	if len(refreshEnc) == 0 {
		return "", fmt.Errorf("provider token refresh token is missing")
	}

	refreshToken, err := s.decryptToken(refreshEnc)
	if err != nil {
		return "", fmt.Errorf("decrypting refresh token: %w", err)
	}

	refreshed, err := s.refreshProviderAccessToken(ctx, provider, refreshToken)
	if err != nil {
		newCount, updateErr := s.recordRefreshFailure(ctx, userID, provider, err.Error())
		if updateErr != nil {
			return "", fmt.Errorf("refreshing provider token: %w (failed to persist refresh failure: %v)", err, updateErr)
		}
		if newCount >= providerTokenRefreshFailureLimit {
			return "", ErrProviderTokenStale
		}
		return "", fmt.Errorf("refreshing provider token: %w", err)
	}

	nextRefreshToken := refreshToken
	if strings.TrimSpace(refreshed.RefreshToken) != "" {
		nextRefreshToken = refreshed.RefreshToken
	}
	nextTokenType := tokenType
	if strings.TrimSpace(refreshed.TokenType) != "" {
		nextTokenType = refreshed.TokenType
	}
	nextScopes := scopes
	if strings.TrimSpace(refreshed.Scopes) != "" {
		nextScopes = refreshed.Scopes
	}

	accessEnc, err := s.encryptToken(refreshed.AccessToken)
	if err != nil {
		return "", fmt.Errorf("encrypting refreshed access token: %w", err)
	}
	refreshEnc, err = s.encryptToken(nextRefreshToken)
	if err != nil {
		return "", fmt.Errorf("encrypting refreshed refresh token: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_oauth_provider_tokens
		 SET access_token_enc = $3,
		     refresh_token_enc = $4,
		     token_type = $5,
		     scopes = $6,
		     expires_at = $7,
		     refresh_failure_count = 0,
		     last_refresh_error = NULL,
		     last_refreshed_at = NOW(),
		     updated_at = NOW()
		 WHERE user_id = $1 AND provider = $2`,
		userID, provider, accessEnc, refreshEnc, nextTokenType, nextScopes, refreshed.ExpiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("persisting refreshed provider token: %w", err)
	}

	return refreshed.AccessToken, nil
}

// recordRefreshFailure increments the refresh failure count for a provider token and records the error message, returning the updated failure count.
func (s *ProviderTokenStore) recordRefreshFailure(ctx context.Context, userID, provider, refreshErr string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`UPDATE _ayb_oauth_provider_tokens
		 SET refresh_failure_count = refresh_failure_count + 1,
		     last_refresh_error = $3,
		     updated_at = NOW()
		 WHERE user_id = $1 AND provider = $2
		 RETURNING refresh_failure_count`,
		userID, provider, refreshErr,
	).Scan(&count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrProviderTokenNotFound
		}
		return 0, err
	}
	return count, nil
}

// refreshProviderAccessToken exchanges a refresh token with the OAuth provider for a new access token by making an HTTP request to the provider's token endpoint.
func (s *ProviderTokenStore) refreshProviderAccessToken(ctx context.Context, provider, refreshToken string) (*ProviderTokenRefreshResult, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("%w: refresh token is required", ErrValidation)
	}
	if s.oauthResolver == nil {
		return nil, fmt.Errorf("oauth provider resolver is not configured")
	}

	client, providerConfig, ok := s.oauthResolver(provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrOAuthNotConfigured, provider)
	}
	providerConfig = providerConfig.withResolvedTemplates()
	if strings.TrimSpace(providerConfig.TokenURL) == "" {
		return nil, fmt.Errorf("%w: token endpoint is not configured for %s", ErrOAuthNotConfigured, provider)
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	headers := make(http.Header)
	applyTokenAuth(form, headers, client, providerConfig)
	if providerConfig.TokenRequestMutator != nil {
		if err := providerConfig.TokenRequestMutator(ctx, provider, client, form, headers); err != nil {
			return nil, fmt.Errorf("building refresh token request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, providerConfig.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building refresh token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	httpClient := s.httpClient
	if httpClient == nil {
		httpClient = oauthHTTPClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading refresh token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	var tokenResp oauthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing refresh token response: %w", err)
	}
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return nil, fmt.Errorf("refresh token response missing access_token")
	}

	nowFn := s.now
	if nowFn == nil {
		nowFn = time.Now
	}

	return &ProviderTokenRefreshResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		Scopes:       tokenResp.Scope,
		ExpiresAt:    providerTokenExpiresAt(nowFn(), tokenResp),
	}, nil
}

func providerTokenExpiresAt(now time.Time, tokenResp oauthTokenResponse) *time.Time {
	if tokenResp.ExpiresIn <= 0 {
		return nil
	}

	expiresAt := now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return &expiresAt
}

func providerTokenExpired(expiresAt *time.Time, now time.Time) bool {
	return expiresAt != nil && !expiresAt.After(now)
}
