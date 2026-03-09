package auth

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Client ID format ---

func TestOAuthClientIDPrefix(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, OAuthClientIDPrefix, "ayb_cid_")
}

func TestIsOAuthClientID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"valid client id", "ayb_cid_" + hex.EncodeToString(make([]byte, 24)), true},
		{"prefix plus one char", "ayb_cid_x", false},
		{"wrong length (too short)", "ayb_cid_" + hex.EncodeToString(make([]byte, 23)), false},
		{"wrong length (too long)", "ayb_cid_" + hex.EncodeToString(make([]byte, 25)), false},
		{"uppercase hex not allowed", "ayb_cid_" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", false},
		{"non-hex lowercase chars", "ayb_cid_" + strings.Repeat("g", 48), false},
		{"api key prefix", "ayb_abc123", false},
		{"jwt token", "eyJhbGciOi.abc.def", false},
		{"empty", "", false},
		{"prefix only", "ayb_cid_", false},
		{"wrong prefix", "ayb_client_abc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, IsOAuthClientID(tt.id), tt.want)
		})
	}
}

func TestGenerateClientID(t *testing.T) {
	t.Parallel()

	cid, err := GenerateClientID()
	testutil.NoError(t, err)

	// Should start with prefix.
	testutil.True(t, len(cid) > len(OAuthClientIDPrefix), "client_id too short")
	testutil.Equal(t, cid[:len(OAuthClientIDPrefix)], OAuthClientIDPrefix)

	// Should be prefix + 48 hex chars (24 bytes).
	hexPart := cid[len(OAuthClientIDPrefix):]
	testutil.Equal(t, 48, len(hexPart))
	for _, c := range hexPart {
		testutil.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"hex part should be lowercase hex, got %c", c)
	}

	// Two calls should produce different IDs.
	cid2, err := GenerateClientID()
	testutil.NoError(t, err)
	testutil.True(t, cid != cid2, "two client IDs should differ")
}

func TestGenerateClientSecret(t *testing.T) {
	t.Parallel()

	secret, err := GenerateClientSecret()
	testutil.NoError(t, err)

	// Should start with prefix.
	testutil.True(t, len(secret) > len(OAuthClientSecretPrefix), "secret too short")
	testutil.Equal(t, secret[:len(OAuthClientSecretPrefix)], OAuthClientSecretPrefix)

	// Should be prefix + 64 hex chars (32 bytes).
	hexPart := secret[len(OAuthClientSecretPrefix):]
	testutil.Equal(t, 64, len(hexPart))

	// Two calls should produce different secrets.
	secret2, err := GenerateClientSecret()
	testutil.NoError(t, err)
	testutil.True(t, secret != secret2, "two secrets should differ")
}

func TestHashAndVerifyClientSecret(t *testing.T) {
	t.Parallel()

	secret := "ayb_cs_" + hex.EncodeToString(make([]byte, 32))
	hash := HashClientSecret(secret)

	testutil.True(t, len(hash) == 64, "hash should be 64 hex chars (SHA-256)")
	testutil.True(t, VerifyClientSecret(secret, hash), "correct secret should verify")
	testutil.False(t, VerifyClientSecret("wrong_secret", hash), "wrong secret should not verify")
}

// --- Redirect URI validation ---

func TestValidateRedirectURIs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		uris    []string
		wantErr bool
		errMsg  string
	}{
		{"valid https", []string{"https://example.com/callback"}, false, ""},
		{"valid https with path", []string{"https://app.example.com/oauth/callback"}, false, ""},
		{"valid localhost http", []string{"http://localhost:3000/callback"}, false, ""},
		{"valid 127.0.0.1 http", []string{"http://127.0.0.1:8080/cb"}, false, ""},
		{"valid localhost no port", []string{"http://localhost/callback"}, false, ""},
		{"multiple valid", []string{"https://example.com/cb", "http://localhost:3000/cb"}, false, ""},
		{"empty list", []string{}, true, "at least one redirect URI"},
		{"nil list", nil, true, "at least one redirect URI"},
		{"http non-localhost", []string{"http://example.com/callback"}, true, "HTTPS required"},
		{"has query params", []string{"https://example.com/callback?foo=bar"}, true, "must not contain query"},
		{"has fragment", []string{"https://example.com/callback#frag"}, true, "must not contain fragment"},
		{"not a url", []string{"not-a-url"}, true, "invalid redirect URI"},
		{"empty string in list", []string{""}, true, "invalid redirect URI"},
		{"wildcard not allowed", []string{"https://*.example.com/cb"}, true, "must not contain wildcard"},
		{"ftp scheme rejected", []string{"ftp://example.com/callback"}, true, "must use http or https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRedirectURIs(tt.uris)
			if tt.wantErr {
				testutil.True(t, err != nil, "expected error")
				if tt.errMsg != "" {
					testutil.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				testutil.NoError(t, err)
			}
		})
	}
}

func TestMatchRedirectURI(t *testing.T) {
	t.Parallel()
	registered := []string{
		"https://example.com/callback",
		"http://localhost:3000/callback",
	}
	tests := []struct {
		name  string
		uri   string
		match bool
	}{
		{"exact match https", "https://example.com/callback", true},
		{"exact match localhost", "http://localhost:3000/callback", true},
		{"different path", "https://example.com/other", false},
		{"different port", "http://localhost:4000/callback", false},
		{"different scheme", "http://example.com/callback", false},
		{"with query", "https://example.com/callback?foo=bar", false},
		{"extra slash", "https://example.com/callback/", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, MatchRedirectURI(tt.uri, registered), tt.match)
		})
	}
}

// --- Scope validation ---

func TestValidateOAuthScopes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		scopes  []string
		wantErr bool
	}{
		{"valid readonly", []string{"readonly"}, false},
		{"valid readwrite", []string{"readwrite"}, false},
		{"valid star", []string{"*"}, false},
		{"valid multiple", []string{"readonly", "readwrite"}, false},
		{"empty", []string{}, true},
		{"nil", nil, true},
		{"invalid scope", []string{"admin"}, true},
		{"mix valid and invalid", []string{"readonly", "invalid"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateOAuthScopes(tt.scopes)
			if tt.wantErr {
				testutil.True(t, err != nil, "expected error")
			} else {
				testutil.NoError(t, err)
			}
		})
	}
}

func TestIsScopeSubset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		requested string
		allowed   []string
		want      bool
	}{
		{"readonly in readonly", "readonly", []string{"readonly"}, true},
		{"readonly in readwrite", "readonly", []string{"readwrite"}, false},
		{"readwrite in readwrite", "readwrite", []string{"readwrite"}, true},
		{"star in star", "*", []string{"*"}, true},
		{"readonly in star", "readonly", []string{"*"}, true},
		{"readwrite in star", "readwrite", []string{"*"}, true},
		{"star not in readonly", "*", []string{"readonly"}, false},
		{"readonly in multiple", "readonly", []string{"readonly", "readwrite"}, true},
		{"empty allowed", "readonly", []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, IsScopeSubset(tt.requested, tt.allowed), tt.want)
		})
	}
}

// --- OAuthClient struct ---

func TestOAuthClientTypeConstants(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, OAuthClientTypeConfidential, "confidential")
	testutil.Equal(t, OAuthClientTypePublic, "public")
}

func TestValidateClientType(t *testing.T) {
	t.Parallel()
	testutil.NoError(t, ValidateClientType("confidential"))
	testutil.NoError(t, ValidateClientType("public"))
	testutil.True(t, ValidateClientType("unknown") != nil, "unknown type should error")
	testutil.True(t, ValidateClientType("") != nil, "empty type should error")
}

// --- Token prefix detection ---

func TestIsOAuthAccessToken(t *testing.T) {
	t.Parallel()
	raw := make([]byte, 32)
	rand.Read(raw)
	token := OAuthAccessTokenPrefix + hex.EncodeToString(raw)
	testutil.True(t, IsOAuthAccessToken(token), "should detect access token prefix")
	testutil.False(t, IsOAuthAccessToken("ayb_rt_"+hex.EncodeToString(raw)), "refresh token should not match")
	testutil.False(t, IsOAuthAccessToken("ayb_abc"), "API key should not match")
	testutil.False(t, IsOAuthAccessToken(""), "empty should not match")
}

func TestIsOAuthRefreshToken(t *testing.T) {
	t.Parallel()
	raw := make([]byte, 48)
	rand.Read(raw)
	token := OAuthRefreshTokenPrefix + hex.EncodeToString(raw)
	testutil.True(t, IsOAuthRefreshToken(token), "should detect refresh token prefix")
	testutil.False(t, IsOAuthRefreshToken("ayb_at_"+hex.EncodeToString(raw)), "access token should not match")
}

func TestIsOAuthToken(t *testing.T) {
	t.Parallel()
	testutil.True(t, IsOAuthToken("ayb_at_abc123"), "access token should match")
	testutil.True(t, IsOAuthToken("ayb_rt_abc123"), "refresh token should match")
	testutil.False(t, IsOAuthToken("ayb_abc123"), "API key should not match")
	testutil.False(t, IsOAuthToken("eyJhbGci"), "JWT should not match")
}

// --- Redirect URI localhost port matching ---

func TestLocalhostRedirectURIPortMatching(t *testing.T) {
	t.Parallel()
	// Per RFC 8252 §7.3, localhost redirect URIs should use exact match including port.
	registered := []string{"http://localhost:3000/callback"}

	testutil.True(t, MatchRedirectURI("http://localhost:3000/callback", registered),
		"exact port match should work")
	testutil.False(t, MatchRedirectURI("http://localhost:4000/callback", registered),
		"different port should not match")
}

// --- PKCE helpers ---

func TestPKCEVerifyS256(t *testing.T) {
	t.Parallel()
	// A known test vector: verifier "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	// When S256 hashed should produce a specific challenge.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := GeneratePKCEChallenge(verifier)

	testutil.True(t, len(challenge) > 0, "challenge should not be empty")
	testutil.True(t, VerifyPKCE(verifier, challenge, "S256"), "S256 verification should pass")
	testutil.False(t, VerifyPKCE("wrong_verifier", challenge, "S256"), "wrong verifier should fail")
	testutil.False(t, VerifyPKCE(verifier, challenge, "plain"), "plain method should be rejected")
	testutil.False(t, VerifyPKCE(verifier, challenge, ""), "empty method should be rejected")
}

func TestGeneratePKCEChallenge(t *testing.T) {
	t.Parallel()
	verifier := "test_verifier_string_that_is_long_enough"
	c1 := GeneratePKCEChallenge(verifier)
	c2 := GeneratePKCEChallenge(verifier)
	testutil.Equal(t, c1, c2) // deterministic

	c3 := GeneratePKCEChallenge("different_verifier")
	testutil.True(t, c1 != c3, "different verifiers should produce different challenges")
}

func TestPKCEChallengeIsBase64URL(t *testing.T) {
	t.Parallel()
	challenge := GeneratePKCEChallenge("my_code_verifier_for_testing")
	// RFC 7636 requires base64url encoding without padding.
	for _, c := range challenge {
		valid := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_'
		testutil.True(t, valid, "challenge should be base64url, got %c", c)
	}
	// No padding.
	testutil.True(t, len(challenge) > 0 && challenge[len(challenge)-1] != '=',
		"challenge should not have padding")
}

// --- RFC 6749 §5.2 error codes ---

func TestOAuthErrorCodes(t *testing.T) {
	t.Parallel()
	// Verify the error code constants match RFC 6749 §5.2.
	testutil.Equal(t, OAuthErrInvalidRequest, "invalid_request")
	testutil.Equal(t, OAuthErrInvalidClient, "invalid_client")
	testutil.Equal(t, OAuthErrInvalidGrant, "invalid_grant")
	testutil.Equal(t, OAuthErrUnauthorizedClient, "unauthorized_client")
	testutil.Equal(t, OAuthErrUnsupportedGrantType, "unsupported_grant_type")
	testutil.Equal(t, OAuthErrInvalidScope, "invalid_scope")
	testutil.Equal(t, OAuthErrAccessDenied, "access_denied")
}

func TestNewOAuthError(t *testing.T) {
	t.Parallel()
	e := NewOAuthError(OAuthErrInvalidRequest, "missing parameter")
	testutil.Equal(t, e.Code, "invalid_request")
	testutil.Equal(t, e.Description, "missing parameter")
	testutil.Contains(t, e.Error(), "invalid_request")
	testutil.Contains(t, e.Error(), "missing parameter")
}

