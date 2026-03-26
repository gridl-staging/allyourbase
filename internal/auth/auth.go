// Package auth provides user registration, authentication, JWT token management, password reset, email verification, OAuth integration, and multi-factor authentication.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors returned by the auth service.
var (
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrEmailTaken          = errors.New("email already registered")
	ErrValidation          = errors.New("validation error")
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
	ErrTokenRevoked        = errors.New("token revoked")
	ErrInvalidResetToken   = errors.New("invalid or expired reset token")
	ErrInvalidVerifyToken  = errors.New("invalid or expired verification token")
	ErrUserNotFound        = errors.New("user not found")
	ErrDailyLimitExceeded  = errors.New("daily SMS limit exceeded")
	ErrInvalidSMSCode      = errors.New("invalid or expired SMS code")
	ErrInvalidPhoneNumber  = sms.ErrInvalidPhoneNumber
)

// argon2id parameters. Vars (not consts) so tests can lower them for speed.
var (
	argonMemory  uint32 = 64 * 1024 // 64 MiB
	argonTime    uint32 = 3
	argonThreads uint8  = 2
)

const (
	argonSaltLen = 16
	argonKeyLen  = 32
)

// Service handles user registration, login, and JWT operations.
// Service handles user authentication operations including registration, login, password reset, email verification, OAuth flows, and multi-factor authentication. It manages JWT token generation and validation, session lifecycle, and integrates with optional email and SMS providers for delivering authentication messages.
type Service struct {
	pool                  *pgxpool.Pool
	jwtSecret             []byte
	jwtSecretMu           sync.RWMutex
	tokenDur              time.Duration
	refreshDur            time.Duration
	minPwLen              int // minimum password length (default 8)
	logger                *slog.Logger
	mailer                mailer.Mailer // nil = email features disabled
	appName               string        // used in email templates
	baseURL               string        // public base URL for action links
	magicLinkDur          time.Duration // 0 = use default (10 min)
	smsProvider           sms.Provider  // nil = SMS features disabled
	smsConfig             sms.Config
	oauthProviderCfg      OAuthProviderModeConfig
	providerTokenStore    ProviderTokenStorage
	providerTokenResolver ProviderTokenOAuthResolver
	oauthIdentityStore    oauthIdentityStore
	emailTplSvc           EmailTemplateRenderer // nil = use legacy hardcoded templates
	encryptionKey         []byte                // AES-256-GCM key for encrypting TOTP secrets (32 bytes)
	mfaFailureTracker     *mfaFailureTracker    // cumulative MFA failure lockout tracker
	hookDispatcher        *HookDispatcher
	denyList              *TokenDenyList
	activityTracker       *SessionActivityTracker
}

// EmailTemplateRenderer renders email templates by key with variable substitution.
// When set on auth.Service, email flows use custom templates with fallback to built-in defaults.
type EmailTemplateRenderer interface {
	// RenderWithFallback renders a template for the given key. If the custom
	// template fails (parse error, timeout, missing var), it falls back to
	// the built-in default. Returns error only if the built-in also fails.
	RenderWithFallback(ctx context.Context, key string, vars map[string]string) (subject, html, text string, err error)
}

// SetEmailTemplateService wires the template service for customizable email flows.
func (s *Service) SetEmailTemplateService(svc EmailTemplateRenderer) {
	s.emailTplSvc = svc
}

// User represents a registered user (without password hash).
type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Phone       string     `json:"phone,omitempty"`
	IsAnonymous bool       `json:"is_anonymous,omitempty"`
	LinkedAt    *time.Time `json:"linked_at,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// Claims are the JWT claims issued by AYB.
// Claims represent the JWT claims issued by AYB, including user identification, email, session ID, API key scope and permissions, multi-factor authentication status, and authentication method references for tracking how the user authenticated. Custom claims can be injected by hooks.
type Claims struct {
	jwt.RegisteredClaims
	Email              string         `json:"email"`
	SessionID          string         `json:"sid,omitempty"`
	APIKeyScope        string         `json:"apiKeyScope,omitempty"` // "*", "readonly", "readwrite"; empty for JWT
	APIKeyID           string         `json:"apiKeyId,omitempty"`
	AllowedTables      []string       `json:"allowedTables,omitempty"` // empty = all tables
	AppID              string         `json:"appId,omitempty"`         // set when API key is app-scoped
	OrgID              string         `json:"orgId,omitempty"`         // set when API key is org-scoped
	TenantID           string         `json:"tenantId,omitempty"`
	AppRateLimitRPS    int            `json:"appRateLimitRps,omitempty"`    // app's configured RPS limit (0 = unlimited)
	AppRateLimitWindow int            `json:"appRateLimitWindow,omitempty"` // app's rate limit window in seconds
	MFAPending         bool           `json:"mfa_pending,omitempty"`
	IsAnonymous        bool           `json:"is_anonymous,omitempty"`
	AAL                string         `json:"aal,omitempty"` // "aal1" or "aal2"
	AMR                []string       `json:"amr,omitempty"` // authentication method references, e.g. ["password", "totp"]
	CustomClaims       map[string]any `json:"custom_claims,omitempty"`
}

// API key scope constants.
const (
	ScopeFullAccess = "*"
	ScopeReadOnly   = "readonly"
	ScopeReadWrite  = "readwrite"
)

// ValidScopes is the set of valid API key scopes.
var ValidScopes = map[string]bool{
	ScopeFullAccess: true,
	ScopeReadOnly:   true,
	ScopeReadWrite:  true,
}

// IsReadAllowed returns true if the scope permits read operations.
func (c *Claims) IsReadAllowed() bool {
	return c.APIKeyScope == "" || ValidScopes[c.APIKeyScope]
}

// IsWriteAllowed returns true if the scope permits write operations (create, update, delete).
func (c *Claims) IsWriteAllowed() bool {
	s := c.APIKeyScope
	return s == "" || s == ScopeFullAccess || s == ScopeReadWrite
}

// IsTableAllowed returns true if the scope permits access to the given table.
func (c *Claims) IsTableAllowed(table string) bool {
	if len(c.AllowedTables) == 0 {
		return true
	}
	for _, t := range c.AllowedTables {
		if t == table {
			return true
		}
	}
	return false
}

// NewService creates a new auth service.
func NewService(pool *pgxpool.Pool, jwtSecret string, tokenDuration, refreshDuration time.Duration, minPasswordLength int, logger *slog.Logger) *Service {
	if minPasswordLength < 1 {
		minPasswordLength = 8
	}
	svc := &Service{
		pool:       pool,
		jwtSecret:  []byte(jwtSecret),
		tokenDur:   tokenDuration,
		refreshDur: refreshDuration,
		minPwLen:   minPasswordLength,
		logger:     logger,
		oauthIdentityStore: &dbOAuthIdentityStore{
			pool: pool,
		},
		mfaFailureTracker: newMFAFailureTracker(emailMFALockoutCount, emailMFALockoutWindow, emailMFALockoutDuration),
		denyList:          NewTokenDenyList(),
	}
	if pool != nil {
		svc.activityTracker = NewSessionActivityTracker(pool, defaultSessionActivityDebounce, logger)
	}
	return svc
}

// UserByID fetches a user by ID.
func (s *Service) UserByID(ctx context.Context, id string) (*User, error) {
	var user User
	err := s.pool.QueryRow(ctx,
		`SELECT id, COALESCE(email, ''), COALESCE(phone, ''), is_anonymous, linked_at, created_at, updated_at
		 FROM _ayb_users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Email, &user.Phone, &user.IsAnonymous, &user.LinkedAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("querying user: %w", err)
	}
	return &user, nil
}

// DB returns the database pool (needed by integration tests).
func (s *Service) DB() *pgxpool.Pool {
	return s.pool
}
