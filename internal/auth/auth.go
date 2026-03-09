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
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/fbmigrate"
	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Sentinel errors returned by the auth service.
var (
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrEmailTaken          = errors.New("email already registered")
	ErrValidation          = errors.New("validation error")
	ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")
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
type Service struct {
	pool         *pgxpool.Pool
	jwtSecret    []byte
	jwtSecretMu  sync.RWMutex
	tokenDur     time.Duration
	refreshDur   time.Duration
	minPwLen     int // minimum password length (default 8)
	logger       *slog.Logger
	mailer       mailer.Mailer // nil = email features disabled
	appName      string        // used in email templates
	baseURL      string        // public base URL for action links
	magicLinkDur time.Duration // 0 = use default (10 min)
	smsProvider      sms.Provider  // nil = SMS features disabled
	smsConfig        sms.Config
	oauthProviderCfg OAuthProviderModeConfig
	emailTplSvc      EmailTemplateRenderer // nil = use legacy hardcoded templates
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

// legacyRenderFuncs maps template keys to their legacy render functions.
var legacyRenderFuncs = map[string]func(mailer.TemplateData) (string, string, error){
	"auth.password_reset":     mailer.RenderPasswordReset,
	"auth.email_verification": mailer.RenderVerification,
	"auth.magic_link":         mailer.RenderMagicLink,
}

// legacySubjects maps template keys to their default subjects.
var legacySubjects = map[string]string{
	"auth.password_reset":     mailer.DefaultPasswordResetSubject,
	"auth.email_verification": mailer.DefaultVerificationSubject,
	"auth.magic_link":         mailer.DefaultMagicLinkSubject,
}

// renderAuthEmail renders an email using the template service if available,
// falling back to the legacy built-in render functions.
func (s *Service) renderAuthEmail(ctx context.Context, key string, vars map[string]string) (subject, html, text string, err error) {
	if s.emailTplSvc != nil {
		return s.emailTplSvc.RenderWithFallback(ctx, key, vars)
	}
	// Legacy path: use hardcoded render functions.
	renderFn, ok := legacyRenderFuncs[key]
	if !ok {
		return "", "", "", fmt.Errorf("unknown auth email template key: %s", key)
	}
	data := mailer.TemplateData{
		AppName:   vars["AppName"],
		ActionURL: vars["ActionURL"],
	}
	html, text, err = renderFn(data)
	if err != nil {
		return "", "", "", err
	}
	return legacySubjects[key], html, text, nil
}

// User represents a registered user (without password hash).
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Claims are the JWT claims issued by AYB.
type Claims struct {
	jwt.RegisteredClaims
	Email         string   `json:"email"`
	APIKeyScope   string   `json:"apiKeyScope,omitempty"`   // "*", "readonly", "readwrite"; empty for JWT
	AllowedTables []string `json:"allowedTables,omitempty"` // empty = all tables
	AppID              string   `json:"appId,omitempty"`              // set when API key is app-scoped
	AppRateLimitRPS    int      `json:"appRateLimitRps,omitempty"`    // app's configured RPS limit (0 = unlimited)
	AppRateLimitWindow int      `json:"appRateLimitWindow,omitempty"` // app's rate limit window in seconds
	MFAPending         bool     `json:"mfa_pending,omitempty"`
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
	return &Service{
		pool:       pool,
		jwtSecret:  []byte(jwtSecret),
		tokenDur:   tokenDuration,
		refreshDur: refreshDuration,
		minPwLen:   minPasswordLength,
		logger:     logger,
	}
}

// Register creates a new user and returns the user, an access token, and a refresh token.
func (s *Service) Register(ctx context.Context, email, password string) (*User, string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := validateEmail(email); err != nil {
		return nil, "", "", err
	}
	if err := validatePassword(password, s.minPwLen); err != nil {
		return nil, "", "", err
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, "", "", fmt.Errorf("hashing password: %w", err)
	}

	var user User
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ($1, $2)
		 RETURNING id, email, created_at, updated_at`,
		email, hash,
	).Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, "", "", ErrEmailTaken
		}
		return nil, "", "", fmt.Errorf("inserting user: %w", err)
	}

	s.logger.Info("user registered", "user_id", user.ID, "email", user.Email)

	// Send verification email (best-effort, don't block registration).
	if s.mailer != nil {
		if err := s.SendVerificationEmail(ctx, user.ID, user.Email); err != nil {
			s.logger.Error("failed to send verification email on register", "error", err)
		}
	}

	return s.issueTokens(ctx, &user)
}

// Login authenticates a user and returns the user, an access token, and a refresh token.
func (s *Service) Login(ctx context.Context, email, password string) (*User, string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var user User
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, COALESCE(phone, ''), password_hash, created_at, updated_at
		 FROM _ayb_users WHERE LOWER(email) = $1`,
		email,
	).Scan(&user.ID, &user.Email, &user.Phone, &hash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", ErrInvalidCredentials
		}
		return nil, "", "", fmt.Errorf("querying user: %w", err)
	}

	ok, err := verifyPassword(hash, password)
	if err != nil {
		return nil, "", "", fmt.Errorf("verifying password: %w", err)
	}
	if !ok {
		return nil, "", "", ErrInvalidCredentials
	}

	// Progressive re-hash: upgrade bcrypt/firebase-scrypt hashes to argon2id on successful login.
	if isBcryptHash(hash) || strings.HasPrefix(hash, "$firebase-scrypt$") {
		if err := s.upgradePasswordHash(ctx, user.ID, password); err != nil {
			s.logger.Error("failed to upgrade password hash", "user_id", user.ID, "error", err)
		}
	}

	// If user has MFA enrolled, return a pending token instead of full tokens.
	hasMFA, err := s.HasSMSMFA(ctx, user.ID)
	if err != nil {
		return nil, "", "", fmt.Errorf("checking MFA enrollment: %w", err)
	}
	if hasMFA {
		pendingToken, err := s.generateMFAPendingToken(&user)
		if err != nil {
			return nil, "", "", fmt.Errorf("generating MFA pending token: %w", err)
		}
		return &user, pendingToken, "", nil
	}

	return s.issueTokens(ctx, &user)
}

// ValidateToken parses and validates a JWT token string.
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	s.jwtSecretMu.RLock()
	secret := s.jwtSecret
	s.jwtSecretMu.RUnlock()

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// UserByID fetches a user by ID.
func (s *Service) UserByID(ctx context.Context, id string) (*User, error) {
	var user User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, COALESCE(phone, ''), created_at, updated_at FROM _ayb_users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Email, &user.Phone, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("querying user: %w", err)
	}
	return &user, nil
}

func (s *Service) generateToken(user *User) (string, error) {
	now := time.Now()
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", fmt.Errorf("generating jti: %w", err)
	}
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenDur)),
			ID:        hex.EncodeToString(jti),
		},
		Email: user.Email,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s.jwtSecretMu.RLock()
	secret := s.jwtSecret
	s.jwtSecretMu.RUnlock()
	return token.SignedString(secret)
}

// IssueTestToken generates a JWT for the given user ID and email. Intended for testing.
func (s *Service) IssueTestToken(userID, email string) (string, error) {
	return s.generateToken(&User{ID: userID, Email: email})
}

// RotateJWTSecret generates a new random JWT secret, invalidating all existing tokens.
func (s *Service) RotateJWTSecret() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generating secret: %w", err)
	}
	hex := fmt.Sprintf("%x", secret)
	s.jwtSecretMu.Lock()
	s.jwtSecret = []byte(hex)
	s.jwtSecretMu.Unlock()
	return hex, nil
}

// hashPassword hashes a password using argon2id and returns a PHC-format string.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// verifyPassword checks a password against a stored hash.
// Supports argon2id (PHC format) and bcrypt ($2a$/$2b$/$2y$).
func verifyPassword(encoded, password string) (bool, error) {
	if isBcryptHash(encoded) {
		err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password))
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("bcrypt verify: %w", err)
		}
		return true, nil
	}

	if strings.HasPrefix(encoded, "$argon2id$") {
		return verifyArgon2id(encoded, password)
	}

	if strings.HasPrefix(encoded, "$firebase-scrypt$") {
		return verifyFirebaseScrypt(encoded, password)
	}

	return false, fmt.Errorf("unsupported hash format")
}

// isBcryptHash returns true if the hash string is a bcrypt hash.
func isBcryptHash(hash string) bool {
	return strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$")
}

// verifyArgon2id checks a password against a PHC-format argon2id hash.
func verifyArgon2id(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid argon2id hash format")
	}

	var memory uint32
	var iterations uint32
	var threads uint8
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads)
	if err != nil {
		return false, fmt.Errorf("parsing hash params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decoding salt: %w", err)
	}

	expectedKey, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decoding key: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(expectedKey)))
	return subtle.ConstantTimeCompare(key, expectedKey) == 1, nil
}

// verifyFirebaseScrypt checks a password against a Firebase modified-scrypt hash
// stored in AYB format: $firebase-scrypt$<signerKey>$<saltSep>$<salt>$<rounds>$<memCost>$<passwordHash>
// Uses fbmigrate for crypto routines — the scrypt code lives in fbmigrate because it's
// used both during migration (encoding hashes) and at login time (verification).
func verifyFirebaseScrypt(encoded, password string) (bool, error) {
	signerKey, saltSep, salt, passwordHash, rounds, memCost, err := fbmigrate.ParseFirebaseScryptHash(encoded)
	if err != nil {
		return false, fmt.Errorf("parsing firebase-scrypt hash: %w", err)
	}

	return fbmigrate.VerifyFirebaseScrypt(password, salt, passwordHash, signerKey, saltSep, rounds, memCost)
}

// upgradePasswordHash re-hashes the password with argon2id and updates the database.
// Called after successful bcrypt login to progressively migrate to the stronger algorithm.
func (s *Service) upgradePasswordHash(ctx context.Context, userID, password string) error {
	newHash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		newHash, userID,
	)
	if err != nil {
		return fmt.Errorf("updating password hash: %w", err)
	}
	s.logger.Info("upgraded password hash to argon2id", "user_id", userID)
	return nil
}

func validateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("%w: email is required", ErrValidation)
	}
	atIdx := strings.Index(email, "@")
	if atIdx < 1 {
		return fmt.Errorf("%w: invalid email format", ErrValidation)
	}
	domain := email[atIdx+1:]
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("%w: invalid email format", ErrValidation)
	}
	return nil
}

func validatePassword(password string, minLen int) error {
	if len(password) == 0 {
		return fmt.Errorf("%w: password is required", ErrValidation)
	}
	if minLen < 1 {
		minLen = 8
	}
	if len(password) < minLen {
		return fmt.Errorf("%w: password must be at least %d characters", ErrValidation, minLen)
	}
	return nil
}

// RefreshToken validates a refresh token, rotates it, and returns the user
// with a new access token and refresh token.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*User, string, string, error) {
	hash := hashToken(refreshToken)

	var sessionID, userID string
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id FROM _ayb_sessions
		 WHERE token_hash = $1 AND expires_at > NOW()`,
		hash,
	).Scan(&sessionID, &userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", ErrInvalidRefreshToken
		}
		return nil, "", "", fmt.Errorf("querying session: %w", err)
	}

	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	// Rotate: generate new refresh token and update the session row.
	raw := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", "", fmt.Errorf("generating refresh token: %w", err)
	}
	newPlaintext := base64.RawURLEncoding.EncodeToString(raw)
	newHash := hashToken(newPlaintext)

	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_sessions SET token_hash = $1, expires_at = $2 WHERE id = $3`,
		newHash, time.Now().Add(s.refreshDur), sessionID,
	)
	if err != nil {
		return nil, "", "", fmt.Errorf("rotating session: %w", err)
	}

	accessToken, err := s.generateToken(user)
	if err != nil {
		return nil, "", "", fmt.Errorf("generating token: %w", err)
	}

	return user, accessToken, newPlaintext, nil
}

// Logout revokes a refresh token by deleting its session.
// Idempotent — returns nil even if the token doesn't match any session.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	hash := hashToken(refreshToken)
	_, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_sessions WHERE token_hash = $1`, hash,
	)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

const refreshTokenBytes = 32

// CreateUser creates a user without issuing tokens.
// Used by CLI commands that need to bootstrap users before the server starts.
func CreateUser(ctx context.Context, pool *pgxpool.Pool, email, password string, minPasswordLength int) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	if err := validatePassword(password, minPasswordLength); err != nil {
		return nil, err
	}

	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	var user User
	err = pool.QueryRow(ctx,
		`INSERT INTO _ayb_users (email, password_hash) VALUES ($1, $2)
		 RETURNING id, email, created_at, updated_at`,
		email, hash,
	).Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("inserting user: %w", err)
	}
	return &user, nil
}

// SetMailer configures the mailer for email-based auth flows.
func (s *Service) SetMailer(m mailer.Mailer, appName, baseURL string) {
	s.mailer = m
	s.appName = appName
	if appName == "" {
		s.appName = "Allyourbase"
	}
	s.baseURL = strings.TrimRight(baseURL, "/")
}

// SetSMSProvider sets the SMS provider for phone-based auth flows.
func (s *Service) SetSMSProvider(p sms.Provider) {
	s.smsProvider = p
}

// SetSMSConfig sets the SMS configuration.
func (s *Service) SetSMSConfig(c sms.Config) {
	s.smsConfig = c
}

// DB returns the database pool (needed by integration tests).
func (s *Service) DB() *pgxpool.Pool {
	return s.pool
}

const (
	resetTokenBytes   = 32
	resetTokenExpiry  = 1 * time.Hour
	verifyTokenBytes  = 32
	verifyTokenExpiry = 24 * time.Hour
)

// RequestPasswordReset generates a reset token and emails it to the user.
// Always returns nil to prevent email enumeration — caller should always return 200.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	if s.mailer == nil {
		return nil
	}
	email = strings.ToLower(strings.TrimSpace(email))

	var userID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM _ayb_users WHERE LOWER(email) = $1`, email,
	).Scan(&userID)
	if err != nil {
		// User not found — return nil to prevent enumeration.
		return nil
	}

	// Delete any existing reset tokens for this user.
	_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_password_resets WHERE user_id = $1`, userID)

	// Generate token.
	raw := make([]byte, resetTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("generating reset token: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(plaintext)

	_, err = s.pool.Exec(ctx,
		`INSERT INTO _ayb_password_resets (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, hash, time.Now().Add(resetTokenExpiry),
	)
	if err != nil {
		return fmt.Errorf("inserting reset token: %w", err)
	}

	actionURL := s.baseURL + "/auth/password-reset/confirm?token=" + plaintext
	vars := map[string]string{"AppName": s.appName, "ActionURL": actionURL}
	subject, html, text, err := s.renderAuthEmail(ctx, "auth.password_reset", vars)
	if err != nil {
		return fmt.Errorf("rendering reset email: %w", err)
	}

	if err := s.mailer.Send(ctx, &mailer.Message{
		To:      email,
		Subject: subject,
		HTML:    html,
		Text:    text,
	}); err != nil {
		s.logger.Error("failed to send password reset email", "error", err, "email", email)
	}
	return nil
}

// ConfirmPasswordReset validates the token and sets a new password.
func (s *Service) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	if err := validatePassword(newPassword, s.minPwLen); err != nil {
		return err
	}

	hash := hashToken(token)

	var userID string
	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM _ayb_password_resets
		 WHERE token_hash = $1 AND expires_at > NOW()`,
		hash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidResetToken
		}
		return fmt.Errorf("querying reset token: %w", err)
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing new password: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		newHash, userID,
	)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}

	// Delete all reset tokens for this user.
	if _, err := s.pool.Exec(ctx, `DELETE FROM _ayb_password_resets WHERE user_id = $1`, userID); err != nil {
		s.logger.Error("failed to delete reset tokens after password reset", "user_id", userID, "error", err)
	}

	// Invalidate all existing sessions (force re-login).
	if _, err := s.pool.Exec(ctx, `DELETE FROM _ayb_sessions WHERE user_id = $1`, userID); err != nil {
		s.logger.Error("failed to invalidate sessions after password reset", "user_id", userID, "error", err)
		return fmt.Errorf("invalidating sessions: %w", err)
	}

	s.logger.Info("password reset completed", "user_id", userID)
	return nil
}

// SendVerificationEmail generates a verification token and emails it.
func (s *Service) SendVerificationEmail(ctx context.Context, userID, email string) error {
	if s.mailer == nil {
		return nil
	}

	// Delete any existing verification tokens for this user.
	_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_email_verifications WHERE user_id = $1`, userID)

	raw := make([]byte, verifyTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("generating verification token: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(plaintext)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_email_verifications (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, hash, time.Now().Add(verifyTokenExpiry),
	)
	if err != nil {
		return fmt.Errorf("inserting verification token: %w", err)
	}

	actionURL := s.baseURL + "/auth/verify?token=" + plaintext
	vars := map[string]string{"AppName": s.appName, "ActionURL": actionURL}
	subject, html, text, err := s.renderAuthEmail(ctx, "auth.email_verification", vars)
	if err != nil {
		return fmt.Errorf("rendering verification email: %w", err)
	}

	if err := s.mailer.Send(ctx, &mailer.Message{
		To:      email,
		Subject: subject,
		HTML:    html,
		Text:    text,
	}); err != nil {
		s.logger.Error("failed to send verification email", "error", err, "email", email)
	}
	return nil
}

// ConfirmEmail validates the verification token and marks the user's email as verified.
func (s *Service) ConfirmEmail(ctx context.Context, token string) error {
	hash := hashToken(token)

	var userID string
	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM _ayb_email_verifications
		 WHERE token_hash = $1 AND expires_at > NOW()`,
		hash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidVerifyToken
		}
		return fmt.Errorf("querying verification token: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_users SET email_verified = true, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("updating email_verified: %w", err)
	}

	// Delete all verification tokens for this user.
	_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_email_verifications WHERE user_id = $1`, userID)

	s.logger.Info("email verified", "user_id", userID)
	return nil
}

// AdminUser is a user record with additional fields visible only to admins.
type AdminUser struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"emailVerified"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// UserListResult is a paginated list of admin users.
type UserListResult struct {
	Items      []AdminUser `json:"items"`
	Page       int         `json:"page"`
	PerPage    int         `json:"perPage"`
	TotalItems int         `json:"totalItems"`
	TotalPages int         `json:"totalPages"`
}

// ListUsers returns a paginated list of users (admin-only).
func (s *Service) ListUsers(ctx context.Context, page, perPage int, search string) (*UserListResult, error) {
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
	var rows []AdminUser

	if search != "" {
		pattern := "%" + search + "%"
		err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM _ayb_users WHERE email ILIKE $1`, pattern,
		).Scan(&totalItems)
		if err != nil {
			return nil, fmt.Errorf("counting users: %w", err)
		}

		dbRows, err := s.pool.Query(ctx,
			`SELECT id, email, email_verified, created_at, updated_at
			 FROM _ayb_users WHERE email ILIKE $1
			 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			pattern, perPage, offset,
		)
		if err != nil {
			return nil, fmt.Errorf("querying users: %w", err)
		}
		defer dbRows.Close()

		for dbRows.Next() {
			var u AdminUser
			if err := dbRows.Scan(&u.ID, &u.Email, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt); err != nil {
				return nil, fmt.Errorf("scanning user: %w", err)
			}
			rows = append(rows, u)
		}
		if err := dbRows.Err(); err != nil {
			return nil, fmt.Errorf("iterating users: %w", err)
		}
	} else {
		err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM _ayb_users`,
		).Scan(&totalItems)
		if err != nil {
			return nil, fmt.Errorf("counting users: %w", err)
		}

		dbRows, err := s.pool.Query(ctx,
			`SELECT id, email, email_verified, created_at, updated_at
			 FROM _ayb_users
			 ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
			perPage, offset,
		)
		if err != nil {
			return nil, fmt.Errorf("querying users: %w", err)
		}
		defer dbRows.Close()

		for dbRows.Next() {
			var u AdminUser
			if err := dbRows.Scan(&u.ID, &u.Email, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt); err != nil {
				return nil, fmt.Errorf("scanning user: %w", err)
			}
			rows = append(rows, u)
		}
		if err := dbRows.Err(); err != nil {
			return nil, fmt.Errorf("iterating users: %w", err)
		}
	}

	if rows == nil {
		rows = []AdminUser{}
	}

	totalPages := totalItems / perPage
	if totalItems%perPage != 0 {
		totalPages++
	}

	return &UserListResult{
		Items:      rows,
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}, nil
}

// DeleteUser removes a user by ID, including all their sessions, apps, and
// app-scoped API keys. The _ayb_apps FK uses ON DELETE CASCADE from the user,
// but _ayb_api_keys.app_id uses ON DELETE RESTRICT to prevent silent privilege
// escalation. We must detach keys from the user's apps before the cascade can
// proceed.
func (s *Service) DeleteUser(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Revoke active app-scoped keys and detach all keys from the user's apps.
	// This satisfies the ON DELETE RESTRICT FK on api_keys.app_id so that the
	// subsequent CASCADE delete of _ayb_apps rows can succeed.
	_, err = tx.Exec(ctx,
		`UPDATE _ayb_api_keys
		 SET revoked_at = COALESCE(revoked_at, NOW()), app_id = NULL
		 WHERE app_id IN (SELECT id FROM _ayb_apps WHERE owner_user_id = $1)`, id)
	if err != nil {
		return fmt.Errorf("detaching app keys before user delete: %w", err)
	}

	result, err := tx.Exec(ctx,
		`DELETE FROM _ayb_users WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing user delete: %w", err)
	}

	s.logger.Info("user deleted by admin", "user_id", id)
	return nil
}

// hashToken hashes a plaintext token with SHA-256 for storage.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Service) createSession(ctx context.Context, userID string) (string, error) {
	raw := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(plaintext)

	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_sessions (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, hash, time.Now().Add(s.refreshDur),
	)
	if err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}
	return plaintext, nil
}
