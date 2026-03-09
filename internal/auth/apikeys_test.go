package auth

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsAPIKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{"valid api key prefix", "ayb_abc123def456", true},
		{"just prefix plus one char", "ayb_x", true},
		{"full length key", "ayb_" + hex.EncodeToString(make([]byte, 24)), true},
		{"jwt token", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abc.def", false},
		{"empty string", "", false},
		{"prefix only no content", "ayb_", false},
		{"similar but wrong prefix", "ayb-abc123", false},
		{"random string", "someothertoken", false},
		{"case sensitive prefix", "AYB_abc123", false},
		{"partial prefix", "ayb", false},
		{"whitespace token", " ayb_abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, IsAPIKey(tt.token), tt.want)
		})
	}
}

// TestAPIKeyPrefix removed — pure constant check, behaviorally covered by TestIsAPIKey.

func TestAPIKeyFormat(t *testing.T) {
	// Verify that a generated key has the expected format.
	t.Parallel()

	raw := make([]byte, apiKeyRawBytes)
	_, err := rand.Read(raw)
	testutil.NoError(t, err)

	plaintext := APIKeyPrefix + hex.EncodeToString(raw)
	testutil.Equal(t, 52, len(plaintext))
	testutil.True(t, IsAPIKey(plaintext), "generated key should pass IsAPIKey")

	// Key should start with "ayb_" prefix.
	testutil.True(t, plaintext[:4] == "ayb_", "key should start with ayb_")
	// Remaining 48 chars should be hex (lowercase letters and digits).
	hexPart := plaintext[4:]
	for _, c := range hexPart {
		testutil.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"hex part should only contain hex chars, got %c", c)
	}
}

func TestAPIKeyUniqueness(t *testing.T) {
	// Two generated keys should be different.
	t.Parallel()

	raw1 := make([]byte, apiKeyRawBytes)
	_, err := rand.Read(raw1)
	testutil.NoError(t, err)

	raw2 := make([]byte, apiKeyRawBytes)
	_, err = rand.Read(raw2)
	testutil.NoError(t, err)

	key1 := APIKeyPrefix + hex.EncodeToString(raw1)
	key2 := APIKeyPrefix + hex.EncodeToString(raw2)
	testutil.True(t, key1 != key2, "two generated keys should not collide")
}

func TestAPIKeyHashConsistency(t *testing.T) {
	// Same plaintext should produce the same hash.
	t.Parallel()

	key := "ayb_abcdef1234567890abcdef1234567890abcdef12345678"
	hash1 := hashToken(key)
	hash2 := hashToken(key)
	testutil.Equal(t, hash1, hash2)

	// Different plaintexts should produce different hashes.
	otherKey := "ayb_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	otherHash := hashToken(otherKey)
	testutil.True(t, hash1 != otherHash, "different keys should have different hashes")
}

func TestAPIKeyErrorSentinels(t *testing.T) {
	// Verify error sentinel values are distinct and have meaningful messages.
	t.Parallel()

	testutil.True(t, ErrAPIKeyNotFound != ErrAPIKeyRevoked, "not found != revoked")
	testutil.True(t, ErrAPIKeyNotFound != ErrAPIKeyExpired, "not found != expired")
	testutil.True(t, ErrAPIKeyRevoked != ErrAPIKeyExpired, "revoked != expired")
	testutil.True(t, ErrInvalidScope != ErrAPIKeyNotFound, "invalid scope != not found")

	testutil.Contains(t, ErrAPIKeyNotFound.Error(), "not found")
	testutil.Contains(t, ErrAPIKeyRevoked.Error(), "revoked")
	testutil.Contains(t, ErrAPIKeyExpired.Error(), "expired")
	testutil.Contains(t, ErrInvalidScope.Error(), "invalid scope")
}

// --- Scope tests ---

func TestValidScopes(t *testing.T) {
	t.Parallel()
	testutil.True(t, ValidScopes[ScopeFullAccess], "* should be valid")
	testutil.True(t, ValidScopes[ScopeReadOnly], "readonly should be valid")
	testutil.True(t, ValidScopes[ScopeReadWrite], "readwrite should be valid")
	testutil.True(t, !ValidScopes["admin"], "admin should not be valid")
	testutil.True(t, !ValidScopes[""], "empty should not be valid")
	testutil.True(t, !ValidScopes["READONLY"], "uppercase should not be valid")
}

// TestScopeConstants removed — pure constant checks, behaviorally covered by
// TestClaimsIsReadAllowed, TestClaimsIsWriteAllowed, TestCheckWriteScope.

func TestClaimsIsReadAllowed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		scope string
		want  bool
	}{
		{"empty scope (JWT)", "", true},
		{"full access", "*", true},
		{"readonly", "readonly", true},
		{"readwrite", "readwrite", true},
		{"invalid scope", "bogus", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &Claims{APIKeyScope: tt.scope}
			testutil.Equal(t, tt.want, c.IsReadAllowed())
		})
	}
}

func TestClaimsIsWriteAllowed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		scope string
		want  bool
	}{
		{"empty scope (JWT)", "", true},
		{"full access", "*", true},
		{"readwrite", "readwrite", true},
		{"readonly blocks writes", "readonly", false},
		{"invalid scope blocks writes", "bogus", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &Claims{APIKeyScope: tt.scope}
			testutil.Equal(t, tt.want, c.IsWriteAllowed())
		})
	}
}

func TestClaimsIsTableAllowed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tables []string
		table  string
		want   bool
	}{
		{"no restrictions", nil, "posts", true},
		{"empty restrictions", []string{}, "posts", true},
		{"allowed table", []string{"posts", "comments"}, "posts", true},
		{"allowed second table", []string{"posts", "comments"}, "comments", true},
		{"denied table", []string{"posts", "comments"}, "users", false},
		{"single table allowed", []string{"posts"}, "posts", true},
		{"single table denied", []string{"posts"}, "comments", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &Claims{AllowedTables: tt.tables}
			testutil.Equal(t, tt.want, c.IsTableAllowed(tt.table))
		})
	}
}

func TestCheckWriteScope(t *testing.T) {
	// nil claims should pass (no-auth mode)
	t.Parallel()

	testutil.NoError(t, CheckWriteScope(nil))

	// JWT claims (no scope) should pass
	testutil.NoError(t, CheckWriteScope(&Claims{}))

	// Full access should pass
	testutil.NoError(t, CheckWriteScope(&Claims{APIKeyScope: "*"}))

	// Readwrite should pass
	testutil.NoError(t, CheckWriteScope(&Claims{APIKeyScope: "readwrite"}))

	// Readonly should fail
	err := CheckWriteScope(&Claims{APIKeyScope: "readonly"})
	testutil.Equal(t, ErrScopeReadOnly, err)
}

func TestCheckTableScope(t *testing.T) {
	// nil claims should pass
	t.Parallel()

	testutil.NoError(t, CheckTableScope(nil, "posts"))

	// No restrictions should pass
	testutil.NoError(t, CheckTableScope(&Claims{}, "posts"))

	// Allowed table should pass
	testutil.NoError(t, CheckTableScope(&Claims{AllowedTables: []string{"posts"}}, "posts"))

	// Denied table should fail
	err := CheckTableScope(&Claims{AllowedTables: []string{"posts"}}, "users")
	testutil.Equal(t, ErrScopeTableDenied, err)
}

func TestCheckWriteScopeInvalidScope(t *testing.T) {
	// An unrecognized scope should deny writes (fail closed).
	t.Parallel()

	err := CheckWriteScope(&Claims{APIKeyScope: "bogus"})
	testutil.Equal(t, ErrScopeReadOnly, err)
}

func TestCheckTableScopeCaseSensitive(t *testing.T) {
	// Table matching should be case-sensitive.
	t.Parallel()

	c := &Claims{AllowedTables: []string{"Posts"}}
	testutil.True(t, c.IsTableAllowed("Posts"), "exact case should match")
	testutil.True(t, !c.IsTableAllowed("posts"), "lowercase should not match uppercase restriction")
}

func TestApplyAppRateLimitClaimsPopulatesFields(t *testing.T) {
	t.Parallel()

	claims := &Claims{}
	appID := "550e8400-e29b-41d4-a716-446655440000"
	rps := 25
	window := 90

	applyAppRateLimitClaims(claims, &appID, &rps, &window)

	testutil.Equal(t, appID, claims.AppID)
	testutil.Equal(t, 25, claims.AppRateLimitRPS)
	testutil.Equal(t, 90, claims.AppRateLimitWindow)
}

func TestApplyAppRateLimitClaimsWithoutAppLeavesDefaults(t *testing.T) {
	t.Parallel()

	claims := &Claims{
		AppID:              "",
		AppRateLimitRPS:    0,
		AppRateLimitWindow: 0,
	}
	rps := 10
	window := 60

	applyAppRateLimitClaims(claims, nil, &rps, &window)

	testutil.Equal(t, "", claims.AppID)
	testutil.Equal(t, 0, claims.AppRateLimitRPS)
	testutil.Equal(t, 0, claims.AppRateLimitWindow)
}

func TestMapCreateAPIKeyInsertErrorAppFK(t *testing.T) {
	t.Parallel()

	err := mapCreateAPIKeyInsertError(&pgconn.PgError{
		Code:           "23503",
		ConstraintName: "_ayb_api_keys_app_id_fkey",
	})
	testutil.Equal(t, ErrInvalidAppID, err)
}

func TestMapCreateAPIKeyInsertErrorUserFK(t *testing.T) {
	t.Parallel()

	err := mapCreateAPIKeyInsertError(&pgconn.PgError{
		Code:           "23503",
		ConstraintName: "_ayb_api_keys_user_id_fkey",
	})
	testutil.Equal(t, ErrUserNotFound, err)
}

func TestMapCreateAPIKeyInsertErrorInvalidUUID(t *testing.T) {
	t.Parallel()

	err := mapCreateAPIKeyInsertError(&pgconn.PgError{
		Code: "22P02",
	})
	testutil.Equal(t, ErrInvalidAppID, err)
}

func TestMapCreateAPIKeyInsertErrorPassthrough(t *testing.T) {
	t.Parallel()

	wrapped := mapCreateAPIKeyInsertError(&pgconn.PgError{Code: "23505"})
	testutil.True(t, wrapped != nil, "error should be wrapped")
	testutil.Contains(t, wrapped.Error(), "inserting api key")
}
