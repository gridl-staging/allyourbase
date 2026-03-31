package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// RegisterOAuthClient creates a new OAuth client linked to an app.
// Returns the plaintext client_secret (shown once) for confidential clients, empty for public.
func (s *Service) RegisterOAuthClient(ctx context.Context, appID, name, clientType string, redirectURIs, scopes []string) (string, *OAuthClient, error) {
	if name == "" {
		return "", nil, ErrOAuthClientNameRequired
	}
	if appID == "" {
		return "", nil, ErrOAuthAppRequired
	}
	if err := ValidateClientType(clientType); err != nil {
		return "", nil, err
	}
	if err := ValidateRedirectURIs(redirectURIs); err != nil {
		return "", nil, err
	}
	if err := ValidateOAuthScopes(scopes); err != nil {
		return "", nil, err
	}

	clientID, err := GenerateClientID()
	if err != nil {
		return "", nil, err
	}

	var secretPlaintext string
	var secretHash *string
	if clientType == OAuthClientTypeConfidential {
		secretPlaintext, err = GenerateClientSecret()
		if err != nil {
			return "", nil, err
		}
		h := HashClientSecret(secretPlaintext)
		secretHash = &h
	}

	var client OAuthClient
	err = s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_oauth_clients (app_id, client_id, client_secret_hash, name, redirect_uris, scopes, client_type)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, app_id, client_id, name, redirect_uris, scopes, client_type, created_at, updated_at, revoked_at`,
		appID, clientID, secretHash, name, redirectURIs, scopes, clientType,
	).Scan(&client.ID, &client.AppID, &client.ClientID, &client.Name,
		&client.RedirectURIs, &client.Scopes, &client.ClientType,
		&client.CreatedAt, &client.UpdatedAt, &client.RevokedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23503": // foreign_key_violation
				return "", nil, ErrAppNotFound
			case "22P02": // invalid_text_representation (bad UUID)
				return "", nil, ErrAppNotFound
			}
		}
		return "", nil, fmt.Errorf("inserting oauth client: %w", err)
	}

	s.logger.Info("oauth client registered", "client_id", client.ClientID, "app_id", appID, "type", clientType)
	return secretPlaintext, &client, nil
}

// GetOAuthClient retrieves an OAuth client by its client_id string (not UUID).
func (s *Service) GetOAuthClient(ctx context.Context, clientID string) (*OAuthClient, error) {
	var client OAuthClient
	err := s.pool.QueryRow(ctx,
		`SELECT id, app_id, client_id, name, redirect_uris, scopes, client_type, created_at, updated_at, revoked_at
		 FROM _ayb_oauth_clients WHERE client_id = $1`,
		clientID,
	).Scan(&client.ID, &client.AppID, &client.ClientID, &client.Name,
		&client.RedirectURIs, &client.Scopes, &client.ClientType,
		&client.CreatedAt, &client.UpdatedAt, &client.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOAuthClientNotFound
		}
		return nil, fmt.Errorf("querying oauth client: %w", err)
	}
	return &client, nil
}

// GetOAuthClientByUUID retrieves an OAuth client by its internal UUID.
func (s *Service) GetOAuthClientByUUID(ctx context.Context, id string) (*OAuthClient, error) {
	var client OAuthClient
	err := s.pool.QueryRow(ctx,
		`SELECT id, app_id, client_id, name, redirect_uris, scopes, client_type, created_at, updated_at, revoked_at
		 FROM _ayb_oauth_clients WHERE id = $1`,
		id,
	).Scan(&client.ID, &client.AppID, &client.ClientID, &client.Name,
		&client.RedirectURIs, &client.Scopes, &client.ClientType,
		&client.CreatedAt, &client.UpdatedAt, &client.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOAuthClientNotFound
		}
		return nil, fmt.Errorf("querying oauth client: %w", err)
	}
	return &client, nil
}

// ListOAuthClients returns a paginated list of all OAuth clients.
func (s *Service) ListOAuthClients(ctx context.Context, page, perPage int) (*OAuthClientListResult, error) {
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
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_oauth_clients`).Scan(&totalItems)
	if err != nil {
		return nil, fmt.Errorf("counting oauth clients: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT
			c.id,
			c.app_id,
			c.client_id,
			c.name,
			c.redirect_uris,
			c.scopes,
			c.client_type,
			c.created_at,
			c.updated_at,
			c.revoked_at,
			COALESCE(stats.active_access_token_count, 0) AS active_access_token_count,
			COALESCE(stats.active_refresh_token_count, 0) AS active_refresh_token_count,
			COALESCE(stats.total_grants, 0) AS total_grants,
			stats.last_token_issued_at
		FROM _ayb_oauth_clients c
		LEFT JOIN (
			SELECT
				client_id,
				COUNT(*) FILTER (
					WHERE token_type = 'access' AND revoked_at IS NULL AND expires_at > NOW()
				) AS active_access_token_count,
				COUNT(*) FILTER (
					WHERE token_type = 'refresh' AND revoked_at IS NULL AND expires_at > NOW()
				) AS active_refresh_token_count,
				COUNT(DISTINCT grant_id) AS total_grants,
				MAX(created_at) AS last_token_issued_at
			FROM _ayb_oauth_tokens
			GROUP BY client_id
		) stats ON stats.client_id = c.client_id
		ORDER BY c.created_at DESC
		LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing oauth clients: %w", err)
	}
	defer rows.Close()

	var items []OAuthClient
	for rows.Next() {
		var c OAuthClient
		if err := rows.Scan(&c.ID, &c.AppID, &c.ClientID, &c.Name,
			&c.RedirectURIs, &c.Scopes, &c.ClientType,
			&c.CreatedAt, &c.UpdatedAt, &c.RevokedAt,
			&c.ActiveAccessTokenCount, &c.ActiveRefreshTokenCount,
			&c.TotalGrants, &c.LastTokenIssuedAt); err != nil {
			return nil, fmt.Errorf("scanning oauth client: %w", err)
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating oauth clients: %w", err)
	}
	if items == nil {
		items = []OAuthClient{}
	}

	totalPages := totalItems / perPage
	if totalItems%perPage != 0 {
		totalPages++
	}

	return &OAuthClientListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}, nil
}

// UpdateOAuthClient updates a non-revoked OAuth client's name, redirect URIs, and scopes.
// Client type and app association are immutable — use delete + recreate to change those.
func (s *Service) UpdateOAuthClient(ctx context.Context, clientID, name string, redirectURIs, scopes []string) (*OAuthClient, error) {
	if name == "" {
		return nil, ErrOAuthClientNameRequired
	}
	if err := ValidateRedirectURIs(redirectURIs); err != nil {
		return nil, err
	}
	if err := ValidateOAuthScopes(scopes); err != nil {
		return nil, err
	}

	// Pre-check: distinguish "not found" from "revoked" for clear error messages.
	existing, err := s.GetOAuthClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if existing.RevokedAt != nil {
		return nil, ErrOAuthClientRevoked
	}

	var client OAuthClient
	err = s.pool.QueryRow(ctx,
		`UPDATE _ayb_oauth_clients
		 SET name = $2, redirect_uris = $3, scopes = $4, updated_at = NOW()
		 WHERE client_id = $1 AND revoked_at IS NULL
		 RETURNING id, app_id, client_id, name, redirect_uris, scopes, client_type, created_at, updated_at, revoked_at`,
		clientID, name, redirectURIs, scopes,
	).Scan(&client.ID, &client.AppID, &client.ClientID, &client.Name,
		&client.RedirectURIs, &client.Scopes, &client.ClientType,
		&client.CreatedAt, &client.UpdatedAt, &client.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOAuthClientNotFound
		}
		return nil, fmt.Errorf("updating oauth client: %w", err)
	}

	s.logger.Info("oauth client updated", "client_id", clientID)
	return &client, nil
}

// RevokeOAuthClient soft-deletes an OAuth client by setting revoked_at.
func (s *Service) RevokeOAuthClient(ctx context.Context, clientID string) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE _ayb_oauth_clients SET revoked_at = NOW(), updated_at = NOW()
		 WHERE client_id = $1 AND revoked_at IS NULL`,
		clientID,
	)
	if err != nil {
		return fmt.Errorf("revoking oauth client: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrOAuthClientNotFound
	}
	s.logger.Info("oauth client revoked", "client_id", clientID)
	return nil
}

// RegenerateOAuthClientSecret generates a new secret for a confidential client.
// Returns the plaintext secret (shown once).
func (s *Service) RegenerateOAuthClientSecret(ctx context.Context, clientID string) (string, error) {
	// Check client exists and is confidential.
	client, err := s.GetOAuthClient(ctx, clientID)
	if err != nil {
		return "", err
	}
	if client.RevokedAt != nil {
		return "", ErrOAuthClientRevoked
	}
	if client.ClientType != OAuthClientTypeConfidential {
		return "", ErrOAuthClientPublicSecretRotator
	}

	newSecret, err := GenerateClientSecret()
	if err != nil {
		return "", err
	}
	newHash := HashClientSecret(newSecret)

	result, err := s.pool.Exec(ctx,
		`UPDATE _ayb_oauth_clients SET client_secret_hash = $1, updated_at = NOW()
		 WHERE client_id = $2 AND revoked_at IS NULL`,
		newHash, clientID,
	)
	if err != nil {
		return "", fmt.Errorf("updating client secret: %w", err)
	}
	if result.RowsAffected() == 0 {
		return "", ErrOAuthClientNotFound
	}

	s.logger.Info("oauth client secret regenerated", "client_id", clientID)
	return newSecret, nil
}

// ValidateOAuthClientCredentials validates client_id + client_secret.
// Returns the client if valid, or an error.
func (s *Service) ValidateOAuthClientCredentials(ctx context.Context, clientID, clientSecret string) (*OAuthClient, error) {
	var client OAuthClient
	var secretHash *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, app_id, client_id, client_secret_hash, name, redirect_uris, scopes, client_type, created_at, updated_at, revoked_at
		 FROM _ayb_oauth_clients WHERE client_id = $1`,
		clientID,
	).Scan(&client.ID, &client.AppID, &client.ClientID, &secretHash, &client.Name,
		&client.RedirectURIs, &client.Scopes, &client.ClientType,
		&client.CreatedAt, &client.UpdatedAt, &client.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, NewOAuthError(OAuthErrInvalidClient, "unknown client")
		}
		return nil, fmt.Errorf("querying oauth client: %w", err)
	}

	if client.RevokedAt != nil {
		return nil, NewOAuthError(OAuthErrInvalidClient, "client has been revoked")
	}

	if client.ClientType == OAuthClientTypeConfidential {
		if secretHash == nil || clientSecret == "" {
			return nil, NewOAuthError(OAuthErrInvalidClient, "client authentication required")
		}
		if !VerifyClientSecret(clientSecret, *secretHash) {
			return nil, NewOAuthError(OAuthErrInvalidClient, "invalid client credentials")
		}
	}

	return &client, nil
}
