package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Default durations for OAuth provider tokens.
const (
	DefaultAccessTokenDuration  = 1 * time.Hour
	DefaultRefreshTokenDuration = 30 * 24 * time.Hour
	DefaultAuthCodeDuration     = 10 * time.Minute
)

// OAuthProviderModeConfig holds runtime-configurable durations for the OAuth provider.
type OAuthProviderModeConfig struct {
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	AuthCodeDuration     time.Duration
}

// OAuthAuthorizationCode represents a stored authorization code.
type OAuthAuthorizationCode struct {
	ID                  string     `json:"id"`
	CodeHash            string     `json:"-"`
	ClientID            string     `json:"clientId"`
	UserID              string     `json:"userId"`
	RedirectURI         string     `json:"redirectUri"`
	Scope               string     `json:"scope"`
	AllowedTables       []string   `json:"allowedTables,omitempty"`
	CodeChallenge       string     `json:"-"`
	CodeChallengeMethod string     `json:"-"`
	State               string     `json:"state"`
	ExpiresAt           time.Time  `json:"expiresAt"`
	UsedAt              *time.Time `json:"usedAt,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
}

// OAuthToken represents a stored opaque token (access or refresh).
type OAuthToken struct {
	ID            string     `json:"id"`
	TokenHash     string     `json:"-"`
	TokenType     string     `json:"tokenType"` // "access" or "refresh"
	ClientID      string     `json:"clientId"`
	UserID        *string    `json:"userId,omitempty"` // nil for client_credentials
	Scope         string     `json:"scope"`
	AllowedTables []string   `json:"allowedTables,omitempty"`
	GrantID       string     `json:"grantId"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	RevokedAt     *time.Time `json:"revokedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// OAuthTokenResponse is the RFC 6749 §5.1 token response.
type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

// OAuthConsent represents a stored user consent record.
type OAuthConsent struct {
	ID            string    `json:"id"`
	UserID        string    `json:"userId"`
	ClientID      string    `json:"clientId"`
	Scope         string    `json:"scope"`
	AllowedTables []string  `json:"allowedTables,omitempty"`
	GrantedAt     time.Time `json:"grantedAt"`
}

// SetOAuthProviderModeConfig sets the OAuth provider token durations.
func (s *Service) SetOAuthProviderModeConfig(cfg OAuthProviderModeConfig) {
	s.oauthProviderCfg = cfg
}

func (s *Service) oauthAccessTokenDuration() time.Duration {
	if s.oauthProviderCfg.AccessTokenDuration > 0 {
		return s.oauthProviderCfg.AccessTokenDuration
	}
	return DefaultAccessTokenDuration
}

func (s *Service) oauthRefreshTokenDuration() time.Duration {
	if s.oauthProviderCfg.RefreshTokenDuration > 0 {
		return s.oauthProviderCfg.RefreshTokenDuration
	}
	return DefaultRefreshTokenDuration
}

func (s *Service) oauthAuthCodeDuration() time.Duration {
	if s.oauthProviderCfg.AuthCodeDuration > 0 {
		return s.oauthProviderCfg.AuthCodeDuration
	}
	return DefaultAuthCodeDuration
}

// --- Authorization Code ---

// CreateAuthorizationCode creates a new authorization code for the given client and user.
// Returns the plaintext code (to be sent to the client via redirect).
func (s *Service) CreateAuthorizationCode(ctx context.Context, clientID, userID, redirectURI, scope string, allowedTables []string, codeChallenge, codeChallengeMethod, state string) (string, error) {
	if codeChallengeMethod != "S256" {
		return "", NewOAuthError(OAuthErrInvalidRequest, "code_challenge_method must be S256")
	}
	if codeChallenge == "" {
		return "", NewOAuthError(OAuthErrInvalidRequest, "code_challenge is required")
	}
	if state == "" {
		return "", NewOAuthError(OAuthErrInvalidRequest, "state parameter is required")
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating authorization code: %w", err)
	}
	code := hex.EncodeToString(raw)
	codeHash := hashToken(code)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_authorization_codes
		 (code_hash, client_id, user_id, redirect_uri, scope, allowed_tables, code_challenge, code_challenge_method, state, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		codeHash, clientID, userID, redirectURI, scope, allowedTables,
		codeChallenge, codeChallengeMethod, state,
		time.Now().Add(s.oauthAuthCodeDuration()),
	)
	if err != nil {
		return "", fmt.Errorf("inserting authorization code: %w", err)
	}

	s.logger.Info("authorization code created", "client_id", clientID, "user_id", userID)
	return code, nil
}

// ExchangeAuthorizationCode validates and exchanges an authorization code for tokens.
// Implements grant_type=authorization_code per RFC 6749 §4.1.3.
func (s *Service) ExchangeAuthorizationCode(ctx context.Context, code, clientID, redirectURI, codeVerifier string) (*OAuthTokenResponse, error) {
	codeHash := hashToken(code)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("starting authorization code exchange transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var authCode OAuthAuthorizationCode
	err = tx.QueryRow(ctx,
		`SELECT id, code_hash, client_id, user_id, redirect_uri, scope, allowed_tables,
		        code_challenge, code_challenge_method, state, expires_at, used_at, created_at
		 FROM _ayb_oauth_authorization_codes WHERE code_hash = $1
		 FOR UPDATE`,
		codeHash,
	).Scan(&authCode.ID, &authCode.CodeHash, &authCode.ClientID, &authCode.UserID,
		&authCode.RedirectURI, &authCode.Scope, &authCode.AllowedTables,
		&authCode.CodeChallenge, &authCode.CodeChallengeMethod, &authCode.State,
		&authCode.ExpiresAt, &authCode.UsedAt, &authCode.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, NewOAuthError(OAuthErrInvalidGrant, "invalid authorization code")
		}
		return nil, fmt.Errorf("querying authorization code: %w", err)
	}

	// Check single-use.
	if authCode.UsedAt != nil {
		return nil, NewOAuthError(OAuthErrInvalidGrant, "authorization code already used")
	}

	// Validate expiry.
	if time.Now().After(authCode.ExpiresAt) {
		return nil, NewOAuthError(OAuthErrInvalidGrant, "authorization code expired")
	}

	// Validate client_id matches.
	if authCode.ClientID != clientID {
		return nil, NewOAuthError(OAuthErrInvalidGrant, "client_id mismatch")
	}

	// Validate redirect_uri matches.
	if authCode.RedirectURI != redirectURI {
		return nil, NewOAuthError(OAuthErrInvalidGrant, "redirect_uri mismatch")
	}

	// Verify PKCE.
	if !VerifyPKCE(codeVerifier, authCode.CodeChallenge, authCode.CodeChallengeMethod) {
		return nil, NewOAuthError(OAuthErrInvalidGrant, "PKCE verification failed")
	}

	// Consume the code only after all request validations pass.
	updateResult, err := tx.Exec(ctx,
		`UPDATE _ayb_oauth_authorization_codes SET used_at = NOW() WHERE id = $1 AND used_at IS NULL`,
		authCode.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("marking authorization code as used: %w", err)
	}
	if updateResult.RowsAffected() != 1 {
		return nil, NewOAuthError(OAuthErrInvalidGrant, "authorization code already used")
	}

	// Issue tokens in the same transaction as code consumption.
	userID := authCode.UserID
	resp, err := s.issueOAuthTokenPairTx(ctx, tx, clientID, &userID, authCode.Scope, authCode.AllowedTables)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing authorization code exchange transaction: %w", err)
	}
	return resp, nil
}

// --- Client Credentials Grant ---

// ClientCredentialsGrant issues an access token for client_credentials grant.
// No refresh token is issued (per RFC 6749 §4.4.3).
func (s *Service) ClientCredentialsGrant(ctx context.Context, clientID, scope string, allowedTables []string) (*OAuthTokenResponse, error) {
	accessToken, err := s.generateOAuthAccessToken()
	if err != nil {
		return nil, err
	}
	accessHash := hashToken(accessToken)
	grantID := uuid.New().String()

	_, err = s.pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_tokens (token_hash, token_type, client_id, user_id, scope, allowed_tables, grant_id, expires_at)
		 VALUES ($1, 'access', $2, NULL, $3, $4, $5, $6)`,
		accessHash, clientID, scope, allowedTables, grantID,
		time.Now().Add(s.oauthAccessTokenDuration()),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting client_credentials access token: %w", err)
	}

	s.logger.Info("client_credentials token issued", "client_id", clientID)
	return &OAuthTokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(s.oauthAccessTokenDuration().Seconds()),
		Scope:       scope,
	}, nil
}

// --- Refresh Token ---

// RefreshOAuthToken validates a refresh token and issues a new token pair.
// Implements refresh token rotation with reuse detection per RFC 9700 §4.14.2.
func (s *Service) RefreshOAuthToken(ctx context.Context, refreshToken, clientID string) (*OAuthTokenResponse, error) {
	tokenHash := hashToken(refreshToken)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("starting refresh token transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var token OAuthToken
	err = tx.QueryRow(ctx,
		`SELECT id, token_hash, token_type, client_id, user_id, scope, allowed_tables, grant_id, expires_at, revoked_at, created_at
		 FROM _ayb_oauth_tokens WHERE token_hash = $1 AND token_type = 'refresh'
		 FOR UPDATE`,
		tokenHash,
	).Scan(&token.ID, &token.TokenHash, &token.TokenType, &token.ClientID,
		&token.UserID, &token.Scope, &token.AllowedTables, &token.GrantID,
		&token.ExpiresAt, &token.RevokedAt, &token.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, NewOAuthError(OAuthErrInvalidGrant, "invalid refresh token")
		}
		return nil, fmt.Errorf("querying refresh token: %w", err)
	}

	// Reuse detection: if already revoked, someone is replaying a rotated token.
	// Revoke ALL tokens for this grant (compromise indicator per RFC 9700 §4.14.2).
	if token.RevokedAt != nil {
		_, err = tx.Exec(ctx,
			`UPDATE _ayb_oauth_tokens SET revoked_at = COALESCE(revoked_at, NOW()) WHERE grant_id = $1`,
			token.GrantID,
		)
		if err != nil {
			s.logger.Error("failed to revoke grant tokens on reuse detection", "grant_id", token.GrantID, "error", err)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			s.logger.Error("failed to commit grant revocation transaction", "grant_id", token.GrantID, "error", commitErr)
		}
		s.logger.Warn("refresh token reuse detected — all grant tokens revoked", "grant_id", token.GrantID, "client_id", clientID)
		return nil, NewOAuthError(OAuthErrInvalidGrant, "refresh token has been revoked (possible token theft)")
	}

	// Check expiry.
	if time.Now().After(token.ExpiresAt) {
		return nil, NewOAuthError(OAuthErrInvalidGrant, "refresh token expired")
	}

	// Validate client_id matches.
	if token.ClientID != clientID {
		return nil, NewOAuthError(OAuthErrInvalidGrant, "client_id mismatch")
	}

	// Rotation: revoke old refresh token.
	revokeResult, err := tx.Exec(ctx,
		`UPDATE _ayb_oauth_tokens SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`,
		token.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("revoking old refresh token: %w", err)
	}
	if revokeResult.RowsAffected() != 1 {
		_, revokeErr := tx.Exec(ctx,
			`UPDATE _ayb_oauth_tokens SET revoked_at = COALESCE(revoked_at, NOW()) WHERE grant_id = $1`,
			token.GrantID,
		)
		if revokeErr != nil {
			s.logger.Error("failed to revoke grant tokens after concurrent refresh detection", "grant_id", token.GrantID, "error", revokeErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			s.logger.Error("failed to commit concurrent refresh revocation transaction", "grant_id", token.GrantID, "error", commitErr)
		}
		return nil, NewOAuthError(OAuthErrInvalidGrant, "refresh token has been revoked (possible token theft)")
	}

	// Issue new pair with same grant_id.
	resp, err := s.issueOAuthTokenPairWithGrantIDTx(ctx, tx, token.ClientID, token.UserID, token.Scope, token.AllowedTables, token.GrantID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing refresh token transaction: %w", err)
	}
	return resp, nil
}

// --- Token Revocation (RFC 7009) ---

// RevokeOAuthToken revokes a token by its plaintext value.
// On refresh token revocation, also revokes all tokens with the same grant_id.
// Returns nil always (per RFC 7009, don't leak token existence).
func (s *Service) RevokeOAuthToken(ctx context.Context, token string) error {
	if s.pool == nil {
		s.logger.Warn("oauth token revocation skipped: database is not configured")
		return nil
	}

	tokenHash := hashToken(token)

	var tokenID, grantID, tokenType string
	err := s.pool.QueryRow(ctx,
		`SELECT id, grant_id, token_type FROM _ayb_oauth_tokens WHERE token_hash = $1`,
		tokenHash,
	).Scan(&tokenID, &grantID, &tokenType)
	if err != nil {
		// Per RFC 7009: return 200 OK even for unknown tokens.
		return nil
	}

	if tokenType == "refresh" {
		// Revoke all tokens for this grant.
		_, err = s.pool.Exec(ctx,
			`UPDATE _ayb_oauth_tokens SET revoked_at = COALESCE(revoked_at, NOW()) WHERE grant_id = $1`,
			grantID,
		)
	} else {
		// Revoke just the access token.
		_, err = s.pool.Exec(ctx,
			`UPDATE _ayb_oauth_tokens SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`,
			tokenID,
		)
	}
	if err != nil {
		s.logger.Error("failed to revoke oauth token", "token_type", tokenType, "error", err)
	}
	return nil
}

// --- Token Validation ---

// OAuthTokenInfo holds the validated token's claims.
type OAuthTokenInfo struct {
	UserID                    *string
	ClientID                  string
	Scope                     string
	AllowedTables             []string
	AppID                     string // resolved from the OAuth client's app_id
	AppRateLimitRPS           int    // app's configured RPS limit (0 = unlimited)
	AppRateLimitWindowSeconds int    // app's rate limit window in seconds (0 = default)
}

// ValidateOAuthToken validates an opaque OAuth access token.
// Returns token info if valid, or an error.
func (s *Service) ValidateOAuthToken(ctx context.Context, token string) (*OAuthTokenInfo, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("oauth token validation unavailable: database is not configured")
	}

	tokenHash := hashToken(token)

	var tok OAuthToken
	var appID string
	var appRateLimitRPS int
	var appRateLimitWindow int
	err := s.pool.QueryRow(ctx,
		`SELECT t.id, t.token_type, t.client_id, t.user_id, t.scope, t.allowed_tables,
		        t.grant_id, t.expires_at, t.revoked_at, c.app_id,
		        COALESCE(a.rate_limit_rps, 0), COALESCE(a.rate_limit_window_seconds, 0)
		 FROM _ayb_oauth_tokens t
		 JOIN _ayb_oauth_clients c ON c.client_id = t.client_id
		 LEFT JOIN _ayb_apps a ON a.id = c.app_id
		 WHERE t.token_hash = $1 AND t.token_type = 'access' AND c.revoked_at IS NULL`,
		tokenHash,
	).Scan(&tok.ID, &tok.TokenType, &tok.ClientID, &tok.UserID, &tok.Scope,
		&tok.AllowedTables, &tok.GrantID, &tok.ExpiresAt, &tok.RevokedAt, &appID,
		&appRateLimitRPS, &appRateLimitWindow)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("invalid oauth token")
		}
		return nil, fmt.Errorf("querying oauth token: %w", err)
	}

	if tok.RevokedAt != nil {
		return nil, fmt.Errorf("oauth token has been revoked")
	}
	if time.Now().After(tok.ExpiresAt) {
		return nil, fmt.Errorf("oauth token has expired")
	}

	var uid *string
	if tok.UserID != nil {
		uid = tok.UserID
	}

	return &OAuthTokenInfo{
		UserID:                    uid,
		ClientID:                  tok.ClientID,
		Scope:                     tok.Scope,
		AllowedTables:             tok.AllowedTables,
		AppID:                     appID,
		AppRateLimitRPS:           appRateLimitRPS,
		AppRateLimitWindowSeconds: appRateLimitWindow,
	}, nil
}

// --- Consent ---

// HasConsent checks if a user has previously consented to the given client request.
// The stored consent scope must cover the requested scope, and stored allowed_tables
// must cover requested allowed_tables when table restrictions are present.
func (s *Service) HasConsent(ctx context.Context, userID, clientID, scope string, allowedTables []string) (bool, error) {
	var consentScope string
	var consentAllowedTables []string
	err := s.pool.QueryRow(ctx,
		`SELECT scope, allowed_tables
		 FROM _ayb_oauth_consents
		 WHERE user_id = $1 AND client_id = $2`,
		userID, clientID,
	).Scan(&consentScope, &consentAllowedTables)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("checking consent: %w", err)
	}

	if !consentScopeCoversRequest(consentScope, scope) {
		return false, nil
	}
	if !consentAllowedTablesCoverRequest(consentAllowedTables, allowedTables) {
		return false, nil
	}
	return true, nil
}

// SaveConsent records that a user has consented to a client for a given scope.
// Uses INSERT ... ON CONFLICT to upsert.
func (s *Service) SaveConsent(ctx context.Context, userID, clientID, scope string, allowedTables []string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_consents (user_id, client_id, scope, allowed_tables)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, client_id) DO UPDATE
		 SET scope = $3, allowed_tables = $4, granted_at = NOW()`,
		userID, clientID, scope, allowedTables,
	)
	if err != nil {
		return fmt.Errorf("saving consent: %w", err)
	}
	return nil
}

// --- Internal helpers ---

func (s *Service) generateOAuthAccessToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating access token: %w", err)
	}
	return OAuthAccessTokenPrefix + hex.EncodeToString(raw), nil
}

func (s *Service) generateOAuthRefreshToken() (string, error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}
	return OAuthRefreshTokenPrefix + hex.EncodeToString(raw), nil
}

func (s *Service) issueOAuthTokenPairTx(ctx context.Context, tx pgx.Tx, clientID string, userID *string, scope string, allowedTables []string) (*OAuthTokenResponse, error) {
	grantID := uuid.New().String()
	return s.issueOAuthTokenPairWithGrantIDTx(ctx, tx, clientID, userID, scope, allowedTables, grantID)
}

func consentScopeCoversRequest(consentedScope, requestedScope string) bool {
	switch consentedScope {
	case ScopeFullAccess:
		return true
	case ScopeReadWrite:
		return requestedScope == ScopeReadWrite || requestedScope == ScopeReadOnly
	case ScopeReadOnly:
		return requestedScope == ScopeReadOnly
	default:
		return consentedScope == requestedScope
	}
}

func consentAllowedTablesCoverRequest(consentedTables, requestedTables []string) bool {
	// Nil/empty consent means unrestricted table access was granted.
	if len(consentedTables) == 0 {
		return true
	}
	// Restricted consent cannot satisfy unrestricted table requests.
	if len(requestedTables) == 0 {
		return false
	}

	allowed := make(map[string]struct{}, len(consentedTables))
	for _, t := range consentedTables {
		allowed[t] = struct{}{}
	}
	for _, requested := range requestedTables {
		if _, ok := allowed[requested]; !ok {
			return false
		}
	}
	return true
}

func (s *Service) issueOAuthTokenPairWithGrantIDTx(ctx context.Context, tx pgx.Tx, clientID string, userID *string, scope string, allowedTables []string, grantID string) (*OAuthTokenResponse, error) {
	accessToken, err := s.generateOAuthAccessToken()
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.generateOAuthRefreshToken()
	if err != nil {
		return nil, err
	}

	accessHash := hashToken(accessToken)
	refreshHash := hashToken(refreshToken)
	now := time.Now()

	_, err = tx.Exec(ctx,
		`INSERT INTO _ayb_oauth_tokens (token_hash, token_type, client_id, user_id, scope, allowed_tables, grant_id, expires_at)
		 VALUES ($1, 'access', $2, $3, $4, $5, $6, $7)`,
		accessHash, clientID, userID, scope, allowedTables, grantID,
		now.Add(s.oauthAccessTokenDuration()),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting access token: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO _ayb_oauth_tokens (token_hash, token_type, client_id, user_id, scope, allowed_tables, grant_id, expires_at)
		 VALUES ($1, 'refresh', $2, $3, $4, $5, $6, $7)`,
		refreshHash, clientID, userID, scope, allowedTables, grantID,
		now.Add(s.oauthRefreshTokenDuration()),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting refresh token: %w", err)
	}

	return &OAuthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.oauthAccessTokenDuration().Seconds()),
		RefreshToken: refreshToken,
		Scope:        scope,
	}, nil
}
