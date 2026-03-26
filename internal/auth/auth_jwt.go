package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const maxJWTTokenLength = 16 * 1024

var errJWTSecretNotConfigured = errors.New("jwt secret is not configured")

// readJWTSecret returns the current JWT signing secret under a read-lock.
func (s *Service) readJWTSecret() ([]byte, error) {
	s.jwtSecretMu.RLock()
	secret := s.jwtSecret
	s.jwtSecretMu.RUnlock()
	if len(secret) == 0 {
		return nil, errJWTSecretNotConfigured
	}
	return secret, nil
}

// ValidateToken parses and validates a JWT token string.
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	if len(tokenString) > maxJWTTokenLength {
		return nil, errors.New("token too large")
	}

	secret, err := s.readJWTSecret()
	if err != nil {
		return nil, err
	}

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
	if s.denyList != nil && claims.SessionID != "" && s.denyList.IsDenied(claims.SessionID) {
		return nil, ErrTokenRevoked
	}
	return claims, nil
}

// tokenOptions carries optional claims for token generation.
type tokenOptions struct {
	AAL       string   // "aal1" or "aal2"
	AMR       []string // e.g. ["password", "totp"]
	SessionID string   // refresh-token session id
	TenantID  string   // resolved tenant context for authenticated sessions
}

func (s *Service) generateToken(ctx context.Context, user *User) (string, error) {
	return s.generateTokenWithOpts(ctx, user, nil)
}

// generateTokenWithOpts generates a signed JWT token for the given user with optional claims including authentication assurance level, authentication method references, and session ID. It invokes the custom_access_token hook if configured, allowing hooks to add custom claims to the token.
func (s *Service) generateTokenWithOpts(ctx context.Context, user *User, opts *tokenOptions) (string, error) {
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
		Email:       user.Email,
		IsAnonymous: user.IsAnonymous,
		AAL:         "aal1",
	}
	if opts != nil {
		if opts.AAL != "" {
			claims.AAL = opts.AAL
		}
		if len(opts.AMR) > 0 {
			claims.AMR = opts.AMR
		}
		if opts.SessionID != "" {
			claims.SessionID = opts.SessionID
		}
		if opts.TenantID != "" {
			claims.TenantID = opts.TenantID
		}
	}

	if s.hookDispatcher != nil {
		claimsJSON, err := json.Marshal(claims)
		if err != nil {
			return "", fmt.Errorf("marshaling claims for hook: %w", err)
		}
		var claimsMap map[string]any
		if err := json.Unmarshal(claimsJSON, &claimsMap); err != nil {
			return "", fmt.Errorf("unmarshaling claims for hook: %w", err)
		}
		hookClaims, err := s.hookDispatcher.CustomAccessToken(ctx, user.ID, claimsMap)
		if err != nil {
			return "", fmt.Errorf("custom_access_token hook failed: %w", err)
		}
		if customClaims, ok := hookClaims["custom_claims"].(map[string]any); ok && len(customClaims) > 0 {
			claims.CustomClaims = customClaims
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	secret, err := s.readJWTSecret()
	if err != nil {
		return "", err
	}
	return token.SignedString(secret)
}

// IssueTestToken generates a JWT for the given user ID and email. Intended for testing.
func (s *Service) IssueTestToken(userID, email string) (string, error) {
	return s.generateToken(context.Background(), &User{ID: userID, Email: email})
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
