package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func FuzzValidateToken(f *testing.F) {
	svc := newTestService()
	user := &User{
		ID:    "550e8400-e29b-41d4-a716-446655440000",
		Email: "fuzz@example.com",
	}
	anonymousUser := &User{
		ID:          "550e8400-e29b-41d4-a716-446655440001",
		IsAnonymous: true,
	}

	validToken, err := svc.generateToken(context.Background(), user)
	if err != nil {
		f.Fatalf("generateToken seed: %v", err)
	}

	anonymousToken, err := svc.generateToken(context.Background(), anonymousUser)
	if err != nil {
		f.Fatalf("generateToken anonymous seed: %v", err)
	}

	optsToken, err := svc.generateTokenWithOpts(context.Background(), user, &tokenOptions{
		AAL:       "aal2",
		AMR:       []string{"password", "totp"},
		SessionID: "session-fuzz",
	})
	if err != nil {
		f.Fatalf("generateTokenWithOpts seed: %v", err)
	}

	mfaToken, err := svc.generateMFAPendingToken(user)
	if err != nil {
		f.Fatalf("generateMFAPendingToken seed: %v", err)
	}

	issuedToken, err := svc.IssueTestToken("user-123", "test@example.com")
	if err != nil {
		f.Fatalf("IssueTestToken seed: %v", err)
	}

	expiredSvc := newTestService()
	expiredSvc.tokenDur = -time.Second
	expiredToken, err := expiredSvc.generateToken(context.Background(), user)
	if err != nil {
		f.Fatalf("expired token seed: %v", err)
	}

	noneClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Email: user.Email,
	}
	noneJWT := jwt.NewWithClaims(jwt.SigningMethodNone, noneClaims)
	noneToken, err := noneJWT.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		f.Fatalf("none signing token seed: %v", err)
	}

	tamperedToken := validToken
	if parts := strings.SplitN(validToken, ".", 3); len(parts) == 3 {
		tamperedToken = parts[0] + "." + parts[1] + ".invalidsignature"
	}

	wrongSecretSvc := &Service{jwtSecret: []byte("different-secret-that-is-also-32-chars-long!!"), tokenDur: time.Hour}
	wrongSecretToken, err := wrongSecretSvc.generateToken(context.Background(), user)
	if err != nil {
		f.Fatalf("wrong secret token seed: %v", err)
	}

	revokedSvc := newTestService()
	revokedSvc.denyList = NewTokenDenyList()
	revokedToken, err := revokedSvc.generateTokenWithOpts(context.Background(), user, &tokenOptions{SessionID: "revoked-session"})
	if err != nil {
		f.Fatalf("revoked token seed: %v", err)
	}
	revokedSvc.denyList.Add("revoked-session", time.Hour)

	seeds := []string{
		validToken,
		anonymousToken,
		optsToken,
		mfaToken,
		issuedToken,
		expiredToken,
		tamperedToken,
		noneToken,
		wrongSecretToken,
		revokedToken,
		"",
		".",
		"..",
		"header.payload",
		"header.payload.signature",
		"\x00",
		"\x00.\x01.\x02",
		"你好.世界.签名",
		strings.Repeat("a", maxJWTTokenLength),
		strings.Repeat("a", maxJWTTokenLength+1),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, tokenString string) {
		if len(tokenString) > 64*1024 {
			t.Skip()
		}

		_, _ = svc.ValidateToken(tokenString)
		_, _ = revokedSvc.ValidateToken(tokenString)
	})
}
