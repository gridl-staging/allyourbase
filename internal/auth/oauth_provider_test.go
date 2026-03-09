package auth

import (
	"context"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestOAuthAccessTokenFormat(t *testing.T) {
	t.Parallel()
	s := &Service{}
	token, err := s.generateOAuthAccessToken()
	testutil.NoError(t, err)
	testutil.True(t, IsOAuthAccessToken(token), "should have access token prefix")
	// ayb_at_ + 64 hex chars (32 bytes)
	testutil.Equal(t, len(OAuthAccessTokenPrefix)+64, len(token))
}

func TestOAuthRefreshTokenFormat(t *testing.T) {
	t.Parallel()
	s := &Service{}
	token, err := s.generateOAuthRefreshToken()
	testutil.NoError(t, err)
	testutil.True(t, IsOAuthRefreshToken(token), "should have refresh token prefix")
	// ayb_rt_ + 96 hex chars (48 bytes)
	testutil.Equal(t, len(OAuthRefreshTokenPrefix)+96, len(token))
}

func TestOAuthProviderModeConfigDefaults(t *testing.T) {
	t.Parallel()
	s := &Service{}
	testutil.Equal(t, DefaultAccessTokenDuration, s.oauthAccessTokenDuration())
	testutil.Equal(t, DefaultRefreshTokenDuration, s.oauthRefreshTokenDuration())
	testutil.Equal(t, DefaultAuthCodeDuration, s.oauthAuthCodeDuration())
}

func TestOAuthProviderModeConfigCustom(t *testing.T) {
	t.Parallel()
	s := &Service{}
	s.oauthProviderCfg = OAuthProviderModeConfig{
		AccessTokenDuration:  30 * time.Minute,
		RefreshTokenDuration: 1 * time.Hour,
		AuthCodeDuration:     5 * time.Minute,
	}
	testutil.Equal(t, 30*time.Minute, s.oauthAccessTokenDuration())
	testutil.Equal(t, 1*time.Hour, s.oauthRefreshTokenDuration())
	testutil.Equal(t, 5*time.Minute, s.oauthAuthCodeDuration())
}

func TestCreateAuthorizationCodeValidation(t *testing.T) {
	t.Parallel()

	// These tests only test validation logic, no DB needed.
	s := &Service{logger: testutil.DiscardLogger()}

	tests := []struct {
		name            string
		challengeMethod string
		challenge       string
		state           string
		wantErrCode     string
	}{
		{"plain method rejected", "plain", "challenge", "state123", OAuthErrInvalidRequest},
		{"empty method rejected", "", "challenge", "state123", OAuthErrInvalidRequest},
		{"missing challenge", "S256", "", "state123", OAuthErrInvalidRequest},
		{"missing state", "S256", "challenge", "", OAuthErrInvalidRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := s.CreateAuthorizationCode(context.TODO(), "cid", "uid", "https://example.com/cb", "readonly", nil, tt.challenge, tt.challengeMethod, tt.state)
			testutil.True(t, err != nil, "expected error")
			oauthErr, ok := err.(*OAuthError)
			testutil.True(t, ok, "expected OAuthError, got %T", err)
			testutil.Equal(t, tt.wantErrCode, oauthErr.Code)
		})
	}
}

