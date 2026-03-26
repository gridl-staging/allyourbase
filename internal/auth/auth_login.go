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
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/fbmigrate"
	"github.com/allyourbase/ayb/internal/observability"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Login authenticates a user and returns the user, an access token, and a refresh token.
func (s *Service) Login(ctx context.Context, email, password string) (*User, string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	ctx, span := otel.Tracer("ayb/auth").Start(ctx, "auth.login",
		trace.WithAttributes(semconv.EnduserID(email)),
	)
	defer span.End()
	if s.pool == nil {
		err := errors.New("database pool is not configured")
		observability.RecordSpanError(span, err)
		return nil, "", "", err
	}

	var user User
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, COALESCE(email, ''), COALESCE(phone, ''), password_hash, is_anonymous, created_at, updated_at
		 FROM _ayb_users WHERE LOWER(email) = $1`,
		email,
	).Scan(&user.ID, &user.Email, &user.Phone, &hash, &user.IsAnonymous, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			observability.RecordSpanError(span, ErrInvalidCredentials)
			return nil, "", "", ErrInvalidCredentials
		}
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("querying user: %w", err)
	}

	ok, err := verifyPassword(hash, password)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("verifying password: %w", err)
	}
	if !ok {
		observability.RecordSpanError(span, ErrInvalidCredentials)
		return nil, "", "", ErrInvalidCredentials
	}

	// Progressive re-hash: upgrade bcrypt/firebase-scrypt hashes to argon2id on successful login.
	// Non-fatal: log the error but do not mark the span as failed — login still succeeds.
	if isBcryptHash(hash) || strings.HasPrefix(hash, "$firebase-scrypt$") {
		if err := s.upgradePasswordHash(ctx, user.ID, password); err != nil {
			s.logger.Error("failed to upgrade password hash", "user_id", user.ID, "error", err)
		}
	}

	// If user has any MFA factor enrolled, return a pending token instead of full tokens.
	hasMFA, _, err := s.HasAnyMFA(ctx, user.ID)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("checking MFA enrollment: %w", err)
	}
	if hasMFA {
		pendingToken, err := s.generateMFAPendingTokenWithMethod(&user, "password")
		if err != nil {
			observability.RecordSpanError(span, err)
			return nil, "", "", fmt.Errorf("generating MFA pending token: %w", err)
		}
		return &user, pendingToken, "", nil
	}

	userOut, accessToken, refreshToken, err := s.issueTokens(ctx, &user)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", err
	}
	return userOut, accessToken, refreshToken, nil
}

// RefreshToken validates a refresh token, rotates it, and returns the user
// with a new access token and refresh token.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*User, string, string, error) {
	ctx, span := otel.Tracer("ayb/auth").Start(ctx, "auth.refresh_token")
	defer span.End()
	hash := hashToken(refreshToken)

	var sessionID, userID, sessionAAL, sessionAMR string
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, COALESCE(aal, 'aal1'), COALESCE(amr, '')
		 FROM _ayb_sessions
		 WHERE token_hash = $1 AND expires_at > NOW()`,
		hash,
	).Scan(&sessionID, &userID, &sessionAAL, &sessionAMR)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			observability.RecordSpanError(span, ErrInvalidRefreshToken)
			return nil, "", "", ErrInvalidRefreshToken
		}
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("querying session: %w", err)
	}

	user, err := s.UserByID(ctx, userID)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	// Rotate: generate new refresh token and update the session row.
	raw := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("generating refresh token: %w", err)
	}
	newPlaintext := base64.RawURLEncoding.EncodeToString(raw)
	newHash := hashToken(newPlaintext)
	userAgent, ipAddress := requestMetadataFromContext(ctx)

	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_sessions
		 SET token_hash = $1,
		     expires_at = $2,
		     user_agent = COALESCE($4, user_agent),
		     ip_address = COALESCE($5, ip_address),
		     last_active_at = NOW()
		 WHERE id = $3`,
		newHash,
		time.Now().Add(s.refreshDur),
		sessionID,
		nullableString(userAgent),
		nullableString(ipAddress),
	)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("rotating session: %w", err)
	}

	// Preserve AAL/AMR from the session — refresh never elevates or downgrades.
	opts := refreshTokenOptions(sessionAAL, sessionAMR)
	if opts == nil {
		opts = &tokenOptions{}
	}
	opts.SessionID = sessionID

	opts, err = s.sessionTokenOptions(ctx, user, opts)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("resolving session tenant: %w", err)
	}

	accessToken, err := s.generateTokenWithOpts(ctx, user, opts)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, "", "", fmt.Errorf("generating token: %w", err)
	}

	return user, accessToken, newPlaintext, nil
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

// refreshTokenOptions reconstructs token options from session authentication assurance level and method strings. It parses the comma-separated authentication methods and returns nil if no extra auth context is present, indicating the default token generation path should be used.
func refreshTokenOptions(sessionAAL, sessionAMR string) *tokenOptions {
	parsedAMR := make([]string, 0, 2)
	if sessionAMR != "" {
		for _, method := range strings.Split(sessionAMR, ",") {
			method = strings.TrimSpace(method)
			if method != "" {
				parsedAMR = append(parsedAMR, method)
			}
		}
	}

	// Keep default claims generation path when session carries no extra auth context.
	if (sessionAAL == "" || sessionAAL == "aal1") && len(parsedAMR) == 0 {
		return nil
	}

	opts := &tokenOptions{AAL: sessionAAL}
	if opts.AAL == "" {
		opts.AAL = "aal1"
	}
	if len(parsedAMR) > 0 {
		opts.AMR = parsedAMR
	}

	return opts
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

// createSession creates a session record for the user with optional authentication context, generating a new refresh token and storing its hash in the database. It records the user agent and IP address from the request context and returns the session ID and plaintext refresh token.
func (s *Service) createSession(ctx context.Context, userID string, opts *tokenOptions) (string, string, error) {
	raw := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generating refresh token: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(plaintext)
	userAgent, ipAddress := requestMetadataFromContext(ctx)

	aal := "aal1"
	amr := ""
	if opts != nil {
		if opts.AAL != "" {
			aal = opts.AAL
		}
		if len(opts.AMR) > 0 {
			amr = strings.Join(opts.AMR, ",")
		}
	}

	var sessionID string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_sessions (user_id, token_hash, expires_at, aal, amr, user_agent, ip_address, last_active_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		 RETURNING id`,
		userID,
		hash,
		time.Now().Add(s.refreshDur),
		aal,
		amr,
		nullableString(userAgent),
		nullableString(ipAddress),
	).Scan(&sessionID)
	if err != nil {
		return "", "", fmt.Errorf("inserting session: %w", err)
	}
	return sessionID, plaintext, nil
}

// hashToken hashes a plaintext token with SHA-256 for storage.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

const refreshTokenBytes = 32
