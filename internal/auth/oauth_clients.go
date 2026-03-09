package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// OAuth client ID/secret prefixes.
const (
	OAuthClientIDPrefix     = "ayb_cid_"
	OAuthClientSecretPrefix = "ayb_cs_"
)

// OAuth token prefixes.
const (
	OAuthAccessTokenPrefix  = "ayb_at_"
	OAuthRefreshTokenPrefix = "ayb_rt_"
)

// OAuth client types.
const (
	OAuthClientTypeConfidential = "confidential"
	OAuthClientTypePublic       = "public"
)

// OAuth error codes per RFC 6749 §5.2.
const (
	OAuthErrInvalidRequest       = "invalid_request"
	OAuthErrInvalidClient        = "invalid_client"
	OAuthErrInvalidGrant         = "invalid_grant"
	OAuthErrUnauthorizedClient   = "unauthorized_client"
	OAuthErrUnsupportedGrantType = "unsupported_grant_type"
	OAuthErrInvalidScope         = "invalid_scope"
	OAuthErrAccessDenied         = "access_denied"
)

// OAuthError is an RFC 6749 §5.2 error response.
type OAuthError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *OAuthError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Description)
}

// NewOAuthError creates a new RFC 6749 error.
func NewOAuthError(code, description string) *OAuthError {
	return &OAuthError{Code: code, Description: description}
}

// OAuthClient represents a registered OAuth 2.0 client.
type OAuthClient struct {
	ID                      string     `json:"id"`
	AppID                   string     `json:"appId"`
	ClientID                string     `json:"clientId"`
	Name                    string     `json:"name"`
	RedirectURIs            []string   `json:"redirectUris"`
	Scopes                  []string   `json:"scopes"`
	ClientType              string     `json:"clientType"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
	RevokedAt               *time.Time `json:"revokedAt"`
	ActiveAccessTokenCount  int        `json:"activeAccessTokenCount"`
	ActiveRefreshTokenCount int        `json:"activeRefreshTokenCount"`
	TotalGrants             int        `json:"totalGrants"`
	LastTokenIssuedAt       *time.Time `json:"lastTokenIssuedAt"`
}

// OAuthClientListResult is a paginated list of OAuth clients.
type OAuthClientListResult struct {
	Items      []OAuthClient `json:"items"`
	Page       int           `json:"page"`
	PerPage    int           `json:"perPage"`
	TotalItems int           `json:"totalItems"`
	TotalPages int           `json:"totalPages"`
}

// Sentinel errors for OAuth clients.
var (
	ErrOAuthClientNotFound            = errors.New("oauth client not found")
	ErrOAuthClientRevoked             = errors.New("oauth client has been revoked")
	ErrOAuthClientNameRequired        = errors.New("oauth client name is required")
	ErrOAuthAppRequired               = errors.New("app_id is required for oauth client")
	ErrOAuthClientPublicSecretRotator = errors.New("cannot regenerate secret for public client")
)

// --- Generators ---

// GenerateClientID generates a new OAuth client ID: ayb_cid_ + 24 random hex bytes.
func GenerateClientID() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating client id: %w", err)
	}
	return OAuthClientIDPrefix + hex.EncodeToString(raw), nil
}

// GenerateClientSecret generates a new OAuth client secret: ayb_cs_ + 32 random hex bytes.
func GenerateClientSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating client secret: %w", err)
	}
	return OAuthClientSecretPrefix + hex.EncodeToString(raw), nil
}

// IsOAuthClientID returns true if the string looks like an OAuth client ID.
func IsOAuthClientID(s string) bool {
	const expectedHexLen = 48 // 24 bytes
	if len(s) != len(OAuthClientIDPrefix)+expectedHexLen {
		return false
	}
	if !strings.HasPrefix(s, OAuthClientIDPrefix) {
		return false
	}
	for _, c := range s[len(OAuthClientIDPrefix):] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// IsOAuthAccessToken returns true if the token has the access token prefix.
func IsOAuthAccessToken(s string) bool {
	return len(s) > len(OAuthAccessTokenPrefix) && s[:len(OAuthAccessTokenPrefix)] == OAuthAccessTokenPrefix
}

// IsOAuthRefreshToken returns true if the token has the refresh token prefix.
func IsOAuthRefreshToken(s string) bool {
	return len(s) > len(OAuthRefreshTokenPrefix) && s[:len(OAuthRefreshTokenPrefix)] == OAuthRefreshTokenPrefix
}

// IsOAuthToken returns true if the token is either an OAuth access or refresh token.
func IsOAuthToken(s string) bool {
	return IsOAuthAccessToken(s) || IsOAuthRefreshToken(s)
}

// --- Secret hashing (SHA-256, same pattern as API keys) ---

// HashClientSecret hashes a client secret with SHA-256 for storage.
func HashClientSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// VerifyClientSecret checks a plaintext secret against a stored hash.
func VerifyClientSecret(secret, hash string) bool {
	computed := sha256.Sum256([]byte(secret))
	computedHex := hex.EncodeToString(computed[:])
	return subtle.ConstantTimeCompare([]byte(computedHex), []byte(hash)) == 1
}

// --- Redirect URI validation ---

// ValidateRedirectURIs validates a list of redirect URIs per RFC 6749 and RFC 8252.
// Rules: HTTPS required (except localhost), no query params, no fragments, no wildcards, exact match.
func ValidateRedirectURIs(uris []string) error {
	if len(uris) == 0 {
		return fmt.Errorf("at least one redirect URI is required")
	}
	for _, raw := range uris {
		if raw == "" {
			return fmt.Errorf("invalid redirect URI: empty string")
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("invalid redirect URI: %s", raw)
		}
		if u.RawQuery != "" {
			return fmt.Errorf("redirect URI must not contain query parameters: %s", raw)
		}
		if u.Fragment != "" {
			return fmt.Errorf("redirect URI must not contain fragment: %s", raw)
		}
		if strings.Contains(u.Host, "*") {
			return fmt.Errorf("redirect URI must not contain wildcard: %s", raw)
		}
		// Allow HTTP only for localhost/127.0.0.1 (development).
		host := u.Hostname()
		if u.Scheme == "http" && host != "localhost" && host != "127.0.0.1" {
			return fmt.Errorf("HTTPS required for non-localhost redirect URI: %s", raw)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("redirect URI must use http or https scheme: %s", raw)
		}
	}
	return nil
}

// MatchRedirectURI checks if a redirect URI exactly matches one of the registered URIs.
func MatchRedirectURI(uri string, registered []string) bool {
	for _, r := range registered {
		if uri == r {
			return true
		}
	}
	return false
}

// --- Scope validation ---

// ValidateOAuthScopes validates that all scopes are in the allowed set.
func ValidateOAuthScopes(scopes []string) error {
	if len(scopes) == 0 {
		return fmt.Errorf("at least one scope is required")
	}
	for _, s := range scopes {
		if !ValidScopes[s] {
			return fmt.Errorf("invalid scope: %s (must be one of: readonly, readwrite, *)", s)
		}
	}
	return nil
}

// IsScopeSubset returns true if the requested scope is contained in the allowed scopes.
// The "*" scope contains all others.
func IsScopeSubset(requested string, allowed []string) bool {
	for _, a := range allowed {
		if a == ScopeFullAccess || a == requested {
			return true
		}
	}
	return false
}

// --- Client type validation ---

// ValidateClientType validates the OAuth client type.
func ValidateClientType(ct string) error {
	if ct != OAuthClientTypeConfidential && ct != OAuthClientTypePublic {
		return fmt.Errorf("invalid client type: %s (must be confidential or public)", ct)
	}
	return nil
}

// --- PKCE (RFC 7636) ---

// GeneratePKCEChallenge computes S256(verifier) as base64url-no-pad.
func GeneratePKCEChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// VerifyPKCE checks a code_verifier against a stored code_challenge using S256.
// Only S256 is supported (plain is rejected per RFC 9700).
func VerifyPKCE(verifier, challenge, method string) bool {
	if method != "S256" {
		return false
	}
	computed := GeneratePKCEChallenge(verifier)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// --- Database CRUD ---

// RegisterOAuthClient creates a new OAuth client linked to an app.
// Returns the plaintext client_secret (shown once) for confidential clients, empty for public.
func (s *Service) RegisterOAuthClient(ctx context.Context, appID, name, clientType string, redirectURIs, scopes []string) (string, *OAuthClient, error) {
	if name == "" {
		return "", nil, ErrOAuthClientNameRequired
	}
	if appID == "" {
		return "", nil, ErrOAuthAppRequired
	}
	if err := ValidateClientType(clientType); err != nil {
		return "", nil, err
	}
	if err := ValidateRedirectURIs(redirectURIs); err != nil {
		return "", nil, err
	}
	if err := ValidateOAuthScopes(scopes); err != nil {
		return "", nil, err
	}

	clientID, err := GenerateClientID()
	if err != nil {
		return "", nil, err
	}

	var secretPlaintext string
	var secretHash *string
	if clientType == OAuthClientTypeConfidential {
		secretPlaintext, err = GenerateClientSecret()
		if err != nil {
			return "", nil, err
		}
		h := HashClientSecret(secretPlaintext)
		secretHash = &h
	}

	var client OAuthClient
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_oauth_clients (app_id, client_id, client_secret_hash, name, redirect_uris, scopes, client_type)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, app_id, client_id, name, redirect_uris, scopes, client_type, created_at, updated_at, revoked_at`,
		appID, clientID, secretHash, name, redirectURIs, scopes, clientType,
	).Scan(&client.ID, &client.AppID, &client.ClientID, &client.Name,
		&client.RedirectURIs, &client.Scopes, &client.ClientType,
		&client.CreatedAt, &client.UpdatedAt, &client.RevokedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23503": // foreign_key_violation
				return "", nil, ErrAppNotFound
			case "22P02": // invalid_text_representation (bad UUID)
				return "", nil, ErrAppNotFound
			}
		}
		return "", nil, fmt.Errorf("inserting oauth client: %w", err)
	}

	s.logger.Info("oauth client registered", "client_id", client.ClientID, "app_id", appID, "type", clientType)
	return secretPlaintext, &client, nil
}

// GetOAuthClient retrieves an OAuth client by its client_id string (not UUID).
func (s *Service) GetOAuthClient(ctx context.Context, clientID string) (*OAuthClient, error) {
	var client OAuthClient
	err := s.pool.QueryRow(ctx,
		`SELECT id, app_id, client_id, name, redirect_uris, scopes, client_type, created_at, updated_at, revoked_at
		 FROM _ayb_oauth_clients WHERE client_id = $1`,
		clientID,
	).Scan(&client.ID, &client.AppID, &client.ClientID, &client.Name,
		&client.RedirectURIs, &client.Scopes, &client.ClientType,
		&client.CreatedAt, &client.UpdatedAt, &client.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOAuthClientNotFound
		}
		return nil, fmt.Errorf("querying oauth client: %w", err)
	}
	return &client, nil
}

// GetOAuthClientByUUID retrieves an OAuth client by its internal UUID.
func (s *Service) GetOAuthClientByUUID(ctx context.Context, id string) (*OAuthClient, error) {
	var client OAuthClient
	err := s.pool.QueryRow(ctx,
		`SELECT id, app_id, client_id, name, redirect_uris, scopes, client_type, created_at, updated_at, revoked_at
		 FROM _ayb_oauth_clients WHERE id = $1`,
		id,
	).Scan(&client.ID, &client.AppID, &client.ClientID, &client.Name,
		&client.RedirectURIs, &client.Scopes, &client.ClientType,
		&client.CreatedAt, &client.UpdatedAt, &client.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOAuthClientNotFound
		}
		return nil, fmt.Errorf("querying oauth client: %w", err)
	}
	return &client, nil
}

// ListOAuthClients returns a paginated list of all OAuth clients.
func (s *Service) ListOAuthClients(ctx context.Context, page, perPage int) (*OAuthClientListResult, error) {
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
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_oauth_clients`).Scan(&totalItems)
	if err != nil {
		return nil, fmt.Errorf("counting oauth clients: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT
			c.id,
			c.app_id,
			c.client_id,
			c.name,
			c.redirect_uris,
			c.scopes,
			c.client_type,
			c.created_at,
			c.updated_at,
			c.revoked_at,
			COALESCE(stats.active_access_token_count, 0) AS active_access_token_count,
			COALESCE(stats.active_refresh_token_count, 0) AS active_refresh_token_count,
			COALESCE(stats.total_grants, 0) AS total_grants,
			stats.last_token_issued_at
		FROM _ayb_oauth_clients c
		LEFT JOIN (
			SELECT
				client_id,
				COUNT(*) FILTER (
					WHERE token_type = 'access' AND revoked_at IS NULL AND expires_at > NOW()
				) AS active_access_token_count,
				COUNT(*) FILTER (
					WHERE token_type = 'refresh' AND revoked_at IS NULL AND expires_at > NOW()
				) AS active_refresh_token_count,
				COUNT(DISTINCT grant_id) AS total_grants,
				MAX(created_at) AS last_token_issued_at
			FROM _ayb_oauth_tokens
			GROUP BY client_id
		) stats ON stats.client_id = c.client_id
		ORDER BY c.created_at DESC
		LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing oauth clients: %w", err)
	}
	defer rows.Close()

	var items []OAuthClient
	for rows.Next() {
		var c OAuthClient
		if err := rows.Scan(&c.ID, &c.AppID, &c.ClientID, &c.Name,
			&c.RedirectURIs, &c.Scopes, &c.ClientType,
			&c.CreatedAt, &c.UpdatedAt, &c.RevokedAt,
			&c.ActiveAccessTokenCount, &c.ActiveRefreshTokenCount,
			&c.TotalGrants, &c.LastTokenIssuedAt); err != nil {
			return nil, fmt.Errorf("scanning oauth client: %w", err)
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating oauth clients: %w", err)
	}
	if items == nil {
		items = []OAuthClient{}
	}

	totalPages := totalItems / perPage
	if totalItems%perPage != 0 {
		totalPages++
	}

	return &OAuthClientListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}, nil
}

// UpdateOAuthClient updates a non-revoked OAuth client's name, redirect URIs, and scopes.
// Client type and app association are immutable — use delete + recreate to change those.
func (s *Service) UpdateOAuthClient(ctx context.Context, clientID, name string, redirectURIs, scopes []string) (*OAuthClient, error) {
	if name == "" {
		return nil, ErrOAuthClientNameRequired
	}
	if err := ValidateRedirectURIs(redirectURIs); err != nil {
		return nil, err
	}
	if err := ValidateOAuthScopes(scopes); err != nil {
		return nil, err
	}

	// Pre-check: distinguish "not found" from "revoked" for clear error messages.
	existing, err := s.GetOAuthClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if existing.RevokedAt != nil {
		return nil, ErrOAuthClientRevoked
	}

	var client OAuthClient
	err = s.pool.QueryRow(ctx,
		`UPDATE _ayb_oauth_clients
		 SET name = $2, redirect_uris = $3, scopes = $4, updated_at = NOW()
		 WHERE client_id = $1 AND revoked_at IS NULL
		 RETURNING id, app_id, client_id, name, redirect_uris, scopes, client_type, created_at, updated_at, revoked_at`,
		clientID, name, redirectURIs, scopes,
	).Scan(&client.ID, &client.AppID, &client.ClientID, &client.Name,
		&client.RedirectURIs, &client.Scopes, &client.ClientType,
		&client.CreatedAt, &client.UpdatedAt, &client.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOAuthClientNotFound
		}
		return nil, fmt.Errorf("updating oauth client: %w", err)
	}

	s.logger.Info("oauth client updated", "client_id", clientID)
	return &client, nil
}

// RevokeOAuthClient soft-deletes an OAuth client by setting revoked_at.
func (s *Service) RevokeOAuthClient(ctx context.Context, clientID string) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE _ayb_oauth_clients SET revoked_at = NOW(), updated_at = NOW()
		 WHERE client_id = $1 AND revoked_at IS NULL`,
		clientID,
	)
	if err != nil {
		return fmt.Errorf("revoking oauth client: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrOAuthClientNotFound
	}
	s.logger.Info("oauth client revoked", "client_id", clientID)
	return nil
}

// RegenerateOAuthClientSecret generates a new secret for a confidential client.
// Returns the plaintext secret (shown once).
func (s *Service) RegenerateOAuthClientSecret(ctx context.Context, clientID string) (string, error) {
	// Check client exists and is confidential.
	client, err := s.GetOAuthClient(ctx, clientID)
	if err != nil {
		return "", err
	}
	if client.RevokedAt != nil {
		return "", ErrOAuthClientRevoked
	}
	if client.ClientType != OAuthClientTypeConfidential {
		return "", ErrOAuthClientPublicSecretRotator
	}

	newSecret, err := GenerateClientSecret()
	if err != nil {
		return "", err
	}
	newHash := HashClientSecret(newSecret)

	result, err := s.pool.Exec(ctx,
		`UPDATE _ayb_oauth_clients SET client_secret_hash = $1, updated_at = NOW()
		 WHERE client_id = $2 AND revoked_at IS NULL`,
		newHash, clientID,
	)
	if err != nil {
		return "", fmt.Errorf("updating client secret: %w", err)
	}
	if result.RowsAffected() == 0 {
		return "", ErrOAuthClientNotFound
	}

	s.logger.Info("oauth client secret regenerated", "client_id", clientID)
	return newSecret, nil
}

// ValidateOAuthClientCredentials validates client_id + client_secret.
// Returns the client if valid, or an error.
func (s *Service) ValidateOAuthClientCredentials(ctx context.Context, clientID, clientSecret string) (*OAuthClient, error) {
	var client OAuthClient
	var secretHash *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, app_id, client_id, client_secret_hash, name, redirect_uris, scopes, client_type, created_at, updated_at, revoked_at
		 FROM _ayb_oauth_clients WHERE client_id = $1`,
		clientID,
	).Scan(&client.ID, &client.AppID, &client.ClientID, &secretHash, &client.Name,
		&client.RedirectURIs, &client.Scopes, &client.ClientType,
		&client.CreatedAt, &client.UpdatedAt, &client.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, NewOAuthError(OAuthErrInvalidClient, "unknown client")
		}
		return nil, fmt.Errorf("querying oauth client: %w", err)
	}

	if client.RevokedAt != nil {
		return nil, NewOAuthError(OAuthErrInvalidClient, "client has been revoked")
	}

	if client.ClientType == OAuthClientTypeConfidential {
		if secretHash == nil || clientSecret == "" {
			return nil, NewOAuthError(OAuthErrInvalidClient, "client authentication required")
		}
		if !VerifyClientSecret(clientSecret, *secretHash) {
			return nil, NewOAuthError(OAuthErrInvalidClient, "invalid client credentials")
		}
	}

	return &client, nil
}
