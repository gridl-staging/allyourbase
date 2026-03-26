// Package auth provider_tokens.go implements encrypted OAuth provider token storage with automatic refresh capabilities and failure tracking.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	providerTokenNonceSize            = 12
	providerTokenRefreshFailureLimit  = 5
	providerTokenDefaultRefreshWindow = 10 * time.Minute
)

var (
	ErrProviderTokenStoreNotConfigured = errors.New("provider token store is not configured")
	ErrProviderTokenNotFound           = errors.New("provider token not found")
	ErrProviderTokenStale              = errors.New("provider token refresh is stale after repeated failures")
)

// ProviderTokenStorage defines provider token persistence and retrieval behavior.
type ProviderTokenStorage interface {
	StoreTokens(ctx context.Context, userID, provider, accessToken, refreshToken, tokenType, scopes string, expiresAt *time.Time) error
	GetProviderToken(ctx context.Context, userID, provider string) (string, error)
	ListProviderTokens(ctx context.Context, userID string) ([]ProviderTokenInfo, error)
	DeleteProviderToken(ctx context.Context, userID, provider string) error
	RefreshExpiringProviderTokens(ctx context.Context, window time.Duration) error
}

// ProviderTokenInfo is a redacted provider-token record for list APIs.
type ProviderTokenInfo struct {
	Provider            string     `json:"provider"`
	TokenType           string     `json:"token_type,omitempty"`
	Scopes              string     `json:"scopes,omitempty"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	RefreshFailureCount int        `json:"refresh_failure_count"`
	LastRefreshError    string     `json:"last_refresh_error,omitempty"`
	LastRefreshedAt     *time.Time `json:"last_refreshed_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// ProviderTokenOAuthResolver resolves OAuth client/provider config at refresh time.
type ProviderTokenOAuthResolver func(provider string) (OAuthClientConfig, OAuthProviderConfig, bool)

type providerTokenCryptor interface {
	Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error)
	Decrypt(ciphertext, nonce []byte) ([]byte, error)
}

// ProviderTokenRefreshResult is the normalized result returned by provider refresh.
type ProviderTokenRefreshResult struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	Scopes       string
	ExpiresAt    *time.Time
}

// ProviderTokenStore stores and refreshes vault-encrypted OAuth provider tokens.
type ProviderTokenStore struct {
	pool          *pgxpool.Pool
	vault         providerTokenCryptor
	logger        *slog.Logger
	httpClient    *http.Client
	oauthResolver ProviderTokenOAuthResolver
	now           func() time.Time
}

// NewProviderTokenStore creates a ProviderTokenStore.
func NewProviderTokenStore(pool *pgxpool.Pool, vault providerTokenCryptor, logger *slog.Logger) *ProviderTokenStore {
	return &ProviderTokenStore{
		pool:       pool,
		vault:      vault,
		logger:     logger,
		httpClient: oauthHTTPClient,
		now:        time.Now,
	}
}

// SetOAuthResolver configures provider credential/config lookup for token refresh.
func (s *ProviderTokenStore) SetOAuthResolver(resolver ProviderTokenOAuthResolver) {
	s.oauthResolver = resolver
}

// StoreTokens upserts encrypted provider tokens for a user/provider pair.
func (s *ProviderTokenStore) StoreTokens(ctx context.Context, userID, provider, accessToken, refreshToken, tokenType, scopes string, expiresAt *time.Time) error {
	if s == nil || s.pool == nil || s.vault == nil {
		return ErrProviderTokenStoreNotConfigured
	}
	userID = strings.TrimSpace(userID)
	provider = strings.TrimSpace(provider)
	if userID == "" || provider == "" {
		return fmt.Errorf("%w: user_id and provider are required", ErrValidation)
	}
	if strings.TrimSpace(accessToken) == "" {
		return fmt.Errorf("%w: access token is required", ErrValidation)
	}

	accessEnc, err := s.encryptToken(accessToken)
	if err != nil {
		return fmt.Errorf("encrypting access token: %w", err)
	}

	var refreshEnc []byte
	if strings.TrimSpace(refreshToken) != "" {
		refreshEnc, err = s.encryptToken(refreshToken)
		if err != nil {
			return fmt.Errorf("encrypting refresh token: %w", err)
		}
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_provider_tokens
		    (user_id, provider, access_token_enc, refresh_token_enc, token_type, scopes, expires_at, refresh_failure_count, last_refresh_error, updated_at)
		 VALUES
		    ($1, $2, $3, $4, $5, $6, $7, 0, NULL, NOW())
		 ON CONFLICT (user_id, provider) DO UPDATE
		 SET access_token_enc = EXCLUDED.access_token_enc,
		     refresh_token_enc = CASE
		         WHEN EXCLUDED.refresh_token_enc IS NULL OR OCTET_LENGTH(EXCLUDED.refresh_token_enc) = 0
		             THEN _ayb_oauth_provider_tokens.refresh_token_enc
		         ELSE EXCLUDED.refresh_token_enc
		     END,
		     token_type = EXCLUDED.token_type,
		     scopes = EXCLUDED.scopes,
		     expires_at = EXCLUDED.expires_at,
		     refresh_failure_count = 0,
		     last_refresh_error = NULL,
		     updated_at = NOW()`,
		userID, provider, accessEnc, refreshEnc, tokenType, scopes, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("upserting provider token: %w", err)
	}
	return nil
}

// GetProviderToken returns a usable access token, refreshing expired tokens when possible.
func (s *ProviderTokenStore) GetProviderToken(ctx context.Context, userID, provider string) (string, error) {
	if s == nil || s.pool == nil || s.vault == nil {
		return "", ErrProviderTokenStoreNotConfigured
	}
	userID = strings.TrimSpace(userID)
	provider = strings.TrimSpace(provider)
	if userID == "" || provider == "" {
		return "", fmt.Errorf("%w: user_id and provider are required", ErrValidation)
	}

	var (
		accessEnc           []byte
		refreshEnc          []byte
		tokenType           string
		scopes              string
		expiresAt           *time.Time
		refreshFailureCount int
	)
	err := s.pool.QueryRow(ctx,
		`SELECT access_token_enc,
		        refresh_token_enc,
		        COALESCE(token_type, ''),
		        COALESCE(scopes, ''),
		        expires_at,
		        COALESCE(refresh_failure_count, 0)
		 FROM _ayb_oauth_provider_tokens
		 WHERE user_id = $1 AND provider = $2`,
		userID, provider,
	).Scan(&accessEnc, &refreshEnc, &tokenType, &scopes, &expiresAt, &refreshFailureCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrProviderTokenNotFound
		}
		return "", fmt.Errorf("querying provider token: %w", err)
	}

	accessToken, err := s.decryptToken(accessEnc)
	if err != nil {
		return "", fmt.Errorf("decrypting access token: %w", err)
	}
	if !providerTokenExpired(expiresAt, s.now()) {
		return accessToken, nil
	}

	if refreshFailureCount >= providerTokenRefreshFailureLimit {
		return "", ErrProviderTokenStale
	}
	if len(refreshEnc) == 0 {
		return "", fmt.Errorf("provider token expired and no refresh token is stored")
	}

	refreshedAccessToken, err := s.refreshProviderToken(ctx, userID, provider, refreshEnc, tokenType, scopes)
	if err != nil {
		return "", err
	}

	return refreshedAccessToken, nil
}

// RefreshExpiringProviderTokens refreshes all non-stale provider tokens that expire within the window.
func (s *ProviderTokenStore) RefreshExpiringProviderTokens(ctx context.Context, window time.Duration) error {
	if s == nil || s.pool == nil || s.vault == nil {
		return ErrProviderTokenStoreNotConfigured
	}
	if window <= 0 {
		window = providerTokenDefaultRefreshWindow
	}

	rows, err := s.pool.Query(ctx,
		`SELECT user_id,
		        provider,
		        refresh_token_enc,
		        COALESCE(token_type, ''),
		        COALESCE(scopes, '')
		   FROM _ayb_oauth_provider_tokens
		  WHERE refresh_token_enc IS NOT NULL
		    AND OCTET_LENGTH(refresh_token_enc) > 0
		    AND refresh_failure_count < $1
		    AND expires_at IS NOT NULL
		    AND expires_at <= NOW() + make_interval(secs => $2)`,
		providerTokenRefreshFailureLimit, int64(window.Seconds()),
	)
	if err != nil {
		return fmt.Errorf("querying expiring provider tokens: %w", err)
	}
	defer rows.Close()

	var lastErr error
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}

		var (
			userID     string
			provider   string
			refreshEnc []byte
			tokenType  string
			scopes     string
		)
		if err := rows.Scan(&userID, &provider, &refreshEnc, &tokenType, &scopes); err != nil {
			return fmt.Errorf("scanning expiring provider token: %w", err)
		}

		if _, err := s.refreshProviderToken(ctx, userID, provider, refreshEnc, tokenType, scopes); err != nil {
			lastErr = err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating expiring provider tokens: %w", err)
	}

	return lastErr
}

// ListProviderTokens lists provider-token metadata for a user (never raw tokens).
func (s *ProviderTokenStore) ListProviderTokens(ctx context.Context, userID string) ([]ProviderTokenInfo, error) {
	if s == nil || s.pool == nil {
		return nil, ErrProviderTokenStoreNotConfigured
	}
	rows, err := s.pool.Query(ctx,
		`SELECT provider,
		        COALESCE(token_type, ''),
		        COALESCE(scopes, ''),
		        expires_at,
		        COALESCE(refresh_failure_count, 0),
		        COALESCE(last_refresh_error, ''),
		        last_refreshed_at,
		        created_at,
		        updated_at
		 FROM _ayb_oauth_provider_tokens
		 WHERE user_id = $1
		 ORDER BY provider`,
		strings.TrimSpace(userID),
	)
	if err != nil {
		return nil, fmt.Errorf("listing provider tokens: %w", err)
	}
	defer rows.Close()

	items := make([]ProviderTokenInfo, 0)
	for rows.Next() {
		var item ProviderTokenInfo
		if err := rows.Scan(
			&item.Provider,
			&item.TokenType,
			&item.Scopes,
			&item.ExpiresAt,
			&item.RefreshFailureCount,
			&item.LastRefreshError,
			&item.LastRefreshedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning provider token: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating provider tokens: %w", err)
	}
	return items, nil
}

// DeleteProviderToken deletes one provider-token record for a user.
func (s *ProviderTokenStore) DeleteProviderToken(ctx context.Context, userID, provider string) error {
	if s == nil || s.pool == nil {
		return ErrProviderTokenStoreNotConfigured
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_oauth_provider_tokens WHERE user_id = $1 AND provider = $2`,
		strings.TrimSpace(userID), strings.TrimSpace(provider),
	)
	if err != nil {
		return fmt.Errorf("deleting provider token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrProviderTokenNotFound
	}
	return nil
}

// encryptToken encrypts a plaintext token using the vault cryptor, prepending the nonce to the ciphertext, and returning the combined bytes.
func (s *ProviderTokenStore) encryptToken(token string) ([]byte, error) {
	if s == nil || s.vault == nil {
		return nil, ErrProviderTokenStoreNotConfigured
	}
	ciphertext, nonce, err := s.vault.Encrypt([]byte(token))
	if err != nil {
		return nil, err
	}
	if len(nonce) != providerTokenNonceSize {
		return nil, fmt.Errorf("unexpected nonce length %d (want %d)", len(nonce), providerTokenNonceSize)
	}
	combined := make([]byte, 0, len(nonce)+len(ciphertext))
	combined = append(combined, nonce...)
	combined = append(combined, ciphertext...)
	return combined, nil
}

func (s *ProviderTokenStore) decryptToken(combined []byte) (string, error) {
	if s == nil || s.vault == nil {
		return "", ErrProviderTokenStoreNotConfigured
	}
	if len(combined) <= providerTokenNonceSize {
		return "", fmt.Errorf("invalid encrypted token payload")
	}
	nonce := combined[:providerTokenNonceSize]
	ciphertext := combined[providerTokenNonceSize:]
	plaintext, err := s.vault.Decrypt(ciphertext, nonce)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
