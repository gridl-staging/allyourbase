package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// OAuthProviderConfig holds the OAuth2 endpoints for a provider.
type OAuthProviderConfig struct {
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	Scopes      []string
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
	}
)

// defaultProviders stores the original provider configs for ResetProviderURLs.
var defaultProviders = map[string]OAuthProviderConfig{
	"google": oauthProviders["google"],
	"github": oauthProviders["github"],
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
	return pc, ok
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

// AuthorizationURL builds the URL to redirect the user to the OAuth provider.
func AuthorizationURL(provider string, client OAuthClientConfig, redirectURI, state string) (string, error) {
	pc, ok := getProviderConfig(provider)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrOAuthNotConfigured, provider)
	}

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

	return pc.AuthURL + "?" + params.Encode(), nil
}

// ExchangeCode exchanges an authorization code for provider tokens,
// then fetches user info from the provider.
func ExchangeCode(ctx context.Context, provider string, client OAuthClientConfig, code, redirectURI string) (*OAuthUserInfo, error) {
	pc, ok := getProviderConfig(provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrOAuthNotConfigured, provider)
	}
	return exchangeCode(ctx, provider, client, code, redirectURI, pc, oauthHTTPClient)
}

// exchangeCode is the unexported implementation of ExchangeCode. It accepts
// explicit dependencies (provider config and HTTP client) so that tests can
// exercise the full code path without touching package-level globals.
func exchangeCode(ctx context.Context, provider string, client OAuthClientConfig, code, redirectURI string, pc OAuthProviderConfig, httpClient *http.Client) (*OAuthUserInfo, error) {
	// Exchange code for access token.
	data := url.Values{
		"client_id":     {client.ClientID},
		"client_secret": {client.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pc.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOAuthCodeExchange, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: token endpoint returned %d", ErrOAuthCodeExchange, resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("%w: empty access token", ErrOAuthCodeExchange)
	}

	// Fetch user info.
	return fetchUserInfo(ctx, provider, pc.UserInfoURL, tokenResp.AccessToken, httpClient)
}

func fetchUserInfo(ctx context.Context, provider, userInfoURL, accessToken string, httpClient *http.Client) (*OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

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

	switch provider {
	case "google":
		return parseGoogleUser(body)
	case "github":
		return parseGitHubUser(ctx, body, accessToken, httpClient)
	default:
		return nil, fmt.Errorf("%w: %s", ErrOAuthNotConfigured, provider)
	}
}

func parseGoogleUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Google user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           u.Name,
	}, nil
}

func parseGitHubUser(ctx context.Context, body []byte, accessToken string, httpClient *http.Client) (*OAuthUserInfo, error) {
	var u struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing GitHub user: %w", err)
	}
	if u.ID == 0 {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}

	email := u.Email
	// GitHub users can have private emails — fetch from /user/emails.
	if email == "" {
		var err error
		email, err = fetchGitHubPrimaryEmail(ctx, accessToken, httpClient)
		if err != nil {
			// Non-fatal: proceed without email.
			email = ""
		}
	}

	name := u.Name
	if name == "" {
		name = u.Login
	}

	return &OAuthUserInfo{
		ProviderUserID: fmt.Sprintf("%d", u.ID),
		Email:          email,
		Name:           name,
	}, nil
}

func fetchGitHubPrimaryEmail(ctx context.Context, accessToken string, httpClient *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("emails endpoint returned %d", resp.StatusCode)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no primary verified email")
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
		 RETURNING id, email, created_at, updated_at`,
		email, hash,
	).Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
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

func (s *Service) loginByID(ctx context.Context, userID string) (*User, string, string, error) {
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	// If user has MFA enrolled, return a pending token instead of full tokens.
	hasMFA, err := s.HasSMSMFA(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("checking MFA enrollment: %w", err)
	}
	if hasMFA {
		pendingToken, err := s.generateMFAPendingToken(user)
		if err != nil {
			return nil, "", "", fmt.Errorf("generating MFA pending token: %w", err)
		}
		return user, pendingToken, "", nil
	}

	return s.issueTokens(ctx, user)
}

func (s *Service) issueTokens(ctx context.Context, user *User) (*User, string, string, error) {
	token, err := s.generateToken(user)
	if err != nil {
		return nil, "", "", fmt.Errorf("generating token: %w", err)
	}
	refreshToken, err := s.createSession(ctx, user.ID)
	if err != nil {
		return nil, "", "", fmt.Errorf("creating session: %w", err)
	}
	return user, token, refreshToken, nil
}
