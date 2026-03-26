// Package auth oauth_apple.go implements Apple Sign In OAuth with client secret generation, id_token verification, and JWKS key caching.
package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	appleIssuer  = "https://appleid.apple.com"
	appleJWKSURL = "https://appleid.apple.com/auth/keys"
)

// AppleClientSecretParams holds the parameters needed to generate an Apple client_secret JWT.
type AppleClientSecretParams struct {
	TeamID     string
	ClientID   string // Apple Services ID
	KeyID      string
	PrivateKey string // PEM-encoded EC private key
}

// GenerateAppleClientSecret creates a signed JWT to use as the client_secret
// when exchanging authorization codes with Apple's token endpoint.
// See: https://developer.apple.com/documentation/sign_in_with_apple/generate_and_validate_tokens
func GenerateAppleClientSecret(params AppleClientSecretParams) (string, error) {
	if params.TeamID == "" {
		return "", fmt.Errorf("apple: team_id is required")
	}
	if params.ClientID == "" {
		return "", fmt.Errorf("apple: client_id is required")
	}
	if params.KeyID == "" {
		return "", fmt.Errorf("apple: key_id is required")
	}
	if params.PrivateKey == "" {
		return "", fmt.Errorf("apple: private_key is required")
	}

	key, err := parseES256PrivateKey(params.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("apple: %w", err)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": params.TeamID,
		"sub": params.ClientID,
		"aud": appleIssuer,
		"iat": now.Unix(),
		"exp": now.Add(6 * 30 * 24 * time.Hour).Unix(), // 6-month max
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = params.KeyID

	return token.SignedString(key)
}

// parseES256PrivateKey decodes a PEM-encoded ECDSA private key, attempting PKCS8 format first before falling back to EC private key format.
func parseES256PrivateKey(pemStr string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try PKCS8 first, then EC private key format.
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		ecKey, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS8 key is not ECDSA")
		}
		return ecKey, nil
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing EC private key: %w", err)
	}
	return key, nil
}

// AppleJWKSFetcher fetches and caches Apple's JSON Web Key Set for id_token verification.
type AppleJWKSFetcher struct {
	url        string
	ttl        time.Duration
	httpClient *http.Client

	mu        sync.RWMutex
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
}

// NewAppleJWKSFetcher creates a JWKS fetcher with the given URL and cache TTL.
func NewAppleJWKSFetcher(jwksURL string, ttl time.Duration) *AppleJWKSFetcher {
	return &AppleJWKSFetcher{
		url: jwksURL,
		ttl: ttl,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		keys: make(map[string]*ecdsa.PublicKey),
	}
}

// GetKey returns the public key for the given key ID, fetching from Apple if needed.
// If the key is in cache and the cache isn't expired, returns immediately.
// If the key is missing, refreshes even within TTL (key rotation scenario).
func (f *AppleJWKSFetcher) GetKey(kid string) (*ecdsa.PublicKey, error) {
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
		return nil, fmt.Errorf("apple JWKS: key %q not found", kid)
	}
	return key, nil
}

// refresh fetches Apple's JSON Web Key Set from the configured URL, parses it into ECDSA public keys, and updates the internal cache with the current timestamp.
func (f *AppleJWKSFetcher) refresh() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	client := f.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Get(f.url)
	if err != nil {
		return fmt.Errorf("fetching Apple JWKS: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading Apple JWKS: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("apple JWKS returned %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("parsing Apple JWKS: %w", err)
	}

	keys := make(map[string]*ecdsa.PublicKey, len(jwks.Keys))
	for _, rawKey := range jwks.Keys {
		kid, pub, err := parseJWKToECDSA(rawKey)
		if err != nil {
			continue // skip non-EC keys
		}
		keys[kid] = pub
	}

	f.keys = keys
	f.fetchedAt = time.Now()
	return nil
}

// parseJWKToECDSA parses a JSON Web Key into its key ID and ECDSA public key, validating that it is an EC key with P-256 curve.
func parseJWKToECDSA(raw json.RawMessage) (string, *ecdsa.PublicKey, error) {
	var jwk struct {
		KTY string `json:"kty"`
		KID string `json:"kid"`
		CRV string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}
	if err := json.Unmarshal(raw, &jwk); err != nil {
		return "", nil, err
	}
	if jwk.KTY != "EC" || jwk.CRV != "P-256" {
		return "", nil, fmt.Errorf("unsupported key type: %s/%s", jwk.KTY, jwk.CRV)
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		return "", nil, fmt.Errorf("decoding x: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		return "", nil, fmt.Errorf("decoding y: %w", err)
	}

	pub := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}
	return jwk.KID, pub, nil
}

// VerifyAppleIDToken verifies an Apple id_token JWT and extracts user info.
func VerifyAppleIDToken(idToken, clientID string, fetcher *AppleJWKSFetcher) (*OAuthUserInfo, error) {
	// Parse without verification first to get the kid from the header.
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"ES256"}),
		jwt.WithIssuer(appleIssuer),
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
		// Translate jwt library errors to more readable messages.
		errStr := err.Error()
		switch {
		case strings.Contains(errStr, "token is expired"):
			return nil, fmt.Errorf("apple id_token expired: %w", err)
		case strings.Contains(errStr, "audience"):
			return nil, fmt.Errorf("apple id_token audience mismatch: %w", err)
		case strings.Contains(errStr, "issuer"):
			return nil, fmt.Errorf("apple id_token issuer mismatch: %w", err)
		}
		return nil, fmt.Errorf("apple id_token verification failed: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("apple id_token: unexpected claims type")
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("apple id_token: missing sub claim")
	}

	email, _ := claims["email"].(string)

	return &OAuthUserInfo{
		ProviderUserID: sub,
		Email:          email,
	}, nil
}

// ParseAppleUserPayload parses the first-auth user JSON that Apple sends
// only on the initial authorization. Subsequent logins only return the id_token.
func ParseAppleUserPayload(userJSON string) (*OAuthUserInfo, error) {
	var payload struct {
		Name struct {
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
		} `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal([]byte(userJSON), &payload); err != nil {
		return nil, fmt.Errorf("parsing Apple user payload: %w", err)
	}

	name := strings.TrimSpace(payload.Name.FirstName + " " + payload.Name.LastName)

	return &OAuthUserInfo{
		Email: payload.Email,
		Name:  name,
	}, nil
}
