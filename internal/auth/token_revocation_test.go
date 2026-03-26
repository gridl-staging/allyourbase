package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestValidateTokenRevokedSessionReturnsErrTokenRevoked(t *testing.T) {
	t.Parallel()

	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  time.Hour,
		denyList:  NewTokenDenyList(),
	}

	token, err := svc.generateTokenWithOpts(context.Background(), &User{
		ID:    "user-1",
		Email: "user-1@example.com",
	}, &tokenOptions{SessionID: "session-1"})
	testutil.NoError(t, err)

	svc.denyList.Add("session-1", time.Hour)

	_, err = svc.ValidateToken(token)
	testutil.True(t, errors.Is(err, ErrTokenRevoked), "expected ErrTokenRevoked")
}

func TestValidateTokenDifferentSessionIDStillValid(t *testing.T) {
	t.Parallel()

	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  time.Hour,
		denyList:  NewTokenDenyList(),
	}

	token, err := svc.generateTokenWithOpts(context.Background(), &User{
		ID:    "user-1",
		Email: "user-1@example.com",
	}, &tokenOptions{SessionID: "session-1"})
	testutil.NoError(t, err)

	svc.denyList.Add("session-2", time.Hour)

	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, "session-1", claims.SessionID)
}

func TestValidateTokenEmptySessionIDUnaffectedByDenyList(t *testing.T) {
	t.Parallel()

	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  time.Hour,
		denyList:  NewTokenDenyList(),
	}

	token, err := svc.generateToken(context.Background(), &User{
		ID:    "user-1",
		Email: "user-1@example.com",
	})
	testutil.NoError(t, err)

	svc.denyList.Add("session-1", time.Hour)

	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, "", claims.SessionID)
}
