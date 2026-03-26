// Package auth Provides OpenID Connect discovery, JSON Web Key Set fetching and caching, and dynamic OIDC provider registration for OAuth authentication.
package auth

import (
	"crypto"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// OAuthUserInfoSourceIDTokenWithEndpointFallback tries the id_token first,
// then falls back to the userinfo endpoint if the id_token is missing or
// doesn't contain an email address.
const OAuthUserInfoSourceIDTokenWithEndpointFallback OAuthUserInfoSource = "id_token_with_endpoint_fallback"

var builtInOAuthProviderNames = map[string]struct{}{
	"google":    {},
	"github":    {},
	"microsoft": {},
	"apple":     {},
	"discord":   {},
	"twitter":   {},
	"facebook":  {},
	"linkedin":  {},
	"spotify":   {},
	"twitch":    {},
	"gitlab":    {},
	"bitbucket": {},
	"slack":     {},
	"zoom":      {},
	"notion":    {},
	"figma":     {},
}

func isBuiltInOAuthProviderName(name string) bool {
	_, ok := builtInOAuthProviderNames[name]
	return ok
}

// IsBuiltInOAuthProviderName reports whether name matches a built-in provider.
func IsBuiltInOAuthProviderName(name string) bool {
	return isBuiltInOAuthProviderName(name)
}

// OIDCDiscoveryDocument holds the parsed OpenID Connect discovery metadata
// from {issuer}/.well-known/openid-configuration.
type OIDCDiscoveryDocument struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	UserInfoEndpoint                 string   `json:"userinfo_endpoint"`
	JWKSURI                          string   `json:"jwks_uri"`
	ScopesSupported                  []string `json:"scopes_supported"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
}

// ValidateDiscoveryDocument checks that all required OIDC fields are present.
// Returns an error if issuer, authorization_endpoint, or token_endpoint is missing.
// A missing userinfo_endpoint is not an error (some providers omit it).
func ValidateDiscoveryDocument(doc *OIDCDiscoveryDocument) error {
	if doc.Issuer == "" {
		return fmt.Errorf("OIDC discovery: missing required field \"issuer\"")
	}
	if doc.AuthorizationEndpoint == "" {
		return fmt.Errorf("OIDC discovery: missing required field \"authorization_endpoint\"")
	}
	if doc.TokenEndpoint == "" {
		return fmt.Errorf("OIDC discovery: missing required field \"token_endpoint\"")
	}
	return nil
}

// DiscoveryDocumentWarnings returns non-fatal discovery warnings.
func DiscoveryDocumentWarnings(doc *OIDCDiscoveryDocument) []string {
	if doc == nil {
		return nil
	}
	if strings.TrimSpace(doc.UserInfoEndpoint) == "" {
		return []string{
			"OIDC discovery: missing optional field \"userinfo_endpoint\"; relying on id_token claims only",
		}
	}
	return nil
}

// FetchOIDCDiscovery fetches and parses the OIDC discovery document for an issuer.
// The issuer URL should not include a trailing slash or the well-known path.
func FetchOIDCDiscovery(issuerURL string, httpClient *http.Client) (*OIDCDiscoveryDocument, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	discoveryURL := strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
	resp, err := httpClient.Get(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("fetching OIDC discovery from %s: %w", discoveryURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading OIDC discovery response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OIDC discovery endpoint %s returned %d", discoveryURL, resp.StatusCode)
	}

	var doc OIDCDiscoveryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parsing OIDC discovery document: %w", err)
	}

	if err := ValidateDiscoveryDocument(&doc); err != nil {
		return nil, err
	}

	return &doc, nil
}

// OIDCDiscoveryCache caches discovery documents by issuer URL with TTL-based expiry.
type OIDCDiscoveryCache struct {
	mu         sync.RWMutex
	entries    map[string]*oidcDiscoveryCacheEntry
	ttl        time.Duration
	httpClient *http.Client
}

type oidcDiscoveryCacheEntry struct {
	doc       *OIDCDiscoveryDocument
	fetchedAt time.Time
}

// NewOIDCDiscoveryCache creates a cache with the given TTL.
func NewOIDCDiscoveryCache(ttl time.Duration) *OIDCDiscoveryCache {
	return &OIDCDiscoveryCache{
		entries: make(map[string]*oidcDiscoveryCacheEntry),
		ttl:     ttl,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetHTTPClient overrides the HTTP client used for discovery fetches (for testing).
func (c *OIDCDiscoveryCache) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// Get returns the cached discovery document for the issuer, fetching it if the
// cache is empty or expired.
func (c *OIDCDiscoveryCache) Get(issuerURL string) (*OIDCDiscoveryDocument, error) {
	key := strings.TrimRight(issuerURL, "/")

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if ok && time.Since(entry.fetchedAt) < c.ttl {
		return entry.doc, nil
	}

	// Cache miss or expired — fetch.
	doc, err := FetchOIDCDiscovery(issuerURL, c.httpClient)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[key] = &oidcDiscoveryCacheEntry{
		doc:       doc,
		fetchedAt: time.Now(),
	}
	c.mu.Unlock()

	return doc, nil
}

// Invalidate removes a cached entry (e.g., on JWKS verification failure).
func (c *OIDCDiscoveryCache) Invalidate(issuerURL string) {
	key := strings.TrimRight(issuerURL, "/")
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// JWKSFetcher fetches and caches JSON Web Key Sets. Supports both RSA and EC keys.
type JWKSFetcher struct {
	url        string
	ttl        time.Duration
	httpClient *http.Client

	mu        sync.RWMutex
	keys      map[string]crypto.PublicKey
	fetchedAt time.Time
}

// NewJWKSFetcher creates a JWKS fetcher with the given URL and cache TTL.
func NewJWKSFetcher(jwksURL string, ttl time.Duration) *JWKSFetcher {
	return &JWKSFetcher{
		url: jwksURL,
		ttl: ttl,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		keys: make(map[string]crypto.PublicKey),
	}
}

// SetHTTPClient overrides the HTTP client for testing.
func (f *JWKSFetcher) SetHTTPClient(client *http.Client) {
	f.httpClient = client
}

// GetKey returns the public key for the given key ID, fetching from the JWKS
// endpoint if needed. If the key is missing from cache, it refreshes even
// within TTL (key rotation scenario).
func (f *JWKSFetcher) GetKey(kid string) (crypto.PublicKey, error) {
	f.mu.RLock()
	key, found := f.keys[kid]
	expired := f.fetchedAt.IsZero() || time.Since(f.fetchedAt) >= f.ttl
	f.mu.RUnlock()

	if found && !expired {
		return key, nil
	}

	// Cache miss or expired — refresh.
	if err := f.refresh(); err != nil {
		return nil, err
	}

	f.mu.RLock()
	defer f.mu.RUnlock()
	key, ok := f.keys[kid]
	if !ok {
		return nil, fmt.Errorf("JWKS: key %q not found", kid)
	}
	return key, nil
}

// refresh fetches the JSON Web Key Set from the endpoint, parses each key, and updates the cached keys map with a fresh timestamp. The mutex lock is held for the entire operation.
func (f *JWKSFetcher) refresh() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	client := f.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Get(f.url)
	if err != nil {
		return fmt.Errorf("fetching JWKS from %s: %w", f.url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading JWKS response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("parsing JWKS: %w", err)
	}

	keys := make(map[string]crypto.PublicKey, len(jwks.Keys))
	for _, rawKey := range jwks.Keys {
		kid, pub, err := parseJWK(rawKey)
		if err != nil {
			continue // skip unsupported key types
		}
		keys[kid] = pub
	}

	f.keys = keys
	f.fetchedAt = time.Now()
	return nil
}

// parseJWK parses a single JWK into a public key. Supports RSA and EC keys.
func parseJWK(raw json.RawMessage) (string, crypto.PublicKey, error) {
	var header struct {
		KTY string `json:"kty"`
		KID string `json:"kid"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return "", nil, err
	}

	switch header.KTY {
	case "RSA":
		return parseJWKToRSA(raw)
	case "EC":
		kid, ecKey, err := parseJWKToECDSA(raw)
		if err != nil {
			return "", nil, err
		}
		return kid, ecKey, nil
	default:
		return "", nil, fmt.Errorf("unsupported key type: %s", header.KTY)
	}
}

// parseJWKToRSA parses an RSA JWK into an *rsa.PublicKey.
func parseJWKToRSA(raw json.RawMessage) (string, crypto.PublicKey, error) {
	var jwk struct {
		KID string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	if err := json.Unmarshal(raw, &jwk); err != nil {
		return "", nil, err
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return "", nil, fmt.Errorf("decoding RSA n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return "", nil, fmt.Errorf("decoding RSA e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())

	pub := &rsa.PublicKey{
		N: n,
		E: e,
	}
	return jwk.KID, pub, nil
}

var ErrOIDCIDTokenVerificationFailed = errors.New("OIDC id_token verification failed")

// VerifyOIDCIDToken verifies an OIDC id_token JWT and extracts standard claims.
// It validates the signature against JWKS keys, and checks issuer, audience,
// expiry, and (when provided) nonce.
func VerifyOIDCIDToken(idToken, issuer, clientID, expectedNonce string, fetcher *JWKSFetcher) (*OAuthUserInfo, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(clientID),
		jwt.WithExpirationRequired(),
	)

	token, err := parser.Parse(idToken, func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, fmt.Errorf("missing kid in id_token header")
		}
		return fetcher.GetKey(kid)
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOIDCIDTokenVerificationFailed, err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("OIDC id_token: unexpected claims type")
	}

	if expectedNonce != "" {
		nonce, _ := claims["nonce"].(string)
		if nonce == "" {
			return nil, fmt.Errorf("OIDC id_token: missing nonce claim")
		}
		if nonce != expectedNonce {
			return nil, fmt.Errorf("OIDC id_token: nonce mismatch")
		}
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("OIDC id_token: missing sub claim")
	}

	email, _ := claims["email"].(string)
	name, _ := claims["name"].(string)
	if name == "" {
		name, _ = claims["preferred_username"].(string)
	}

	return &OAuthUserInfo{
		ProviderUserID: sub,
		Email:          email,
		Name:           name,
	}, nil
}

// parseOIDCUserInfo parses a standard OIDC userinfo endpoint response.
func parseOIDCUserInfo(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing OIDC userinfo: %w", err)
	}
	if u.Sub == "" {
		return nil, fmt.Errorf("%w: missing user ID (sub)", ErrOAuthProviderError)
	}
	name := u.Name
	if name == "" {
		name = u.PreferredUsername
	}
	return &OAuthUserInfo{
		ProviderUserID: u.Sub,
		Email:          u.Email,
		Name:           name,
	}, nil
}
