// Package auth Provides TOTP multi-factor authentication (RFC 6238) with enrollment, verification, and utilities for managing MFA factors using AES-256-GCM encryption and replay protection.
package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// TOTP configuration constants (RFC 6238, Google Authenticator compatible).
const (
	totpDigits = 6
	totpPeriod = 30 // seconds
	totpSkew   = 1  // accept ±1 adjacent time window
	totpKeyLen = 20 // bytes (160-bit HMAC-SHA1 key)
)

// TOTP sentinel errors.
var (
	ErrTOTPAlreadyEnrolled   = errors.New("TOTP MFA already enrolled")
	ErrTOTPNotEnrolled       = errors.New("no TOTP factor found")
	ErrTOTPInvalidCode       = errors.New("invalid TOTP code")
	ErrTOTPReplay            = errors.New("TOTP code already used")
	ErrTOTPChallengeNotFound = errors.New("MFA challenge not found or expired")
	ErrTOTPChallengeUsed     = errors.New("MFA challenge already verified")
	ErrEncryptionKeyNotSet   = errors.New("encryption key not configured")
)

// SetEncryptionKey sets the AES-256-GCM key used for encrypting TOTP secrets.
// Key must be exactly 32 bytes.
func (s *Service) SetEncryptionKey(key []byte) error {
	if len(key) != 32 {
		return fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}
	s.encryptionKey = make([]byte, 32)
	copy(s.encryptionKey, key)
	return nil
}

// encryptAESGCM encrypts plaintext using AES-256-GCM with the service encryption key.
func (s *Service) encryptAESGCM(plaintext []byte) ([]byte, error) {
	if len(s.encryptionKey) == 0 {
		return nil, ErrEncryptionKeyNotSet
	}
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decryptAESGCM decrypts ciphertext using AES-256-GCM with the service encryption key.
func (s *Service) decryptAESGCM(ciphertext []byte) ([]byte, error) {
	if len(s.encryptionKey) == 0 {
		return nil, ErrEncryptionKeyNotSet
	}
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
}

// generateTOTPCode computes the TOTP value for a secret at a given time step.
// Implements RFC 6238 (TOTP) using HMAC-SHA1, 6 digits, 30-second period.
func generateTOTPCode(secret []byte, timeStep int64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(timeStep))

	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	hash := mac.Sum(nil)

	// Dynamic truncation (RFC 4226 §5.4).
	offset := hash[len(hash)-1] & 0x0f
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code = code % uint32(math.Pow10(totpDigits))

	return fmt.Sprintf("%0*d", totpDigits, code)
}

// validateTOTPCode checks if the provided code matches the secret within the
// allowed time skew. Returns the matched time step, or -1 if no match.
func validateTOTPCode(secret []byte, code string, now time.Time) (int64, bool) {
	currentStep := now.Unix() / totpPeriod
	for offset := -int64(totpSkew); offset <= int64(totpSkew); offset++ {
		step := currentStep + offset
		expected := generateTOTPCode(secret, step)
		if subtle.ConstantTimeCompare([]byte(code), []byte(expected)) == 1 {
			return step, true
		}
	}
	return -1, false
}

// buildOTPAuthURI builds a standard otpauth:// URI for authenticator app enrollment.
func buildOTPAuthURI(secret []byte, email, issuer string) string {
	b32 := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	label := url.PathEscape(issuer + ":" + email)
	params := url.Values{}
	params.Set("secret", b32)
	params.Set("issuer", issuer)
	params.Set("algorithm", "SHA1")
	params.Set("digits", fmt.Sprintf("%d", totpDigits))
	params.Set("period", fmt.Sprintf("%d", totpPeriod))
	return fmt.Sprintf("otpauth://totp/%s?%s", label, params.Encode())
}

// TOTPEnrollment holds the data returned to the client during TOTP enrollment.
type TOTPEnrollment struct {
	FactorID string `json:"factor_id"`
	URI      string `json:"uri"`
	Secret   string `json:"secret"` // base32-encoded, shown once
}

// EnrollTOTP starts TOTP enrollment for a user. Generates a secret, encrypts
// and stores it as an unverified factor. Returns enrollment data for the client.
func (s *Service) EnrollTOTP(ctx context.Context, userID, email, issuer string) (*TOTPEnrollment, error) {
	if s.pool == nil {
		return nil, errors.New("database pool is not configured")
	}
	if len(s.encryptionKey) == 0 {
		return nil, ErrEncryptionKeyNotSet
	}

	// Check for existing verified TOTP factor.
	var existing bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'totp' AND enabled = true)`,
		userID,
	).Scan(&existing)
	if err != nil {
		return nil, fmt.Errorf("checking TOTP enrollment: %w", err)
	}
	if existing {
		return nil, ErrTOTPAlreadyEnrolled
	}

	// Generate random TOTP secret.
	secret := make([]byte, totpKeyLen)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return nil, fmt.Errorf("generating TOTP secret: %w", err)
	}

	// Encrypt the secret for storage.
	encrypted, err := s.encryptAESGCM(secret)
	if err != nil {
		return nil, fmt.Errorf("encrypting TOTP secret: %w", err)
	}

	// Upsert: replace any existing unverified enrollment.
	var factorID string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_user_mfa (user_id, method, totp_secret_enc, enabled)
		 VALUES ($1, 'totp', $2, false)
		 ON CONFLICT (user_id, method) DO UPDATE
		 SET totp_secret_enc = $2, enabled = false, totp_enrolled_at = NULL, last_used_step = 0
		 RETURNING id`,
		userID, encrypted,
	).Scan(&factorID)
	if err != nil {
		return nil, fmt.Errorf("inserting TOTP enrollment: %w", err)
	}

	b32Secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	uri := buildOTPAuthURI(secret, email, issuer)

	s.logger.Info("TOTP enrollment started", "user_id", userID, "factor_id", factorID)
	return &TOTPEnrollment{
		FactorID: factorID,
		URI:      uri,
		Secret:   b32Secret,
	}, nil
}

// ConfirmTOTPEnrollment verifies the user's first TOTP code and activates the factor.
func (s *Service) ConfirmTOTPEnrollment(ctx context.Context, userID, code string) error {
	if s.pool == nil {
		return errors.New("database pool is not configured")
	}

	// Load the unverified factor.
	var factorID string
	var secretEnc []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, totp_secret_enc FROM _ayb_user_mfa
		 WHERE user_id = $1 AND method = 'totp' AND enabled = false`,
		userID,
	).Scan(&factorID, &secretEnc)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTOTPNotEnrolled
		}
		return fmt.Errorf("loading TOTP factor: %w", err)
	}

	secret, err := s.decryptAESGCM(secretEnc)
	if err != nil {
		return fmt.Errorf("decrypting TOTP secret: %w", err)
	}

	// Validate the code.
	_, ok := validateTOTPCode(secret, code, time.Now())
	if !ok {
		return ErrTOTPInvalidCode
	}

	// Activate the factor.
	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_user_mfa SET enabled = true, enrolled_at = NOW(), totp_enrolled_at = NOW()
		 WHERE id = $1`,
		factorID,
	)
	if err != nil {
		return fmt.Errorf("confirming TOTP enrollment: %w", err)
	}

	s.logger.Info("TOTP enrollment confirmed", "user_id", userID, "factor_id", factorID)
	return nil
}

// CreateTOTPChallenge creates a challenge record for the TOTP factor.
// Returns the challenge ID.
func (s *Service) CreateTOTPChallenge(ctx context.Context, userID, ipAddress string) (string, error) {
	if s.pool == nil {
		return "", errors.New("database pool is not configured")
	}

	// Find the user's enabled TOTP factor.
	var factorID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'totp' AND enabled = true`,
		userID,
	).Scan(&factorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrTOTPNotEnrolled
		}
		return "", fmt.Errorf("looking up TOTP factor: %w", err)
	}

	var challengeID string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_mfa_challenges (factor_id, ip_address)
		 VALUES ($1, $2::inet)
		 RETURNING id`,
		factorID, ipAddress,
	).Scan(&challengeID)
	if err != nil {
		return "", fmt.Errorf("creating MFA challenge: %w", err)
	}

	s.logger.Info("TOTP challenge created", "user_id", userID, "challenge_id", challengeID)
	return challengeID, nil
}

// VerifyTOTPChallenge verifies a TOTP code against a specific challenge.
// On success, issues AAL2 tokens.
func (s *Service) VerifyTOTPChallenge(ctx context.Context, userID, challengeID, code, firstFactorMethod string) (*User, string, string, error) {
	if s.pool == nil {
		return nil, "", "", errors.New("database pool is not configured")
	}

	// Load and validate the challenge.
	var factorID string
	var verifiedAt *time.Time
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT c.factor_id, c.verified_at, c.expires_at
		 FROM _ayb_mfa_challenges c
		 JOIN _ayb_user_mfa f ON f.id = c.factor_id
		 WHERE c.id = $1 AND f.user_id = $2`,
		challengeID, userID,
	).Scan(&factorID, &verifiedAt, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", ErrTOTPChallengeNotFound
		}
		return nil, "", "", fmt.Errorf("loading MFA challenge: %w", err)
	}
	if verifiedAt != nil {
		return nil, "", "", ErrTOTPChallengeUsed
	}
	if time.Now().After(expiresAt) {
		return nil, "", "", ErrTOTPChallengeNotFound
	}

	// Load the TOTP secret.
	var secretEnc []byte
	var lastUsedStep int64
	err = s.pool.QueryRow(ctx,
		`SELECT totp_secret_enc, COALESCE(last_used_step, 0) FROM _ayb_user_mfa WHERE id = $1`,
		factorID,
	).Scan(&secretEnc, &lastUsedStep)
	if err != nil {
		return nil, "", "", fmt.Errorf("loading TOTP secret: %w", err)
	}

	secret, err := s.decryptAESGCM(secretEnc)
	if err != nil {
		return nil, "", "", fmt.Errorf("decrypting TOTP secret: %w", err)
	}

	// Validate the code.
	matchedStep, ok := validateTOTPCode(secret, code, time.Now())
	if !ok {
		return nil, "", "", ErrTOTPInvalidCode
	}

	// Replay protection.
	if matchedStep <= lastUsedStep {
		return nil, "", "", ErrTOTPReplay
	}

	// Mark challenge as verified and update last_used_step in one transaction.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, "", "", fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`UPDATE _ayb_mfa_challenges SET verified_at = NOW() WHERE id = $1`,
		challengeID,
	)
	if err != nil {
		return nil, "", "", fmt.Errorf("marking challenge verified: %w", err)
	}

	_, err = tx.Exec(ctx,
		`UPDATE _ayb_user_mfa SET last_used_step = $1 WHERE id = $2`,
		matchedStep, factorID,
	)
	if err != nil {
		return nil, "", "", fmt.Errorf("updating last used step: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", "", fmt.Errorf("committing MFA verification: %w", err)
	}

	// Issue AAL2 tokens.
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	sessionOpts := mfaSessionOptions(firstFactorMethod, "totp")
	sessionID, refreshToken, err := s.createSession(ctx, user.ID, sessionOpts)
	if err != nil {
		return nil, "", "", fmt.Errorf("creating session: %w", err)
	}
	sessionOpts.SessionID = sessionID
	sessionOpts, err = s.sessionTokenOptions(ctx, user, sessionOpts)
	if err != nil {
		return nil, "", "", fmt.Errorf("resolving session tenant: %w", err)
	}

	token, err := s.generateTokenWithOpts(ctx, user, sessionOpts)
	if err != nil {
		return nil, "", "", fmt.Errorf("generating AAL2 token: %w", err)
	}

	s.logger.Info("TOTP MFA verified", "user_id", userID, "challenge_id", challengeID)
	return user, token, refreshToken, nil
}

// DefaultUnverifiedTOTPTTL is the default time-to-live for unverified TOTP
// enrollments. Enrollments older than this are cleaned up to prevent bloat.
const DefaultUnverifiedTOTPTTL = 10 * time.Minute

// CleanupUnverifiedTOTPEnrollments deletes unverified TOTP enrollments older
// than the specified TTL. This prevents stale factor bloat from abandoned enrollments.
func (s *Service) CleanupUnverifiedTOTPEnrollments(ctx context.Context, ttl time.Duration) error {
	if s.pool == nil {
		return errors.New("database pool is not configured")
	}
	cutoff := time.Now().Add(-ttl)
	result, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_user_mfa WHERE method = 'totp' AND enabled = false AND created_at < $1`,
		cutoff,
	)
	if err != nil {
		return fmt.Errorf("cleaning up unverified TOTP enrollments: %w", err)
	}
	if result.RowsAffected() > 0 {
		s.logger.Info("cleaned up unverified TOTP enrollments", "count", result.RowsAffected())
	}
	return nil
}

// HasTOTPMFA checks whether a user has an enabled TOTP MFA enrollment.
func (s *Service) HasTOTPMFA(ctx context.Context, userID string) (bool, error) {
	if s.pool == nil {
		return false, errors.New("database pool is not configured")
	}
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'totp' AND enabled = true)`,
		userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking TOTP MFA enrollment: %w", err)
	}
	return exists, nil
}

// HasAnyMFA checks whether a user has any enabled MFA factor (SMS, TOTP, email).
func (s *Service) HasAnyMFA(ctx context.Context, userID string) (bool, string, error) {
	if s.pool == nil {
		return false, "", errors.New("database pool is not configured")
	}
	var method string
	err := s.pool.QueryRow(ctx,
		`SELECT method FROM _ayb_user_mfa WHERE user_id = $1 AND enabled = true ORDER BY enrolled_at LIMIT 1`,
		userID,
	).Scan(&method)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("checking MFA enrollment: %w", err)
	}
	return true, method, nil
}

// GetUserMFAFactors returns all enabled MFA factors for a user.
type MFAFactor struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Label  string `json:"label"`           // human-readable: "Authenticator app", masked phone/email
	Phone  string `json:"phone,omitempty"` // e.g. "***1234"
	Email  string `json:"email,omitempty"` // e.g. "t***@example.com"
}

// maskEmail masks an email for display, e.g. "test@example.com" → "t***@example.com".
func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	return string(email[0]) + "***" + email[at:]
}

// GetUserMFAFactors returns all enabled MFA factors for a user in enrollment order, masking sensitive data for display: SMS numbers show only the last 4 digits, and emails show the first letter and domain.
func (s *Service) GetUserMFAFactors(ctx context.Context, userID string) ([]MFAFactor, error) {
	if s.pool == nil {
		return nil, errors.New("database pool is not configured")
	}
	rows, err := s.pool.Query(ctx,
		`SELECT f.id, f.method, COALESCE(f.phone, ''), COALESCE(u.email, '')
		 FROM _ayb_user_mfa f
		 JOIN _ayb_users u ON u.id = f.user_id
		 WHERE f.user_id = $1 AND f.enabled = true
		 ORDER BY f.enrolled_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying MFA factors: %w", err)
	}
	defer rows.Close()

	var factors []MFAFactor
	var userEmail string
	for rows.Next() {
		var f MFAFactor
		if err := rows.Scan(&f.ID, &f.Method, &f.Phone, &userEmail); err != nil {
			return nil, fmt.Errorf("scanning MFA factor: %w", err)
		}
		// Set method-specific display fields.
		switch f.Method {
		case "sms":
			if f.Phone != "" && len(f.Phone) > 4 {
				f.Phone = strings.Repeat("*", len(f.Phone)-4) + f.Phone[len(f.Phone)-4:]
			}
			f.Label = "SMS (" + f.Phone + ")"
		case "email":
			if userEmail != "" {
				f.Email = maskEmail(userEmail)
			}
			f.Label = "Email (" + f.Email + ")"
		case "totp":
			f.Label = "Authenticator app"
		default:
			f.Label = f.Method
		}
		factors = append(factors, f)
	}
	return factors, rows.Err()
}
