package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	backupCodeCount   = 10
	backupCodePartLen = 5 // xxxxx-xxxxx
)

var (
	ErrBackupCodeInvalid = errors.New("invalid or already used backup code")
	ErrNoBackupCodes     = errors.New("no backup codes available")
)

// backupCodeAlphabet is the character set for backup code generation (no ambiguous chars).
const backupCodeAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// GenerateBackupCodes creates 10 backup codes, hashes them with bcrypt, stores
// the hashes, and returns the plaintext codes (shown once to the user).
func (s *Service) GenerateBackupCodes(ctx context.Context, userID string) ([]string, error) {
	if s.pool == nil {
		return nil, errors.New("database pool is not configured")
	}

	codes := make([]string, backupCodeCount)
	for i := range codes {
		code, err := generateBackupCode()
		if err != nil {
			return nil, fmt.Errorf("generating backup code: %w", err)
		}
		codes[i] = code
	}

	// Begin transaction: delete old codes, insert new hashed ones.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete existing codes for this user.
	_, err = tx.Exec(ctx, `DELETE FROM _ayb_mfa_backup_codes WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("deleting old backup codes: %w", err)
	}

	// Insert new hashed codes.
	for _, code := range codes {
		// Normalize: strip hyphen for hashing.
		normalized := strings.ReplaceAll(code, "-", "")
		hash, err := bcrypt.GenerateFromPassword([]byte(normalized), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hashing backup code: %w", err)
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO _ayb_mfa_backup_codes (user_id, code_hash) VALUES ($1, $2)`,
			userID, string(hash),
		)
		if err != nil {
			return nil, fmt.Errorf("inserting backup code: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing backup codes: %w", err)
	}

	s.logger.Info("backup codes generated", "user_id", userID, "count", backupCodeCount)
	return codes, nil
}

// VerifyBackupCode verifies a backup code and marks it as used atomically.
// Uses a transaction with SELECT FOR UPDATE to prevent concurrent double-spend.
func (s *Service) VerifyBackupCode(ctx context.Context, userID, code string) error {
	if s.pool == nil {
		return errors.New("database pool is not configured")
	}

	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(code)), "-", "")

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Load unused backup codes with row-level lock to prevent concurrent use.
	rows, err := tx.Query(ctx,
		`SELECT id, code_hash FROM _ayb_mfa_backup_codes
		 WHERE user_id = $1 AND used_at IS NULL
		 ORDER BY created_at
		 FOR UPDATE`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("querying backup codes: %w", err)
	}

	// Collect all codes before closing the cursor so we can use the
	// transaction connection for the subsequent UPDATE.
	type codeRow struct {
		id, hash string
	}
	var candidates []codeRow
	for rows.Next() {
		var cr codeRow
		if err := rows.Scan(&cr.id, &cr.hash); err != nil {
			rows.Close()
			return fmt.Errorf("scanning backup code: %w", err)
		}
		candidates = append(candidates, cr)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating backup codes: %w", err)
	}

	for _, cr := range candidates {
		if bcrypt.CompareHashAndPassword([]byte(cr.hash), []byte(normalized)) == nil {
			// Match — mark as used with used_at IS NULL guard for defense-in-depth.
			tag, err := tx.Exec(ctx,
				`UPDATE _ayb_mfa_backup_codes SET used_at = NOW() WHERE id = $1 AND used_at IS NULL`,
				cr.id,
			)
			if err != nil {
				return fmt.Errorf("marking backup code used: %w", err)
			}
			if tag.RowsAffected() == 0 {
				return ErrBackupCodeInvalid
			}
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("committing backup code use: %w", err)
			}
			s.logger.Info("backup code used", "user_id", userID, "code_id", cr.id)
			return nil
		}
	}

	return ErrBackupCodeInvalid
}

// VerifyBackupCodeMFA verifies a backup code and issues AAL2 tokens.
func (s *Service) VerifyBackupCodeMFA(ctx context.Context, userID, code, firstFactorMethod string) (*User, string, string, error) {
	if err := s.VerifyBackupCode(ctx, userID, code); err != nil {
		return nil, "", "", err
	}

	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return nil, "", "", fmt.Errorf("looking up user: %w", err)
	}

	sessionOpts := mfaSessionOptions(firstFactorMethod, "backup_code")
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

	return user, token, refreshToken, nil
}

// GetBackupCodeCount returns the number of unused backup codes for a user.
func (s *Service) GetBackupCodeCount(ctx context.Context, userID string) (int, error) {
	if s.pool == nil {
		return 0, errors.New("database pool is not configured")
	}
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM _ayb_mfa_backup_codes WHERE user_id = $1 AND used_at IS NULL`,
		userID,
	).Scan(&count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("counting backup codes: %w", err)
	}
	return count, nil
}

// generateBackupCode produces a single xxxxx-xxxxx format code using crypto/rand.
func generateBackupCode() (string, error) {
	alphaLen := big.NewInt(int64(len(backupCodeAlphabet)))
	buf := make([]byte, backupCodePartLen*2)
	for i := range buf {
		n, err := rand.Int(rand.Reader, alphaLen)
		if err != nil {
			return "", err
		}
		buf[i] = backupCodeAlphabet[n.Int64()]
	}
	return string(buf[:backupCodePartLen]) + "-" + string(buf[backupCodePartLen:]), nil
}
