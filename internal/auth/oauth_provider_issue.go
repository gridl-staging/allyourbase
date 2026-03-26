package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) generateOAuthAccessToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating access token: %w", err)
	}
	return OAuthAccessTokenPrefix + hex.EncodeToString(raw), nil
}

func (s *Service) generateOAuthRefreshToken() (string, error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}
	return OAuthRefreshTokenPrefix + hex.EncodeToString(raw), nil
}

func (s *Service) issueOAuthTokenPairTx(ctx context.Context, tx pgx.Tx, clientID string, userID *string, scope string, allowedTables []string) (*OAuthTokenResponse, error) {
	grantID := uuid.New().String()
	return s.issueOAuthTokenPairWithGrantIDTx(ctx, tx, clientID, userID, scope, allowedTables, grantID)
}

// issueOAuthTokenPairWithGrantIDTx generates and stores an access and refresh token pair within a transaction, associating both tokens with the provided grant ID.
func (s *Service) issueOAuthTokenPairWithGrantIDTx(ctx context.Context, tx pgx.Tx, clientID string, userID *string, scope string, allowedTables []string, grantID string) (*OAuthTokenResponse, error) {
	accessToken, err := s.generateOAuthAccessToken()
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.generateOAuthRefreshToken()
	if err != nil {
		return nil, err
	}

	accessHash := hashToken(accessToken)
	refreshHash := hashToken(refreshToken)
	now := time.Now()

	_, err = tx.Exec(ctx,
		`INSERT INTO _ayb_oauth_tokens (token_hash, token_type, client_id, user_id, scope, allowed_tables, grant_id, expires_at)
		 VALUES ($1, 'access', $2, $3, $4, $5, $6, $7)`,
		accessHash, clientID, userID, scope, allowedTables, grantID,
		now.Add(s.oauthAccessTokenDuration()),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting access token: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO _ayb_oauth_tokens (token_hash, token_type, client_id, user_id, scope, allowed_tables, grant_id, expires_at)
		 VALUES ($1, 'refresh', $2, $3, $4, $5, $6, $7)`,
		refreshHash, clientID, userID, scope, allowedTables, grantID,
		now.Add(s.oauthRefreshTokenDuration()),
	)
	if err != nil {
		return nil, fmt.Errorf("inserting refresh token: %w", err)
	}

	return &OAuthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.oauthAccessTokenDuration().Seconds()),
		RefreshToken: refreshToken,
		Scope:        scope,
	}, nil
}
