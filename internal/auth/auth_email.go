package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/mailer"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/jackc/pgx/v5"
)

// legacyRenderFuncs maps template keys to their legacy render functions.
var legacyRenderFuncs = map[string]func(mailer.TemplateData) (string, string, error){
	"auth.password_reset":      mailer.RenderPasswordReset,
	"auth.email_verification":  mailer.RenderVerification,
	"auth.magic_link":          mailer.RenderMagicLink,
	"auth.mfa_email_enroll":    mailer.RenderMFAEmailEnroll,
	"auth.mfa_email_challenge": mailer.RenderMFAEmailChallenge,
}

// legacySubjects maps template keys to their default subjects.
var legacySubjects = map[string]string{
	"auth.password_reset":      mailer.DefaultPasswordResetSubject,
	"auth.email_verification":  mailer.DefaultVerificationSubject,
	"auth.magic_link":          mailer.DefaultMagicLinkSubject,
	"auth.mfa_email_enroll":    mailer.DefaultMFAEmailEnrollSubject,
	"auth.mfa_email_challenge": mailer.DefaultMFAEmailChallengeSubject,
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
		Code:      vars["Code"],
	}
	html, text, err = renderFn(data)
	if err != nil {
		return "", "", "", err
	}
	return legacySubjects[key], html, text, nil
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

// SetHookDispatcher wires auth lifecycle hooks.
func (s *Service) SetHookDispatcher(d *HookDispatcher) {
	s.hookDispatcher = d
}

func (s *Service) emailDeliveryConfigured() bool {
	if s.mailer != nil {
		return true
	}
	if s.hookDispatcher == nil {
		return false
	}
	return strings.TrimSpace(s.hookDispatcher.hooks.SendEmail) != ""
}

func (s *Service) smsDeliveryConfigured() bool {
	if s.smsProvider != nil {
		return true
	}
	if s.hookDispatcher == nil {
		return false
	}
	return strings.TrimSpace(s.hookDispatcher.hooks.SendSMS) != ""
}

// sendAuthEmail sends an authentication email via the configured hook dispatcher or mailer. If a hook dispatcher is configured, it attempts to send via the hook; if that returns ErrHookNotConfigured or no hook dispatcher is present, it falls back to the mailer. Returns nil if email delivery is not configured.
func (s *Service) sendAuthEmail(ctx context.Context, to, subject, html, text, emailType string) error {
	if s.hookDispatcher != nil {
		if err := s.hookDispatcher.SendEmail(ctx, to, subject, html, text, emailType); err != nil {
			if !errors.Is(err, ErrHookNotConfigured) {
				return err
			}
		} else {
			return nil
		}
	}
	if s.mailer == nil {
		return nil
	}
	return s.mailer.Send(ctx, &mailer.Message{
		To:      to,
		Subject: subject,
		HTML:    html,
		Text:    text,
	})
}

// sendAuthSMS sends an SMS message via the configured hook dispatcher or SMS provider. If a hook dispatcher is configured, it attempts to send via the hook; if that returns ErrHookNotConfigured or no hook dispatcher is present, it falls back to the SMS provider. Returns nil if SMS delivery is not configured.
func (s *Service) sendAuthSMS(ctx context.Context, phone, message, smsType string) error {
	if s.hookDispatcher != nil {
		if err := s.hookDispatcher.SendSMS(ctx, phone, message, smsType); err != nil {
			if !errors.Is(err, ErrHookNotConfigured) {
				return err
			}
		} else {
			return nil
		}
	}
	if s.smsProvider == nil {
		return nil
	}
	_, err := s.smsProvider.Send(ctx, phone, message)
	return err
}

const (
	resetTokenBytes   = 32
	resetTokenExpiry  = 1 * time.Hour
	verifyTokenBytes  = 32
	verifyTokenExpiry = 24 * time.Hour
)

// RequestPasswordReset generates a reset token and emails it to the user.
// Returns nil for unknown emails to prevent enumeration — handler should always return 200.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if s.hookDispatcher != nil {
		_, ipAddress := requestMetadataFromContext(ctx)
		if err := s.hookDispatcher.BeforePasswordReset(ctx, email, ipAddress); err != nil {
			if errors.Is(err, ErrHookRejected) {
				return nil
			}
			return fmt.Errorf("before_password_reset hook failed: %w", err)
		}
	}
	if !s.emailDeliveryConfigured() {
		return nil
	}

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

	if err := s.sendAuthEmail(ctx, email, subject, html, text, "recovery"); err != nil {
		return fmt.Errorf("sending password reset email: %w", err)
	}
	return nil
}

// ConfirmPasswordReset validates the token and sets a new password.
func (s *Service) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	if err := validatePassword(newPassword, s.minPwLen); err != nil {
		return err
	}

	hash := hashToken(token)

	// Atomically consume the token: DELETE ... RETURNING prevents double-use
	// races where two concurrent requests could both SELECT the same valid
	// token. This matches the pattern used by ConfirmMagicLink.
	var userID string
	err := s.pool.QueryRow(ctx,
		`DELETE FROM _ayb_password_resets
		 WHERE token_hash = $1 AND expires_at > NOW()
		 RETURNING user_id`,
		hash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidResetToken
		}
		return fmt.Errorf("consuming reset token: %w", err)
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

	// Delete any remaining reset tokens for this user (e.g. other pending resets).
	if _, err := s.pool.Exec(ctx, `DELETE FROM _ayb_password_resets WHERE user_id = $1`, userID); err != nil {
		s.logger.Error("failed to delete remaining reset tokens after password reset", "user_id", userID, "error", err)
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
	if !s.emailDeliveryConfigured() {
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

	if err := s.sendAuthEmail(ctx, email, subject, html, text, "signup"); err != nil {
		return fmt.Errorf("sending verification email: %w", err)
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
