package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// Email MFA sentinel errors.
var (
	ErrEmailMFAAlreadyEnrolled = errors.New("email MFA already enrolled")
	ErrEmailMFANotEnrolled     = errors.New("no email MFA factor found")
	ErrEmailMFAInvalidCode     = errors.New("invalid email MFA code")
	ErrEmailMFAExpired         = errors.New("email MFA code expired")
	ErrEmailMFALocked          = errors.New("too many failed attempts, try again later")
	ErrEmailMFARateLimit       = errors.New("too many email challenges, try again later")
)

// Email MFA configuration constants.
const (
	emailMFACodeLen         = 6
	emailMFACodeExpiry      = 10 * time.Minute
	emailMFAChallengeExpiry = 10 * time.Minute
	emailMFAMaxAttempts     = 5  // per code
	emailMFAMaxChallenges   = 3  // per 10 minutes per user
	emailMFALockoutCount    = 15 // cumulative failures within window
	emailMFALockoutWindow   = time.Hour
	emailMFALockoutDuration = 30 * time.Minute
)

// generateEmailMFACode produces a 6-digit numeric code using crypto/rand.
func generateEmailMFACode() (string, error) {
	max := big.NewInt(1_000_000) // 10^6
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("generating email MFA code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// mfaFailureTracker tracks cumulative MFA verification failures for lockout.
type mfaFailureTracker struct {
	mu              sync.Mutex
	failures        map[string][]time.Time // userID -> timestamps of failures
	threshold       int
	window          time.Duration
	lockoutDuration time.Duration
	lockedUntil     map[string]time.Time // userID -> locked until
}

func newMFAFailureTracker(threshold int, window, lockoutDuration time.Duration) *mfaFailureTracker {
	return &mfaFailureTracker{
		failures:        make(map[string][]time.Time),
		threshold:       threshold,
		window:          window,
		lockoutDuration: lockoutDuration,
		lockedUntil:     make(map[string]time.Time),
	}
}

func (t *mfaFailureTracker) recordFailure(userID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.failures[userID] = append(t.failures[userID], now)
	t.pruneOld(userID, now)
	if len(t.failures[userID]) >= t.threshold {
		t.lockedUntil[userID] = now.Add(t.lockoutDuration)
		t.failures[userID] = nil // reset counter
	}
}

func (t *mfaFailureTracker) isLocked(userID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	until, ok := t.lockedUntil[userID]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(t.lockedUntil, userID)
		return false
	}
	return true
}

func (t *mfaFailureTracker) reset(userID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, userID)
	delete(t.lockedUntil, userID)
}

func (t *mfaFailureTracker) pruneOld(userID string, now time.Time) {
	cutoff := now.Add(-t.window)
	entries := t.failures[userID]
	i := 0
	for i < len(entries) && entries[i].Before(cutoff) {
		i++
	}
	if i == len(entries) {
		delete(t.failures, userID)
	} else {
		t.failures[userID] = entries[i:]
	}
}

// InitMFAFailureTracker initializes the cumulative failure tracker for MFA
// lockout. Must be called before email MFA verification is used.
func (s *Service) InitMFAFailureTracker() {
	s.mfaFailureTracker = newMFAFailureTracker(emailMFALockoutCount, emailMFALockoutWindow, emailMFALockoutDuration)
}

// IsMFALocked checks whether the user is currently locked out of MFA verification.
func (s *Service) IsMFALocked(userID string) bool {
	if s.mfaFailureTracker == nil {
		return false
	}
	return s.mfaFailureTracker.isLocked(userID)
}

// RecordMFAFailure records a failed MFA verification attempt for lockout tracking.
func (s *Service) RecordMFAFailure(userID string) {
	if s.mfaFailureTracker != nil {
		s.mfaFailureTracker.recordFailure(userID)
	}
}

// ResetMFAFailures resets the failure counter for a user after successful MFA.
func (s *Service) ResetMFAFailures(userID string) {
	if s.mfaFailureTracker != nil {
		s.mfaFailureTracker.reset(userID)
	}
}

// EnrollEmailMFA starts email MFA enrollment for a user. Creates an unverified
// email factor, generates a verification code, sends it, and stores the hash.
func (s *Service) EnrollEmailMFA(ctx context.Context, userID, email string) error {
	if s.pool == nil {
		return errors.New("database pool is not configured")
	}

	// Check for existing verified email MFA factor.
	var existing bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'email' AND enabled = true)`,
		userID,
	).Scan(&existing)
	if err != nil {
		return fmt.Errorf("checking email MFA enrollment: %w", err)
	}
	if existing {
		return ErrEmailMFAAlreadyEnrolled
	}

	// Upsert unverified factor.
	var factorID string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_user_mfa (user_id, method, enabled)
		 VALUES ($1, 'email', false)
		 ON CONFLICT (user_id, method) DO UPDATE
		 SET enabled = false, enrolled_at = NULL
		 RETURNING id`,
		userID,
	).Scan(&factorID)
	if err != nil {
		return fmt.Errorf("inserting email MFA enrollment: %w", err)
	}

	// Generate and store verification code in a challenge record.
	code, err := generateEmailMFACode()
	if err != nil {
		return err
	}
	codeHash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing email MFA code: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO _ayb_mfa_challenges (factor_id, otp_code_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		factorID, string(codeHash), time.Now().Add(emailMFACodeExpiry),
	)
	if err != nil {
		return fmt.Errorf("storing email MFA enrollment code: %w", err)
	}

	// Send verification email. Fail enrollment if email cannot be sent,
	// since the user has no way to retrieve the code otherwise.
	if s.emailDeliveryConfigured() {
		if err := s.sendEmailMFACode(ctx, email, code, "auth.mfa_email_enroll"); err != nil {
			s.logger.Error("failed to send email MFA enrollment code", "error", err)
			return fmt.Errorf("sending email MFA enrollment code: %w", err)
		}
	}

	s.logger.Info("email MFA enrollment started", "user_id", userID, "factor_id", factorID)
	return nil
}

// ConfirmEmailMFAEnrollment verifies the enrollment code and activates the factor.
func (s *Service) ConfirmEmailMFAEnrollment(ctx context.Context, userID, code string) error {
	if s.pool == nil {
		return errors.New("database pool is not configured")
	}

	// Find the unverified email factor and its latest challenge.
	var factorID, challengeID string
	var codeHash string
	var expiresAt time.Time
	var attemptCount int
	err := s.pool.QueryRow(ctx,
		`SELECT f.id, c.id, c.otp_code_hash, c.expires_at, c.attempt_count
		 FROM _ayb_user_mfa f
		 JOIN _ayb_mfa_challenges c ON c.factor_id = f.id
		 WHERE f.user_id = $1 AND f.method = 'email' AND f.enabled = false
		   AND c.verified_at IS NULL AND c.otp_code_hash IS NOT NULL
		 ORDER BY c.created_at DESC LIMIT 1`,
		userID,
	).Scan(&factorID, &challengeID, &codeHash, &expiresAt, &attemptCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEmailMFANotEnrolled
		}
		return fmt.Errorf("loading email MFA enrollment challenge: %w", err)
	}

	if time.Now().After(expiresAt) {
		return ErrEmailMFAExpired
	}

	// Per-code attempt limit: reject after emailMFAMaxAttempts failures.
	if attemptCount >= emailMFAMaxAttempts {
		return ErrEmailMFAInvalidCode
	}

	if bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(code)) != nil {
		// Increment attempt count; invalidate on 5th failure.
		if _, err := s.pool.Exec(ctx,
			`UPDATE _ayb_mfa_challenges SET attempt_count = attempt_count + 1 WHERE id = $1`,
			challengeID,
		); err != nil {
			s.logger.Error("failed to increment enrollment attempt counter", "error", err, "challenge_id", challengeID)
		}
		return ErrEmailMFAInvalidCode
	}

	// Activate the factor and mark challenge as verified.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`UPDATE _ayb_user_mfa SET enabled = true, enrolled_at = NOW() WHERE id = $1`,
		factorID,
	)
	if err != nil {
		return fmt.Errorf("enabling email MFA: %w", err)
	}

	_, err = tx.Exec(ctx,
		`UPDATE _ayb_mfa_challenges SET verified_at = NOW() WHERE id = $1`,
		challengeID,
	)
	if err != nil {
		return fmt.Errorf("marking enrollment challenge verified: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing email MFA enrollment: %w", err)
	}

	s.logger.Info("email MFA enrollment confirmed", "user_id", userID, "factor_id", factorID)
	return nil
}

// ChallengeEmailMFA creates a challenge record for email MFA and sends the code.
// Rate-limited to emailMFAMaxChallenges per emailMFAChallengeExpiry window.
func (s *Service) ChallengeEmailMFA(ctx context.Context, userID, email string) (string, error) {
	if s.pool == nil {
		return "", errors.New("database pool is not configured")
	}

	// Find the user's enabled email MFA factor.
	var factorID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'email' AND enabled = true`,
		userID,
	).Scan(&factorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrEmailMFANotEnrolled
		}
		return "", fmt.Errorf("looking up email MFA factor: %w", err)
	}

	// Rate-limit: count recent challenges for this factor.
	var recentCount int
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_mfa_challenges
		 WHERE factor_id = $1 AND created_at > NOW() - INTERVAL '10 minutes'`,
		factorID,
	).Scan(&recentCount)
	if err != nil {
		return "", fmt.Errorf("counting recent challenges: %w", err)
	}
	if recentCount >= emailMFAMaxChallenges {
		return "", ErrEmailMFARateLimit
	}

	// Generate code.
	code, err := generateEmailMFACode()
	if err != nil {
		return "", err
	}
	codeHash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing email MFA code: %w", err)
	}

	var challengeID string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_mfa_challenges (factor_id, otp_code_hash, expires_at)
		 VALUES ($1, $2, $3)
		 RETURNING id`,
		factorID, string(codeHash), time.Now().Add(emailMFAChallengeExpiry),
	).Scan(&challengeID)
	if err != nil {
		return "", fmt.Errorf("creating email MFA challenge: %w", err)
	}

	// Send code via email. Fail challenge if email cannot be sent,
	// since the user has no way to retrieve the code otherwise.
	if s.emailDeliveryConfigured() {
		if err := s.sendEmailMFACode(ctx, email, code, "auth.mfa_email_challenge"); err != nil {
			s.logger.Error("failed to send email MFA challenge code", "error", err)
			return "", fmt.Errorf("sending email MFA challenge code: %w", err)
		}
	}

	s.logger.Info("email MFA challenge created", "user_id", userID, "challenge_id", challengeID)
	return challengeID, nil
}

// VerifyEmailMFA verifies an email MFA code against a challenge and issues AAL2 tokens.
func (s *Service) VerifyEmailMFA(ctx context.Context, userID, challengeID, code, firstFactorMethod string) (*User, string, string, error) {
	if s.pool == nil {
		return nil, "", "", errors.New("database pool is not configured")
	}

	// Load challenge.
	var factorID string
	var codeHash string
	var verifiedAt *time.Time
	var expiresAt time.Time
	var attemptCount int
	err := s.pool.QueryRow(ctx,
		`SELECT c.factor_id, c.otp_code_hash, c.verified_at, c.expires_at, c.attempt_count
		 FROM _ayb_mfa_challenges c
		 JOIN _ayb_user_mfa f ON f.id = c.factor_id
		 WHERE c.id = $1 AND f.user_id = $2 AND f.method = 'email' AND f.enabled = true`,
		challengeID, userID,
	).Scan(&factorID, &codeHash, &verifiedAt, &expiresAt, &attemptCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", "", ErrTOTPChallengeNotFound
		}
		return nil, "", "", fmt.Errorf("loading email MFA challenge: %w", err)
	}
	if verifiedAt != nil {
		return nil, "", "", ErrTOTPChallengeUsed
	}
	if time.Now().After(expiresAt) {
		return nil, "", "", ErrEmailMFAExpired
	}

	// Per-code attempt limit: reject after emailMFAMaxAttempts failures.
	if attemptCount >= emailMFAMaxAttempts {
		return nil, "", "", ErrEmailMFAInvalidCode
	}

	if bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(code)) != nil {
		// Increment attempt count.
		if _, err := s.pool.Exec(ctx,
			`UPDATE _ayb_mfa_challenges SET attempt_count = attempt_count + 1 WHERE id = $1`,
			challengeID,
		); err != nil {
			s.logger.Error("failed to increment verify attempt counter", "error", err, "challenge_id", challengeID)
		}
		return nil, "", "", ErrEmailMFAInvalidCode
	}

	// Mark challenge as verified.
	_, err = s.pool.Exec(ctx,
		`UPDATE _ayb_mfa_challenges SET verified_at = NOW() WHERE id = $1`,
		challengeID,
	)
	if err != nil {
		return nil, "", "", fmt.Errorf("marking email MFA challenge verified: %w", err)
	}

	// Issue AAL2 tokens.
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	sessionOpts := mfaSessionOptions(firstFactorMethod, "email_otp")
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

	s.logger.Info("email MFA verified", "user_id", userID, "challenge_id", challengeID)
	return user, token, refreshToken, nil
}

// sendEmailMFACode sends an MFA code email using the template system.
func (s *Service) sendEmailMFACode(ctx context.Context, email, code, templateKey string) error {
	vars := map[string]string{
		"AppName": s.appName,
		"Code":    code,
	}
	subject, html, text, err := s.renderAuthEmail(ctx, templateKey, vars)
	if err != nil {
		return fmt.Errorf("rendering email MFA template: %w", err)
	}
	return s.sendAuthEmail(ctx, email, subject, html, text, "mfa_email")
}
