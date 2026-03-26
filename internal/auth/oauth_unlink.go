package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrOAuthLinkNotFound indicates no linked OAuth identity matched the request.
var ErrOAuthLinkNotFound = errors.New("oauth identity link not found")

type oauthIdentityStore interface {
	DeleteOAuthIdentity(ctx context.Context, userID, provider string) (bool, error)
}

type dbOAuthIdentityStore struct {
	pool *pgxpool.Pool
}

func (s *dbOAuthIdentityStore) DeleteOAuthIdentity(ctx context.Context, userID, provider string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, errors.New("database pool is not configured")
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_oauth_accounts WHERE user_id = $1 AND provider = $2`,
		userID, provider,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// SetOAuthIdentityStore overrides OAuth identity persistence behavior.
func (s *Service) SetOAuthIdentityStore(store oauthIdentityStore) {
	s.oauthIdentityStore = store
}

// UnlinkOAuth deletes one linked OAuth identity for a user and cascades provider-token cleanup.
func (s *Service) UnlinkOAuth(ctx context.Context, userID, provider string) error {
	userID = strings.TrimSpace(userID)
	provider = strings.TrimSpace(provider)
	if userID == "" || provider == "" {
		return fmt.Errorf("%w: user_id and provider are required", ErrValidation)
	}

	store := s.oauthIdentityStore
	if store == nil {
		store = &dbOAuthIdentityStore{pool: s.pool}
	}

	deleted, err := store.DeleteOAuthIdentity(ctx, userID, provider)
	if err != nil {
		return fmt.Errorf("unlinking OAuth identity: %w", err)
	}
	if !deleted {
		return ErrOAuthLinkNotFound
	}

	if s.providerTokenStore != nil {
		err := s.providerTokenStore.DeleteProviderToken(ctx, userID, provider)
		if err != nil && !errors.Is(err, ErrProviderTokenNotFound) && !errors.Is(err, ErrProviderTokenStoreNotConfigured) {
			return fmt.Errorf("deleting provider token on unlink: %w", err)
		}
	}

	if s.logger != nil {
		s.logger.Info("OAuth identity unlinked", "user_id", userID, "provider", provider)
	}
	return nil
}
