package auth

import (
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const testSecret = "test-secret-that-is-at-least-32-characters-long!!"

func init() {
	// Use minimal argon2id params in unit tests for speed.
	// Production params (64 MiB, 3 iterations) take ~250ms per hash.
	argonMemory = 1024 // 1 MiB
	argonTime = 1
}

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()
	hash, err := hashPassword("mypassword123")
	testutil.NoError(t, err)
	testutil.True(t, len(hash) > 0, "hash should not be empty")
	testutil.Contains(t, hash, "$argon2id$")

	ok, err := verifyPassword(hash, "mypassword123")
	testutil.NoError(t, err)
	testutil.True(t, ok, "correct password should verify")
}

func TestVerifyPasswordWrong(t *testing.T) {
	t.Parallel()
	hash, err := hashPassword("mypassword123")
	testutil.NoError(t, err)

	ok, err := verifyPassword(hash, "wrongpassword")
	testutil.NoError(t, err)
	testutil.False(t, ok, "wrong password should not verify")
}

func TestVerifyPasswordUnsupportedFormat(t *testing.T) {
	t.Parallel()
	ok, err := verifyPassword("not-a-valid-hash", "password")
	testutil.False(t, ok, "should return false for unsupported format")
	testutil.ErrorContains(t, err, "unsupported hash format")
}

func TestVerifyBcryptPassword(t *testing.T) {
	t.Parallel()
	hash, err := bcrypt.GenerateFromPassword([]byte("mypassword123"), bcrypt.MinCost)
	testutil.NoError(t, err)

	ok, err := verifyPassword(string(hash), "mypassword123")
	testutil.NoError(t, err)
	testutil.True(t, ok, "correct password should verify with bcrypt")
}

func TestVerifyBcryptPasswordWrong(t *testing.T) {
	t.Parallel()
	hash, err := bcrypt.GenerateFromPassword([]byte("mypassword123"), bcrypt.MinCost)
	testutil.NoError(t, err)

	ok, err := verifyPassword(string(hash), "wrongpassword")
	testutil.NoError(t, err)
	testutil.False(t, ok, "wrong password should not verify with bcrypt")
}

func TestVerifyBcrypt2bPrefix(t *testing.T) {
	// Go's bcrypt generates $2a$ hashes; verify $2b$ variant also works.
	t.Parallel()

	hash, err := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.MinCost)
	testutil.NoError(t, err)
	hashStr := strings.Replace(string(hash), "$2a$", "$2b$", 1)

	ok, err := verifyPassword(hashStr, "testpass")
	testutil.NoError(t, err)
	testutil.True(t, ok, "$2b$ prefix should verify")
}

func TestIsBcryptHash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		hash string
		want bool
	}{
		{"$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01", true},
		{"$2b$12$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01", true},
		{"$2y$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01", true},
		{"$argon2id$v=19$m=1024,t=1,p=1$salt$key", false},
		{"plaintext", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.hash[:min(len(tt.hash), 4)], func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, isBcryptHash(tt.hash))
		})
	}
}

func TestGenerateAndValidateToken(t *testing.T) {
	t.Parallel()
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  time.Hour,
	}

	user := &User{
		ID:    "550e8400-e29b-41d4-a716-446655440000",
		Email: "test@example.com",
	}

	token, err := svc.generateToken(user)
	testutil.NoError(t, err)
	testutil.True(t, len(token) > 0, "token should not be empty")

	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, user.ID, claims.Subject)
	testutil.Equal(t, user.Email, claims.Email)
	testutil.NotNil(t, claims.ExpiresAt)
	testutil.NotNil(t, claims.IssuedAt)

	// Verify token duration is correct (~1 hour).
	dur := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	testutil.True(t, dur >= 59*time.Minute && dur <= 61*time.Minute,
		"token duration should be ~1 hour, got %v", dur)
}

func TestValidateTokenExpired(t *testing.T) {
	t.Parallel()
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  -time.Hour, // expired immediately
	}

	user := &User{ID: "test-id", Email: "test@example.com"}
	token, err := svc.generateToken(user)
	testutil.NoError(t, err)

	_, err = svc.ValidateToken(token)
	testutil.ErrorContains(t, err, "token is expired")
}

func TestValidateTokenTampered(t *testing.T) {
	t.Parallel()
	svc := &Service{
		jwtSecret: []byte(testSecret),
		tokenDur:  time.Hour,
	}

	user := &User{ID: "test-id", Email: "test@example.com"}
	token, err := svc.generateToken(user)
	testutil.NoError(t, err)

	// Tamper with the token by replacing the signature.
	parts := strings.SplitN(token, ".", 3)
	tampered := parts[0] + "." + parts[1] + ".invalidsignature"
	_, err = svc.ValidateToken(tampered)
	testutil.ErrorContains(t, err, "invalid token")
}

func TestValidateTokenWrongSigningMethod(t *testing.T) {
	// Create a token signed with a different method (none).
	t.Parallel()

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "test-id",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Email: "test@example.com",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	testutil.NoError(t, err)

	svc := &Service{jwtSecret: []byte(testSecret)}
	_, err = svc.ValidateToken(tokenString)
	testutil.ErrorContains(t, err, "unexpected signing method")
}

func TestValidateTokenWrongSecret(t *testing.T) {
	t.Parallel()
	svc1 := &Service{jwtSecret: []byte(testSecret), tokenDur: time.Hour}
	svc2 := &Service{jwtSecret: []byte("different-secret-that-is-also-32-chars-long!!")}

	user := &User{ID: "test-id", Email: "test@example.com"}
	token, err := svc1.generateToken(user)
	testutil.NoError(t, err)

	_, err = svc2.ValidateToken(token)
	testutil.ErrorContains(t, err, "invalid token")
}

func TestValidateEmail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		email   string
		wantErr string
	}{
		{"valid", "user@example.com", ""},
		{"valid subdomain", "user@mail.example.com", ""},
		{"empty", "", "email is required"},
		{"no at", "userexample.com", "invalid email format"},
		{"no domain dot", "user@example", "invalid email format"},
		{"at at start", "@example.com", "invalid email format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateEmail(tt.email)
			if tt.wantErr == "" {
				testutil.NoError(t, err)
			} else {
				testutil.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		password string
		minLen   int
		wantErr  string
	}{
		{"valid 8 chars", "12345678", 8, ""},
		{"valid long", "a-very-long-secure-password", 8, ""},
		{"too short", "1234567", 8, "at least 8 characters"},
		{"empty", "", 8, "password is required"},
		{"single char with min 1", "x", 1, ""},
		{"empty with min 1", "", 1, "password is required"},
		{"3 chars with min 3", "abc", 3, ""},
		{"2 chars with min 3", "ab", 3, "at least 3 characters"},
		{"min 0 defaults to 8", "1234567", 0, "at least 8 characters"},
		{"min negative defaults to 8", "1234567", -1, "at least 8 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validatePassword(tt.password, tt.minLen)
			if tt.wantErr == "" {
				testutil.NoError(t, err)
			} else {
				testutil.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestPasswordHashUniqueSalt(t *testing.T) {
	t.Parallel()
	h1, err := hashPassword("same-password")
	testutil.NoError(t, err)
	h2, err := hashPassword("same-password")
	testutil.NoError(t, err)
	testutil.NotEqual(t, h1, h2)
}

func TestVerifyPasswordCorruptedHash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		hash    string
		wantErr string
	}{
		{"corrupted salt", "$argon2id$v=19$m=1024,t=1,p=1$!!!invalid-base64$keypart", "decoding salt"},
		{"corrupted key", "$argon2id$v=19$m=1024,t=1,p=1$dGVzdHNhbHQ$!!!invalid", "decoding key"},
		{"invalid params", "$argon2id$v=19$garbage$c2FsdA==$a2V5", "parsing hash params"},
		{"wrong part count", "$argon2id$v=19$m=1024,t=1,p=1$salt", "invalid argon2id hash format"},
		{"wrong algorithm prefix", "$argon2d$v=19$m=1024,t=1,p=1$salt$key", "unsupported hash format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ok, err := verifyPassword(tt.hash, "password")
			testutil.False(t, ok, "corrupted hash should return false")
			testutil.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestRotateJWTSecretChangesSecret(t *testing.T) {
	t.Parallel()
	svc := &Service{jwtSecret: []byte(testSecret), tokenDur: time.Hour}

	// Issue a token with the old secret.
	user := &User{ID: "test-id", Email: "test@example.com"}
	oldToken, err := svc.generateToken(user)
	testutil.NoError(t, err)

	// Validate works before rotation.
	_, err = svc.ValidateToken(oldToken)
	testutil.NoError(t, err)

	// Rotate secret.
	newSecret, err := svc.RotateJWTSecret()
	testutil.NoError(t, err)
	testutil.Equal(t, 64, len(newSecret))

	// Verify returned secret is valid hex.
	_, hexErr := hex.DecodeString(newSecret)
	testutil.NoError(t, hexErr)

	// Old token should no longer validate.
	_, err = svc.ValidateToken(oldToken)
	testutil.ErrorContains(t, err, "invalid token")

	// New token should validate.
	newToken, err := svc.generateToken(user)
	testutil.NoError(t, err)
	claims, err := svc.ValidateToken(newToken)
	testutil.NoError(t, err)
	testutil.Equal(t, "test-id", claims.Subject)
}

func TestRotateJWTSecretProducesDifferentSecrets(t *testing.T) {
	t.Parallel()
	svc := &Service{jwtSecret: []byte(testSecret), tokenDur: time.Hour}

	s1, err := svc.RotateJWTSecret()
	testutil.NoError(t, err)
	s2, err := svc.RotateJWTSecret()
	testutil.NoError(t, err)
	testutil.NotEqual(t, s1, s2)
}

// TestRotateJWTSecretConcurrentSafe verifies that concurrent RotateJWTSecret
// and ValidateToken calls are data-race-free. Run with -race to validate the
// jwtSecretMu RWMutex properly protects concurrent access.
func TestRotateJWTSecretConcurrentSafe(t *testing.T) {
	t.Parallel()
	svc := &Service{jwtSecret: []byte(testSecret), tokenDur: time.Hour}
	user := &User{ID: "concurrent-test", Email: "concurrent@example.com"}

	var wg sync.WaitGroup
	const workers = 8

	// Issue a token before the goroutines start so validators have something to work with.
	initialToken, err := svc.generateToken(user)
	testutil.NoError(t, err)

	// Goroutines that continuously rotate the secret.
	for i := 0; i < workers/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, _ = svc.RotateJWTSecret()
			}
		}()
	}

	// Goroutines that continuously generate and validate tokens.
	// Validation may fail (wrong secret after rotation) — that's expected.
	// What we're checking is no data race, not that all validations succeed.
	for i := 0; i < workers/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				tok, err := svc.generateToken(user)
				if err == nil && tok != "" {
					_, _ = svc.ValidateToken(tok)
				}
				// Also validate the initial token (may have been invalidated by rotation).
				_, _ = svc.ValidateToken(initialToken)
			}
		}()
	}

	wg.Wait()
}

func TestIssueTestToken(t *testing.T) {
	t.Parallel()
	svc := &Service{jwtSecret: []byte(testSecret), tokenDur: time.Hour}

	token, err := svc.IssueTestToken("user-123", "test@example.com")
	testutil.NoError(t, err)
	testutil.True(t, token != "", "token should not be empty")

	claims, err := svc.ValidateToken(token)
	testutil.NoError(t, err)
	testutil.Equal(t, "user-123", claims.Subject)
	testutil.Equal(t, "test@example.com", claims.Email)
	testutil.NotNil(t, claims.ExpiresAt)
	testutil.NotNil(t, claims.IssuedAt)

	// Verify expiration is roughly 1 hour from issuance.
	dur := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	testutil.True(t, dur >= 59*time.Minute && dur <= 61*time.Minute,
		"token duration should be ~1 hour, got %v", dur)
}

// TestValidateTokenBoundaryConditions tests JWT validation at expiry boundaries.
// Uses generous margins to avoid flaky failures under load.
func TestValidateTokenBoundaryConditions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		tokenDur   time.Duration
		waitBefore time.Duration
		wantErr    string
	}{
		{
			name:       "well before expiry - valid",
			tokenDur:   2 * time.Second,
			waitBefore: 10 * time.Millisecond,
			wantErr:    "",
		},
		{
			name:       "after expiry - expired",
			tokenDur:   100 * time.Millisecond,
			waitBefore: 300 * time.Millisecond,
			wantErr:    "token is expired",
		},
		{
			name:       "already expired at issuance",
			tokenDur:   -time.Second,
			waitBefore: 0,
			wantErr:    "token is expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := &Service{
				jwtSecret: []byte(testSecret),
				tokenDur:  tt.tokenDur,
			}

			user := &User{ID: "test-id", Email: "test@example.com"}
			token, err := svc.generateToken(user)
			testutil.NoError(t, err)

			if tt.waitBefore > 0 {
				time.Sleep(tt.waitBefore)
			}

			_, err = svc.ValidateToken(token)
			if tt.wantErr == "" {
				testutil.NoError(t, err)
			} else {
				testutil.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

// TestHashTokenDeterministic verifies that hashToken produces deterministic output.
func TestHashTokenDeterministic(t *testing.T) {
	t.Parallel()
	h1 := hashToken("test-token-value")
	h2 := hashToken("test-token-value")
	testutil.Equal(t, h1, h2)
	testutil.Equal(t, 64, len(h1)) // SHA-256 hex = 64 chars
}

// TestHashTokenDifferentInputs verifies that different inputs produce different hashes.
func TestHashTokenDifferentInputs(t *testing.T) {
	t.Parallel()
	h1 := hashToken("token-a")
	h2 := hashToken("token-b")
	testutil.NotEqual(t, h1, h2)
}
