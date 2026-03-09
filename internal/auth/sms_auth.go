package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// generateOTP produces an N-digit numeric string using crypto/rand.
func generateOTP(length int) (string, error) {
	digits := make([]byte, length)
	for i := range digits {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("generating OTP digit: %w", err)
		}
		digits[i] = '0' + byte(n.Int64())
	}
	return string(digits), nil
}

// normalizePhone delegates to sms.NormalizePhone for E.164 normalization.
func normalizePhone(input string) (string, error) {
	return sms.NormalizePhone(input)
}

// phoneCountry delegates to sms.PhoneCountry for country detection.
func phoneCountry(phone string) string {
	return sms.PhoneCountry(phone)
}

// isAllowedCountry delegates to sms.IsAllowedCountry for country allowlist checks.
func isAllowedCountry(phone string, allowed []string) bool {
	return sms.IsAllowedCountry(phone, allowed)
}

// RequestSMSCode sends an OTP to the given phone number.
func (s *Service) RequestSMSCode(ctx context.Context, phone string) error {
	if s.smsProvider == nil {
		return nil
	}

	// Test phone numbers bypass normalization, country checks, and daily limits.
	// They are stored as E.164 strings in config and only need the predetermined
	// code written to the DB for verification.
	if code, ok := s.smsConfig.TestPhoneNumbers[phone]; ok {
		return s.storeOTPCode(ctx, phone, code)
	}

	phone, err := normalizePhone(phone)
	if err != nil {
		return ErrInvalidPhoneNumber
	}

	if !isAllowedCountry(phone, s.smsConfig.AllowedCountries) {
		return nil // anti-enumeration: silently ignore blocked countries
	}

	// Check daily limit.
	if s.smsConfig.DailyLimit > 0 {
		var count int
		err := s.pool.QueryRow(ctx,
			`SELECT count FROM _ayb_sms_daily_counts WHERE date = CURRENT_DATE`,
		).Scan(&count)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.logger.Error("SMS daily count query error", "error", err)
			return nil
		}
		if count >= s.smsConfig.DailyLimit {
			return ErrDailyLimitExceeded
		}
	}

	// Increment daily count before sending (prevents abuse on repeated failures).
	_, err = s.pool.Exec(ctx,
		`INSERT INTO _ayb_sms_daily_counts (date, count) VALUES (CURRENT_DATE, 1)
		 ON CONFLICT (date) DO UPDATE SET count = _ayb_sms_daily_counts.count + 1`,
	)
	if err != nil {
		s.logger.Error("SMS daily count increment error", "error", err)
	}

	// Generate OTP, store it, and send via SMS provider.
	if err := s.sendOTPToPhone(ctx, phone, "Your code is: "); err != nil {
		s.logger.Error("SMS OTP send error", "error", err)
	}
	return nil
}

// ConfirmSMSCode verifies an OTP, finds or creates the user by phone,
// and returns tokens.
func (s *Service) ConfirmSMSCode(ctx context.Context, phone, code string) (*User, string, string, error) {
	// Test phone numbers bypass normalization (they may use fictional numbers
	// like 555-xxx that fail libphonenumber validation).
	if _, ok := s.smsConfig.TestPhoneNumbers[phone]; !ok {
		var err error
		phone, err = normalizePhone(phone)
		if err != nil {
			return nil, "", "", ErrInvalidSMSCode
		}
	}

	if err := s.validateSMSCodeForPhone(ctx, phone, code); err != nil {
		s.incrementSMSStat(ctx, "fail_count")
		return nil, "", "", err
	}

	s.incrementSMSStat(ctx, "confirm_count")

	// Find or create user by phone.
	var user User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, phone, created_at, updated_at FROM _ayb_users WHERE phone = $1`,
		phone,
	).Scan(&user.ID, &user.Email, &user.Phone, &user.CreatedAt, &user.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		randomPW := make([]byte, 32)
		if _, err := rand.Read(randomPW); err != nil {
			return nil, "", "", fmt.Errorf("generating random password: %w", err)
		}
		pwHash, err := hashPassword(base64.RawURLEncoding.EncodeToString(randomPW))
		if err != nil {
			return nil, "", "", fmt.Errorf("hashing placeholder password: %w", err)
		}

		// Generate a placeholder email — _ayb_users.email is NOT NULL.
		placeholderEmail := phone + "@sms.local"

		err = s.pool.QueryRow(ctx,
			`INSERT INTO _ayb_users (email, phone, password_hash) VALUES ($1, $2, $3)
			 RETURNING id, email, phone, created_at, updated_at`,
			placeholderEmail, phone, pwHash,
		).Scan(&user.ID, &user.Email, &user.Phone, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				err2 := s.pool.QueryRow(ctx,
					`SELECT id, email, phone, created_at, updated_at FROM _ayb_users WHERE phone = $1`,
					phone,
				).Scan(&user.ID, &user.Email, &user.Phone, &user.CreatedAt, &user.UpdatedAt)
				if err2 != nil {
					return nil, "", "", fmt.Errorf("querying user after conflict: %w", err2)
				}
			} else {
				return nil, "", "", fmt.Errorf("inserting user: %w", err)
			}
		} else {
			s.logger.Info("user registered via SMS", "user_id", user.ID, "phone", phone)
		}
	} else if err != nil {
		return nil, "", "", fmt.Errorf("querying user: %w", err)
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

// --- Handler types and methods ---

type smsRequest struct {
	Phone string `json:"phone"`
}

type smsConfirmRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

func (h *Handler) handleSMSRequest(w http.ResponseWriter, r *http.Request) {
	if !h.smsEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "SMS authentication is not enabled",
			"https://allyourbase.io/guide/authentication#sms")
		return
	}

	var req smsRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Phone == "" {
		httputil.WriteError(w, http.StatusBadRequest, "phone is required")
		return
	}

	if _, err := normalizePhone(req.Phone); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid phone number format")
		return
	}

	// Always return 200 to prevent phone enumeration.
	if err := h.auth.RequestSMSCode(r.Context(), req.Phone); err != nil {
		if errors.Is(err, ErrDailyLimitExceeded) {
			h.logger.Warn("SMS daily limit exceeded")
		} else {
			h.logger.Error("SMS request error", "error", err)
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "if valid, a verification code has been sent",
	})
}

func (h *Handler) handleSMSConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.smsEnabled {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound, "SMS authentication is not enabled",
			"https://allyourbase.io/guide/authentication#sms")
		return
	}

	var req smsConfirmRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Phone == "" {
		httputil.WriteError(w, http.StatusBadRequest, "phone is required")
		return
	}
	if req.Code == "" {
		httputil.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}

	user, accessToken, refreshToken, err := h.auth.ConfirmSMSCode(r.Context(), req.Phone, req.Code)
	if err != nil {
		if errors.Is(err, ErrInvalidSMSCode) {
			httputil.WriteError(w, http.StatusUnauthorized, "invalid or expired SMS code")
			return
		}
		h.logger.Error("SMS confirm error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if refreshToken == "" {
		httputil.WriteJSON(w, http.StatusOK, mfaPendingResponse{
			MFAPending: true,
			MFAToken:   accessToken,
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, authResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

// incrementSMSStat increments a stat column (confirm_count or fail_count) in
// _ayb_sms_daily_counts for today. Uses upsert in case no row exists yet.
func (s *Service) incrementSMSStat(ctx context.Context, column string) {
	// column is always a compile-time constant ("confirm_count" or "fail_count"),
	// never user input, so string interpolation is safe here.
	query := fmt.Sprintf(
		`INSERT INTO _ayb_sms_daily_counts (date, count, confirm_count, fail_count)
		 VALUES (CURRENT_DATE, 0, 0, 0)
		 ON CONFLICT (date) DO UPDATE SET %s = _ayb_sms_daily_counts.%s + 1`,
		column, column,
	)
	if _, err := s.pool.Exec(ctx, query); err != nil {
		s.logger.Error("SMS stat increment error", "column", column, "error", err)
	}
}
