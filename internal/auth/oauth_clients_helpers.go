package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// OAuthError is an RFC 6749 §5.2 error response.
type OAuthError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *OAuthError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Description)
}

// NewOAuthError creates a new RFC 6749 error.
func NewOAuthError(code, description string) *OAuthError {
	return &OAuthError{Code: code, Description: description}
}

// GenerateClientID generates a new OAuth client ID: ayb_cid_ + 24 random hex bytes.
func GenerateClientID() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating client id: %w", err)
	}
	return OAuthClientIDPrefix + hex.EncodeToString(raw), nil
}

// GenerateClientSecret generates a new OAuth client secret: ayb_cs_ + 32 random hex bytes.
func GenerateClientSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating client secret: %w", err)
	}
	return OAuthClientSecretPrefix + hex.EncodeToString(raw), nil
}

// IsOAuthClientID returns true if the string looks like an OAuth client ID.
func IsOAuthClientID(s string) bool {
	const expectedHexLen = 48 // 24 bytes
	if len(s) != len(OAuthClientIDPrefix)+expectedHexLen {
		return false
	}
	if !strings.HasPrefix(s, OAuthClientIDPrefix) {
		return false
	}
	for _, c := range s[len(OAuthClientIDPrefix):] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// IsOAuthAccessToken returns true if the token has the access token prefix.
func IsOAuthAccessToken(s string) bool {
	return len(s) > len(OAuthAccessTokenPrefix) && s[:len(OAuthAccessTokenPrefix)] == OAuthAccessTokenPrefix
}

// IsOAuthRefreshToken returns true if the token has the refresh token prefix.
func IsOAuthRefreshToken(s string) bool {
	return len(s) > len(OAuthRefreshTokenPrefix) && s[:len(OAuthRefreshTokenPrefix)] == OAuthRefreshTokenPrefix
}

// IsOAuthToken returns true if the token is either an OAuth access or refresh token.
func IsOAuthToken(s string) bool {
	return IsOAuthAccessToken(s) || IsOAuthRefreshToken(s)
}

// HashClientSecret hashes a client secret with SHA-256 for storage.
func HashClientSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// VerifyClientSecret checks a plaintext secret against a stored hash.
func VerifyClientSecret(secret, hash string) bool {
	computed := sha256.Sum256([]byte(secret))
	computedHex := hex.EncodeToString(computed[:])
	return subtle.ConstantTimeCompare([]byte(computedHex), []byte(hash)) == 1
}

// ValidateRedirectURIs validates a list of redirect URIs per RFC 6749 and RFC 8252.
// Rules: HTTPS required (except localhost), no query params, no fragments, no wildcards, exact match.
func ValidateRedirectURIs(uris []string) error {
	if len(uris) == 0 {
		return fmt.Errorf("at least one redirect URI is required")
	}
	for _, raw := range uris {
		if raw == "" {
			return fmt.Errorf("invalid redirect URI: empty string")
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("invalid redirect URI: %s", raw)
		}
		if u.RawQuery != "" {
			return fmt.Errorf("redirect URI must not contain query parameters: %s", raw)
		}
		if u.Fragment != "" {
			return fmt.Errorf("redirect URI must not contain fragment: %s", raw)
		}
		if strings.Contains(u.Host, "*") {
			return fmt.Errorf("redirect URI must not contain wildcard: %s", raw)
		}
		// Allow HTTP only for localhost/127.0.0.1 (development).
		host := u.Hostname()
		if u.Scheme == "http" && host != "localhost" && host != "127.0.0.1" {
			return fmt.Errorf("HTTPS required for non-localhost redirect URI: %s", raw)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("redirect URI must use http or https scheme: %s", raw)
		}
	}
	return nil
}

// MatchRedirectURI checks if a redirect URI exactly matches one of the registered URIs.
func MatchRedirectURI(uri string, registered []string) bool {
	for _, r := range registered {
		if uri == r {
			return true
		}
	}
	return false
}

// ValidateOAuthScopes validates that all scopes are in the allowed set.
func ValidateOAuthScopes(scopes []string) error {
	if len(scopes) == 0 {
		return fmt.Errorf("at least one scope is required")
	}
	for _, s := range scopes {
		if !ValidScopes[s] {
			return fmt.Errorf("invalid scope: %s (must be one of: readonly, readwrite, *)", s)
		}
	}
	return nil
}

// IsScopeSubset returns true if the requested scope is contained in the allowed scopes.
// The "*" scope contains all others.
func IsScopeSubset(requested string, allowed []string) bool {
	for _, a := range allowed {
		if a == ScopeFullAccess || a == requested {
			return true
		}
	}
	return false
}

// ValidateClientType validates the OAuth client type.
func ValidateClientType(ct string) error {
	if ct != OAuthClientTypeConfidential && ct != OAuthClientTypePublic {
		return fmt.Errorf("invalid client type: %s (must be confidential or public)", ct)
	}
	return nil
}

// GeneratePKCEChallenge computes S256(verifier) as base64url-no-pad.
func GeneratePKCEChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// VerifyPKCE checks a code_verifier against a stored code_challenge using S256.
// Only S256 is supported (plain is rejected per RFC 9700).
func VerifyPKCE(verifier, challenge, method string) bool {
	if method != "S256" {
		return false
	}
	computed := GeneratePKCEChallenge(verifier)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}
