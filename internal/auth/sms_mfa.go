package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/allyourbase/ayb/internal/sms"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrMFAAlreadyEnrolled = errors.New("SMS MFA already enrolled")

const mfaPendingTokenDur = 5 * time.Minute

// generateMFAPendingToken issues a short-lived JWT (5 min) with MFAPending: true.
// This token grants access only to the MFA challenge/verify endpoints, not normal routes.
func (s *Service) generateMFAPendingToken(user *User) (string, error) {
	now := time.Now()
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", fmt.Errorf("generating jti: %w", err)
	}
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(mfaPendingTokenDur)),
			ID:        hex.EncodeToString(jti),
		},
		Email:      user.Email,
		MFAPending: true,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s.jwtSecretMu.RLock()
	secret := s.jwtSecret
	s.jwtSecretMu.RUnlock()
	return token.SignedString(secret)
}

// HasSMSMFA checks whether a user has an enabled SMS MFA enrollment.
func (s *Service) HasSMSMFA(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'sms' AND enabled = true)`,
		userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking SMS MFA enrollment: %w", err)
	}
	return exists, nil
}

// EnrollSMSMFA starts the MFA enrollment process by sending an OTP to the given phone.
func (s *Service) EnrollSMSMFA(ctx context.Context, userID, phone string) error {
	phone, err := sms.NormalizePhone(phone)
	if err != nil {
		return ErrInvalidPhoneNumber
	}

	// Check for existing enabled enrollment.
	has, err := s.HasSMSMFA(ctx, userID)
	if err != nil {
		return err
	}
	if has {
		return ErrMFAAlreadyEnrolled
	}

	// Upsert the enrollment row (disabled until confirmed).
	_, err = s.pool.Exec(ctx,
		`INSERT INTO _ayb_user_mfa (user_id, method, phone, enabled)
		 VALUES ($1, 'sms', $2, false)
		 ON CONFLICT (user_id, method) DO UPDATE SET phone = $2, enabled = false, enrolled_at = NULL`,
		userID, phone,
	)
	if err != nil {
		return fmt.Errorf("inserting MFA enrollment: %w", err)
	}

	return s.sendOTPToPhone(ctx, phone, "Your MFA code is: ")
}

// ConfirmSMSMFAEnrollment verifies the OTP and enables the MFA enrollment.
func (s *Service) ConfirmSMSMFAEnrollment(ctx context.Context, userID, phone, code string) error {
	phone, err := sms.NormalizePhone(phone)
	if err != nil {
		return ErrInvalidSMSCode
	}

	if err := s.validateSMSCodeForPhone(ctx, phone, code); err != nil {
		return err
	}

	// Enable the enrollment.
	result, err := s.pool.Exec(ctx,
		`UPDATE _ayb_user_mfa SET enabled = true, enrolled_at = now()
		 WHERE user_id = $1 AND method = 'sms' AND phone = $2`,
		userID, phone,
	)
	if err != nil {
		return fmt.Errorf("enabling MFA enrollment: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("no MFA enrollment found for user")
	}

	return nil
}

// ChallengeSMSMFA sends an OTP to the user's enrolled MFA phone number.
func (s *Service) ChallengeSMSMFA(ctx context.Context, userID string) error {
	phone, err := s.mfaEnrolledPhone(ctx, userID)
	if err != nil {
		return err
	}
	return s.sendOTPToPhone(ctx, phone, "Your verification code is: ")
}

// VerifySMSMFA verifies the MFA challenge OTP and issues full tokens.
func (s *Service) VerifySMSMFA(ctx context.Context, userID, code string) (*User, string, string, error) {
	phone, err := s.mfaEnrolledPhone(ctx, userID)
	if err != nil {
		return nil, "", "", err
	}

	if err := s.validateSMSCodeForPhone(ctx, phone, code); err != nil {
		return nil, "", "", err
	}

	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	return s.issueTokens(ctx, user)
}

// mfaEnrolledPhone looks up the enrolled MFA phone for a user.
func (s *Service) mfaEnrolledPhone(ctx context.Context, userID string) (string, error) {
	var phone string
	err := s.pool.QueryRow(ctx,
		`SELECT phone FROM _ayb_user_mfa WHERE user_id = $1 AND method = 'sms' AND enabled = true`,
		userID,
	).Scan(&phone)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("no SMS MFA enrollment found")
		}
		return "", fmt.Errorf("querying MFA enrollment: %w", err)
	}
	return phone, nil
}

// storeOTPCode hashes the code and stores it in _ayb_sms_codes for the given phone.
func (s *Service) storeOTPCode(ctx context.Context, phone, code string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing OTP: %w", err)
	}

	expiry := s.smsConfig.Expiry
	if expiry <= 0 {
		expiry = 5 * time.Minute
	}

	_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_sms_codes WHERE phone = $1`, phone)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO _ayb_sms_codes (phone, code_hash, expires_at) VALUES ($1, $2, $3)`,
		phone, string(hash), time.Now().Add(expiry),
	)
	if err != nil {
		return fmt.Errorf("inserting OTP: %w", err)
	}
	return nil
}

// sendOTPToPhone generates an OTP, stores it in _ayb_sms_codes, and sends it via SMS.
// The msgPrefix is prepended to the OTP code in the SMS body.
// For test phone numbers (configured in sms.Config.TestPhoneNumbers), the predetermined
// code is used and the SMS provider is not called.
func (s *Service) sendOTPToPhone(ctx context.Context, phone, msgPrefix string) error {
	// Use predetermined code for test phone numbers, skip provider send.
	if code, ok := s.smsConfig.TestPhoneNumbers[phone]; ok {
		return s.storeOTPCode(ctx, phone, code)
	}

	codeLen := s.smsConfig.CodeLength
	if codeLen < 4 {
		codeLen = 6
	}
	otp, err := generateOTP(codeLen)
	if err != nil {
		return fmt.Errorf("generating OTP: %w", err)
	}

	if err := s.storeOTPCode(ctx, phone, otp); err != nil {
		return err
	}

	if s.smsProvider != nil {
		if _, err := s.smsProvider.Send(ctx, phone, msgPrefix+otp); err != nil {
			return fmt.Errorf("sending OTP: %w", err)
		}
	}

	return nil
}

// validateSMSCodeForPhone validates an OTP code against _ayb_sms_codes for the given phone.
// Shared helper used by both MFA enrollment confirmation and MFA challenge verification.
func (s *Service) validateSMSCodeForPhone(ctx context.Context, phone, code string) error {
	maxAttempts := s.smsConfig.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	var codeID int64
	var codeHash string
	err := s.pool.QueryRow(ctx,
		`SELECT id, code_hash FROM _ayb_sms_codes
		 WHERE phone = $1 AND expires_at > NOW() AND attempts < $2
		 ORDER BY created_at DESC LIMIT 1`,
		phone, maxAttempts,
	).Scan(&codeID, &codeHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_sms_codes WHERE phone = $1`, phone)
			return ErrInvalidSMSCode
		}
		return fmt.Errorf("querying SMS code: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(code)); err != nil {
		_, _ = s.pool.Exec(ctx,
			`UPDATE _ayb_sms_codes SET attempts = attempts + 1 WHERE id = $1`, codeID)
		var newAttempts int
		_ = s.pool.QueryRow(ctx,
			`SELECT attempts FROM _ayb_sms_codes WHERE id = $1`, codeID,
		).Scan(&newAttempts)
		if newAttempts >= maxAttempts {
			_, _ = s.pool.Exec(ctx, `DELETE FROM _ayb_sms_codes WHERE id = $1`, codeID)
		}
		return ErrInvalidSMSCode
	}

	// Consume the code.
	var consumedID int64
	err = s.pool.QueryRow(ctx,
		`DELETE FROM _ayb_sms_codes WHERE id = $1 RETURNING id`, codeID,
	).Scan(&consumedID)
	if err != nil {
		return ErrInvalidSMSCode
	}

	return nil
}
