package auth

import (
	"context"
	"time"
)

type providerTokenResolverSetter interface {
	SetOAuthResolver(ProviderTokenOAuthResolver)
}

func (s *Service) SetProviderTokenStore(store ProviderTokenStorage) {
	if s == nil {
		return
	}

	s.providerTokenStore = store
	s.applyProviderTokenResolver()
}

func (s *Service) SetProviderTokenResolver(resolver ProviderTokenOAuthResolver) {
	if s == nil {
		return
	}

	s.providerTokenResolver = resolver
	s.applyProviderTokenResolver()
}

func (s *Service) applyProviderTokenResolver() {
	if s == nil || s.providerTokenStore == nil || s.providerTokenResolver == nil {
		return
	}

	if target, ok := s.providerTokenStore.(providerTokenResolverSetter); ok {
		target.SetOAuthResolver(s.providerTokenResolver)
	}
}

func (s *Service) StoreProviderTokens(ctx context.Context, userID, provider, accessToken, refreshToken, tokenType, scopes string, expiresAt *time.Time) error {
	if s == nil || s.providerTokenStore == nil {
		return ErrProviderTokenStoreNotConfigured
	}
	return s.providerTokenStore.StoreTokens(ctx, userID, provider, accessToken, refreshToken, tokenType, scopes, expiresAt)
}

func (s *Service) GetProviderToken(ctx context.Context, userID, provider string) (string, error) {
	if s == nil || s.providerTokenStore == nil {
		return "", ErrProviderTokenStoreNotConfigured
	}
	return s.providerTokenStore.GetProviderToken(ctx, userID, provider)
}

func (s *Service) ListProviderTokens(ctx context.Context, userID string) ([]ProviderTokenInfo, error) {
	if s == nil || s.providerTokenStore == nil {
		return nil, ErrProviderTokenStoreNotConfigured
	}
	return s.providerTokenStore.ListProviderTokens(ctx, userID)
}

func (s *Service) DeleteProviderToken(ctx context.Context, userID, provider string) error {
	if s == nil || s.providerTokenStore == nil {
		return ErrProviderTokenStoreNotConfigured
	}
	return s.providerTokenStore.DeleteProviderToken(ctx, userID, provider)
}

func (s *Service) RefreshExpiringProviderTokens(ctx context.Context, window time.Duration) error {
	if s == nil || s.providerTokenStore == nil {
		return ErrProviderTokenStoreNotConfigured
	}
	return s.providerTokenStore.RefreshExpiringProviderTokens(ctx, window)
}
