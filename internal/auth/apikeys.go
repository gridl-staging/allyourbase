package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// APIKeyPrefix is the fixed prefix for all AYB API keys.
const APIKeyPrefix = "ayb_"

// apiKeyRawBytes is the number of random bytes in a generated key.
const apiKeyRawBytes = 24

// ErrAPIKeyNotFound is returned when an API key doesn't exist.
var ErrAPIKeyNotFound = errors.New("api key not found")

// ErrAPIKeyRevoked is returned when a revoked API key is used.
var ErrAPIKeyRevoked = errors.New("api key has been revoked")

// ErrAPIKeyExpired is returned when an expired API key is used.
var ErrAPIKeyExpired = errors.New("api key has expired")

// ErrInvalidScope is returned when an invalid scope is provided.
var ErrInvalidScope = errors.New("invalid scope: must be *, readonly, or readwrite")

// ErrInvalidAppID is returned when an API key references a non-existent app.
var ErrInvalidAppID = errors.New("app not found")

// APIKey represents an API key record (without the secret).
type APIKey struct {
	ID            string     `json:"id"`
	UserID        string     `json:"userId"`
	Name          string     `json:"name"`
	KeyPrefix     string     `json:"keyPrefix"`
	Scope         string     `json:"scope"`
	AllowedTables []string   `json:"allowedTables"`
	AppID         *string    `json:"appId"`
	LastUsedAt    *time.Time `json:"lastUsedAt"`
	ExpiresAt     *time.Time `json:"expiresAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	RevokedAt     *time.Time `json:"revokedAt"`
}

// APIKeyListResult is a paginated list of API keys.
type APIKeyListResult struct {
	Items      []APIKey `json:"items"`
	Page       int      `json:"page"`
	PerPage    int      `json:"perPage"`
	TotalItems int      `json:"totalItems"`
	TotalPages int      `json:"totalPages"`
}

// CreateAPIKeyOptions holds optional parameters for API key creation.
type CreateAPIKeyOptions struct {
	Scope         string   // "*", "readonly", "readwrite"; defaults to "*"
	AllowedTables []string // empty = all tables
	AppID         *string  // nil = user-scoped key (legacy); non-nil = app-scoped key
}

// CreateAPIKey generates a new API key for the given user.
// Returns the plaintext key (shown once) and the stored record.
func (s *Service) CreateAPIKey(ctx context.Context, userID, name string, opts ...CreateAPIKeyOptions) (string, *APIKey, error) {
	scope := ScopeFullAccess
	var allowedTables []string
	var appID *string
	if len(opts) > 0 {
		if opts[0].Scope != "" {
			scope = opts[0].Scope
		}
		allowedTables = opts[0].AllowedTables
		appID = opts[0].AppID
	}

	if !ValidScopes[scope] {
		return "", nil, ErrInvalidScope
	}

	if allowedTables == nil {
		allowedTables = []string{}
	}

	raw := make([]byte, apiKeyRawBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generating api key: %w", err)
	}

	plaintext := APIKeyPrefix + hex.EncodeToString(raw)
	hash := hashToken(plaintext)
	prefix := plaintext[:12] // "ayb_" + first 8 hex chars

	var key APIKey
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_api_keys (user_id, name, key_hash, key_prefix, scope, allowed_tables, app_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, user_id, name, key_prefix, scope, allowed_tables, app_id, last_used_at, expires_at, created_at, revoked_at`,
		userID, name, hash, prefix, scope, allowedTables, appID,
	).Scan(&key.ID, &key.UserID, &key.Name, &key.KeyPrefix, &key.Scope, &key.AllowedTables,
		&key.AppID, &key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt, &key.RevokedAt)
	if err != nil {
		return "", nil, mapCreateAPIKeyInsertError(err)
	}

	s.logger.Info("api key created", "key_id", key.ID, "user_id", userID, "name", name, "scope", scope, "app_id", appID)
	return plaintext, &key, nil
}

func mapCreateAPIKeyInsertError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503": // foreign_key_violation
			switch pgErr.ConstraintName {
			case "_ayb_api_keys_app_id_fkey":
				return ErrInvalidAppID
			case "_ayb_api_keys_user_id_fkey":
				return ErrUserNotFound
			}
		case "22P02": // invalid_text_representation (non-UUID app_id)
			return ErrInvalidAppID
		}
	}
	return fmt.Errorf("inserting api key: %w", err)
}

// ListAPIKeys returns all API keys for a specific user.
func (s *Service) ListAPIKeys(ctx context.Context, userID string) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, name, key_prefix, scope, allowed_tables, app_id, last_used_at, expires_at, created_at, revoked_at
		 FROM _ayb_api_keys WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying api keys: %w", err)
	}
	defer rows.Close()

	return scanAPIKeys(rows)
}

// ListAllAPIKeys returns a paginated list of all API keys (admin-only).
func (s *Service) ListAllAPIKeys(ctx context.Context, page, perPage int) (*APIKeyListResult, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	var totalItems int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_api_keys`).Scan(&totalItems)
	if err != nil {
		return nil, fmt.Errorf("counting api keys: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, name, key_prefix, scope, allowed_tables, app_id, last_used_at, expires_at, created_at, revoked_at
		 FROM _ayb_api_keys
		 ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("querying api keys: %w", err)
	}
	defer rows.Close()

	items, err := scanAPIKeys(rows)
	if err != nil {
		return nil, err
	}

	totalPages := totalItems / perPage
	if totalItems%perPage != 0 {
		totalPages++
	}

	return &APIKeyListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}, nil
}

// RevokeAPIKey revokes a key owned by the given user.
func (s *Service) RevokeAPIKey(ctx context.Context, keyID, userID string) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE _ayb_api_keys SET revoked_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		keyID, userID,
	)
	if err != nil {
		return fmt.Errorf("revoking api key: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	s.logger.Info("api key revoked", "key_id", keyID, "user_id", userID)
	return nil
}

// AdminRevokeAPIKey revokes any key regardless of owner (admin-only).
func (s *Service) AdminRevokeAPIKey(ctx context.Context, keyID string) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE _ayb_api_keys SET revoked_at = NOW()
		 WHERE id = $1 AND revoked_at IS NULL`,
		keyID,
	)
	if err != nil {
		return fmt.Errorf("revoking api key: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	s.logger.Info("api key revoked by admin", "key_id", keyID)
	return nil
}

// ValidateAPIKey validates a plaintext API key and returns JWT-compatible claims.
// It also updates the last_used_at timestamp.
func (s *Service) ValidateAPIKey(ctx context.Context, plaintext string) (*Claims, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("api key validation unavailable: database is not configured")
	}

	hash := hashToken(plaintext)

	var userID, email, scope string
	var allowedTables []string
	var revokedAt, expiresAt *time.Time
	var keyID string
	var appID *string
	var appRateLimitRPS, appRateLimitWindow *int
	err := s.pool.QueryRow(ctx,
		`SELECT k.id, k.user_id, k.revoked_at, k.expires_at, k.scope, k.allowed_tables, k.app_id, u.email,
		        a.rate_limit_rps, a.rate_limit_window_seconds
		 FROM _ayb_api_keys k
		 JOIN _ayb_users u ON u.id = k.user_id
		 LEFT JOIN _ayb_apps a ON a.id = k.app_id
		 WHERE k.key_hash = $1`,
		hash,
	).Scan(&keyID, &userID, &revokedAt, &expiresAt, &scope, &allowedTables, &appID, &email,
		&appRateLimitRPS, &appRateLimitWindow)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAPIKeyNotFound
		}
		return nil, fmt.Errorf("querying api key: %w", err)
	}

	if revokedAt != nil {
		return nil, ErrAPIKeyRevoked
	}
	if expiresAt != nil && time.Now().After(*expiresAt) {
		return nil, ErrAPIKeyExpired
	}

	// Update last_used_at (best-effort, don't fail the request).
	_, _ = s.pool.Exec(ctx,
		`UPDATE _ayb_api_keys SET last_used_at = NOW() WHERE id = $1`, keyID,
	)

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID,
		},
		Email:         email,
		APIKeyScope:   scope,
		AllowedTables: allowedTables,
	}
	applyAppRateLimitClaims(claims, appID, appRateLimitRPS, appRateLimitWindow)
	return claims, nil
}

func applyAppRateLimitClaims(claims *Claims, appID *string, appRateLimitRPS, appRateLimitWindow *int) {
	if claims == nil || appID == nil {
		return
	}
	claims.AppID = *appID
	if appRateLimitRPS != nil {
		claims.AppRateLimitRPS = *appRateLimitRPS
	}
	if appRateLimitWindow != nil {
		claims.AppRateLimitWindow = *appRateLimitWindow
	}
}

// IsAPIKey returns true if the token string looks like an AYB API key.
func IsAPIKey(token string) bool {
	return len(token) > len(APIKeyPrefix) && token[:len(APIKeyPrefix)] == APIKeyPrefix
}

func scanAPIKeys(rows pgx.Rows) ([]APIKey, error) {
	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.Scope, &k.AllowedTables,
			&k.AppID, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, fmt.Errorf("scanning api key: %w", err)
		}
		if k.AllowedTables == nil {
			k.AllowedTables = []string{}
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating api keys: %w", err)
	}
	if keys == nil {
		keys = []APIKey{}
	}
	return keys, nil
}
