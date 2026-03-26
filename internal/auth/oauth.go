// Package auth OAuth 2.0 integration for multiple providers including Google, GitHub, Microsoft, and Apple. It manages authorization flows, token exchange, user info fetching and parsing, CSRF state validation, and OAuth identity linking to user accounts.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Sentinel errors for OAuth.
var (
	ErrOAuthStateMismatch = errors.New("OAuth state mismatch")
	ErrOAuthCodeExchange  = errors.New("OAuth code exchange failed")
	ErrOAuthProviderError = errors.New("OAuth provider error")
	ErrOAuthNotConfigured = errors.New("OAuth provider not configured")
)

// oauthHTTPClient is used for all OAuth HTTP requests. It has a 10-second
// timeout to prevent hanging goroutines when provider servers are slow.
var oauthHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
}

// SetOAuthHTTPTransport overrides the transport used by the shared OAuth HTTP client.
func SetOAuthHTTPTransport(rt http.RoundTripper) {
	if rt == nil {
		return
	}
	oauthHTTPClient.Transport = rt
}

type oauthContextKey string

const oauthExpectedNonceContextKey oauthContextKey = "oauth_expected_nonce"

type OAuthTokenAuthMethod string

const (
	// OAuthTokenAuthMethodClientSecretPost sends client_id/client_secret in the
	// form body (the default for most providers).
	OAuthTokenAuthMethodClientSecretPost OAuthTokenAuthMethod = "client_secret_post"
	// OAuthTokenAuthMethodClientSecretBasic sends client credentials via HTTP
	// Basic auth (used by providers like Zoom/Notion/Twitter confidential).
	OAuthTokenAuthMethodClientSecretBasic OAuthTokenAuthMethod = "client_secret_basic"
)

type OAuthUserInfoSource string

const (
	// OAuthUserInfoSourceEndpoint fetches user info from the configured endpoint.
	OAuthUserInfoSourceEndpoint OAuthUserInfoSource = "userinfo_endpoint"
	// OAuthUserInfoSourceIDToken reads user info from the token response id_token.
	OAuthUserInfoSourceIDToken OAuthUserInfoSource = "id_token"
	// OAuthUserInfoSourceTokenResponse reads user info directly from token response JSON.
	OAuthUserInfoSourceTokenResponse OAuthUserInfoSource = "token_response"
)

type OAuthTokenRequestMutator func(ctx context.Context, provider string, client OAuthClientConfig, form url.Values, headers http.Header) error
type OAuthTokenResponseUserInfoExtractor func(tokenBody []byte) (*OAuthUserInfo, error)
type OAuthIDTokenUserInfoParser func(ctx context.Context, idToken string) (*OAuthUserInfo, error)
type OAuthUserInfoParser func(body []byte) (*OAuthUserInfo, error)

// OAuthProviderConfig holds OAuth endpoints and provider-specific behavior knobs.
// OAuthProviderConfig holds OAuth endpoint URLs, scopes, and provider-specific behavior controls including token authentication method, user info source strategy, and optional customization hooks for token requests and user info extraction.
type OAuthProviderConfig struct {
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	Scopes      []string

	// Provider-specific optional config fields.
	TeamID           string
	KeyID            string
	PrivateKey       string
	TenantID         string
	DiscoveryURL     string
	ResponseMode     string
	AdditionalParams map[string]string

	// OAuth abstraction behavior controls.
	TokenAuthMethod                OAuthTokenAuthMethod
	UserInfoSource                 OAuthUserInfoSource
	UserInfoHeaders                map[string]string
	TokenRequestMutator            OAuthTokenRequestMutator
	TokenResponseUserInfoExtractor OAuthTokenResponseUserInfoExtractor
	IDTokenUserInfoParser          OAuthIDTokenUserInfoParser
}

// Known provider configurations.
var (
	oauthMu        sync.RWMutex
	oauthProviders = map[string]OAuthProviderConfig{
		"google": {
			AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:    "https://oauth2.googleapis.com/token",
			UserInfoURL: "https://www.googleapis.com/oauth2/v2/userinfo",
			Scopes:      []string{"openid", "email", "profile"},
		},
		"github": {
			AuthURL:     "https://github.com/login/oauth/authorize",
			TokenURL:    "https://github.com/login/oauth/access_token",
			UserInfoURL: "https://api.github.com/user",
			Scopes:      []string{"user:email"},
		},
		"microsoft": {
			AuthURL:     "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/authorize",
			TokenURL:    "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/token",
			UserInfoURL: "https://graph.microsoft.com/v1.0/me",
			Scopes:      []string{"openid", "profile", "email", "User.Read"},
			TenantID:    "common",
		},
		"apple": {
			AuthURL:        "https://appleid.apple.com/auth/authorize",
			TokenURL:       "https://appleid.apple.com/auth/token",
			Scopes:         []string{"name", "email"},
			ResponseMode:   "form_post",
			UserInfoSource: OAuthUserInfoSourceIDToken,
		},
	}
	oauthUserInfoParsers = map[string]OAuthUserInfoParser{
		"google":    parseGoogleUser,
		"github":    parseGitHubUserPayload,
		"microsoft": parseMicrosoftUser,
	}
)

// defaultProviders stores the original provider configs for ResetProviderURLs.
var defaultProviders = map[string]OAuthProviderConfig{
	"google":    oauthProviders["google"],
	"github":    oauthProviders["github"],
	"microsoft": oauthProviders["microsoft"],
	"apple":     oauthProviders["apple"],
}

var defaultUserInfoParsers = map[string]OAuthUserInfoParser{
	"google":    parseGoogleUser,
	"github":    parseGitHubUserPayload,
	"microsoft": parseMicrosoftUser,
}

// SetProviderURLs overrides the URLs for a provider (for testing).
func SetProviderURLs(provider string, cfg OAuthProviderConfig) {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	oauthProviders[provider] = cfg
}

// ResetProviderURLs restores the default URLs for a provider.
func ResetProviderURLs(provider string) {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	if orig, ok := defaultProviders[provider]; ok {
		oauthProviders[provider] = orig
	}
}

// getProviderConfig returns the config for a provider, protected by the read lock.
func getProviderConfig(provider string) (OAuthProviderConfig, bool) {
	oauthMu.RLock()
	defer oauthMu.RUnlock()
	pc, ok := oauthProviders[provider]
	if !ok {
		return OAuthProviderConfig{}, false
	}
	return pc.withResolvedTemplates(), true
}

// GetProviderConfigRaw returns the global OAuth provider config without
// template resolution (for callers that need to preserve placeholders).
func GetProviderConfigRaw(provider string) (OAuthProviderConfig, bool) {
	oauthMu.RLock()
	defer oauthMu.RUnlock()
	pc, ok := oauthProviders[provider]
	if !ok {
		return OAuthProviderConfig{}, false
	}
	return pc, true
}

func getOAuthUserInfoParser(provider string) (OAuthUserInfoParser, bool) {
	oauthMu.RLock()
	defer oauthMu.RUnlock()
	parser, ok := oauthUserInfoParsers[provider]
	return parser, ok
}

// SetOAuthUserInfoParser registers a parser for a provider.
func SetOAuthUserInfoParser(provider string, parser OAuthUserInfoParser) {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	if parser == nil {
		delete(oauthUserInfoParsers, provider)
		return
	}
	oauthUserInfoParsers[provider] = parser
}

// ResetOAuthUserInfoParser restores the default parser for a provider.
func ResetOAuthUserInfoParser(provider string) {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	if parser, ok := defaultUserInfoParsers[provider]; ok {
		oauthUserInfoParsers[provider] = parser
		return
	}
	delete(oauthUserInfoParsers, provider)
}

// OAuthClientConfig holds the client credentials for one provider.
type OAuthClientConfig struct {
	ClientID     string
	ClientSecret string
}

// OAuthUserInfo is the normalized user info from an OAuth provider.
type OAuthUserInfo struct {
	ProviderUserID string
	Email          string
	Name           string
}

// OAuthStateStore manages CSRF state tokens with TTL-based expiry.
type OAuthStateStore struct {
	mu     sync.Mutex
	states map[string]time.Time
	ttl    time.Duration
}

// NewOAuthStateStore creates a state store with the given TTL.
func NewOAuthStateStore(ttl time.Duration) *OAuthStateStore {
	return &OAuthStateStore{
		states: make(map[string]time.Time),
		ttl:    ttl,
	}
}

// Generate creates a new cryptographic state token and stores it.
func (s *OAuthStateStore) Generate() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	// Prune expired entries opportunistically.
	now := time.Now()
	for k, exp := range s.states {
		if now.After(exp) {
			delete(s.states, k)
		}
	}
	s.states[token] = now.Add(s.ttl)
	return token, nil
}

// RegisterExternalState registers an externally-generated state (e.g. an SSE
// clientId) as a valid CSRF token with the store's TTL. This allows the OAuth
// callback to validate SSE client IDs the same way it validates self-generated
// state tokens.
func (s *OAuthStateStore) RegisterExternalState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = time.Now().Add(s.ttl)
}

// Validate checks and consumes a state token (one-time use).
func (s *OAuthStateStore) Validate(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.states[token]
	if !ok {
		return false
	}
	delete(s.states, token)
	return time.Now().Before(exp)
}

func (pc OAuthProviderConfig) tokenAuthMethod() OAuthTokenAuthMethod {
	if pc.TokenAuthMethod == "" {
		return OAuthTokenAuthMethodClientSecretPost
	}
	return pc.TokenAuthMethod
}

func (pc OAuthProviderConfig) userInfoSource() OAuthUserInfoSource {
	if pc.UserInfoSource == "" {
		return OAuthUserInfoSourceEndpoint
	}
	return pc.UserInfoSource
}

func (pc OAuthProviderConfig) tenantID() string {
	tenant := strings.TrimSpace(pc.TenantID)
	if tenant == "" {
		return "common"
	}
	return tenant
}

func (pc OAuthProviderConfig) withResolvedTemplates() OAuthProviderConfig {
	replaced := strings.NewReplacer("{tenant}", pc.tenantID())
	pc.AuthURL = replaced.Replace(pc.AuthURL)
	pc.TokenURL = replaced.Replace(pc.TokenURL)
	pc.UserInfoURL = replaced.Replace(pc.UserInfoURL)
	return pc
}

// Builds the authorization URL to redirect the user to the OAuth provider with the given client credentials, redirect URI, state token, and provider configuration including scopes and additional parameters.
func authorizationURLWithConfig(provider string, client OAuthClientConfig, redirectURI, state string, pc OAuthProviderConfig) (string, error) {
	pc = pc.withResolvedTemplates()

	params := url.Values{
		"client_id":     {client.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"state":         {state},
		"scope":         {strings.Join(pc.Scopes, " ")},
	}

	if provider == "google" {
		params.Set("access_type", "offline")
	}
	// OIDC providers should carry a nonce so callback verification can enforce
	// id_token nonce claim validation.
	if pc.DiscoveryURL != "" {
		params.Set("nonce", state)
	}
	if pc.ResponseMode != "" {
		params.Set("response_mode", pc.ResponseMode)
	}
	for key, value := range pc.AdditionalParams {
		if key == "" || value == "" {
			continue
		}
		params.Set(key, value)
	}

	return pc.AuthURL + "?" + params.Encode(), nil
}

// AuthorizationURL builds the URL to redirect the user to the OAuth provider.
func AuthorizationURL(provider string, client OAuthClientConfig, redirectURI, state string) (string, error) {
	pc, ok := getProviderConfig(provider)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrOAuthNotConfigured, provider)
	}
	return authorizationURLWithConfig(provider, client, redirectURI, state, pc)
}

// OAuthLogin finds or creates a user from an OAuth identity and returns
// the user with access + refresh tokens.
func (s *Service) OAuthLogin(ctx context.Context, provider string, info *OAuthUserInfo) (*User, string, string, error) {
	// 1. Check if this OAuth identity is already linked.
	var userID string
	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM _ayb_oauth_accounts
		 WHERE provider = $1 AND provider_user_id = $2`,
		provider, info.ProviderUserID,
	).Scan(&userID)

	if err == nil {
		// Existing link — login as that user.
		return s.loginByID(ctx, userID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", "", fmt.Errorf("querying OAuth account: %w", err)
	}

	// 2. No link. Check if a user with this email exists.
	if info.Email != "" {
		email := strings.ToLower(info.Email)
		err := s.pool.QueryRow(ctx,
			`SELECT id FROM _ayb_users WHERE LOWER(email) = $1`, email,
		).Scan(&userID)

		if err == nil {
			// Link this OAuth identity to the existing user.
			if err := s.linkOAuthAccount(ctx, userID, provider, info); err != nil {
				return nil, "", "", err
			}
			return s.loginByID(ctx, userID)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", fmt.Errorf("querying user by email: %w", err)
		}
	}

	// 3. Create a new user and link the OAuth account.
	email := strings.ToLower(info.Email)
	if email == "" {
		// Generate a placeholder email for users without email (rare).
		email = fmt.Sprintf("%s+%s@oauth.local", provider, info.ProviderUserID)
	}

	// Generate a random password hash (user can't login via email/password).
	randomPW := make([]byte, 32)
	if _, err := rand.Read(randomPW); err != nil {
		return nil, "", "", fmt.Errorf("generating random password: %w", err)
	}
	hash, err := hashPassword(base64.RawURLEncoding.EncodeToString(randomPW))
	if err != nil {
		return nil, "", "", fmt.Errorf("hashing placeholder password: %w", err)
	}

	var user User
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ($1, $2)
		 RETURNING id, email, is_anonymous, created_at, updated_at`,
		email, hash,
	).Scan(&user.ID, &user.Email, &user.IsAnonymous, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		// Handle race: another request might have created this user.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// User was created concurrently — find and link.
			err2 := s.pool.QueryRow(ctx,
				`SELECT id FROM _ayb_users WHERE LOWER(email) = $1`, email,
			).Scan(&userID)
			if err2 != nil {
				return nil, "", "", fmt.Errorf("querying user after conflict: %w", err2)
			}
			if err := s.linkOAuthAccount(ctx, userID, provider, info); err != nil {
				return nil, "", "", err
			}
			return s.loginByID(ctx, userID)
		}
		return nil, "", "", fmt.Errorf("inserting user: %w", err)
	}

	if err := s.linkOAuthAccount(ctx, user.ID, provider, info); err != nil {
		return nil, "", "", err
	}

	s.logger.Info("user registered via OAuth", "user_id", user.ID, "provider", provider)
	return s.issueTokens(ctx, &user)
}

func (s *Service) linkOAuthAccount(ctx context.Context, userID, provider string, info *OAuthUserInfo) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_oauth_accounts (user_id, provider, provider_user_id, email, name)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (provider, provider_user_id) DO NOTHING`,
		userID, provider, info.ProviderUserID, info.Email, info.Name,
	)
	if err != nil {
		return fmt.Errorf("linking OAuth account: %w", err)
	}
	return nil
}

// Logs in a user by ID, checking for MFA enrollment and returning either a pending MFA token for further authentication or full access and refresh tokens.
func (s *Service) loginByID(ctx context.Context, userID string) (*User, string, string, error) {
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	// If user has any MFA factor enrolled, return a pending token instead of full tokens.
	hasMFA, _, err := s.HasAnyMFA(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("checking MFA enrollment: %w", err)
	}
	if hasMFA {
		pendingToken, err := s.generateMFAPendingTokenWithMethod(user, "oauth")
		if err != nil {
			return nil, "", "", fmt.Errorf("generating MFA pending token: %w", err)
		}
		return user, pendingToken, "", nil
	}

	return s.issueTokens(ctx, user)
}

func (s *Service) issueTokens(ctx context.Context, user *User) (*User, string, string, error) {
	sessionID, refreshToken, err := s.createSession(ctx, user.ID, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("creating session: %w", err)
	}
	opts, err := s.sessionTokenOptions(ctx, user, &tokenOptions{SessionID: sessionID})
	if err != nil {
		return nil, "", "", fmt.Errorf("resolving session tenant: %w", err)
	}
	token, err := s.generateTokenWithOpts(ctx, user, opts)
	if err != nil {
		return nil, "", "", fmt.Errorf("generating token: %w", err)
	}
	return user, token, refreshToken, nil
}
