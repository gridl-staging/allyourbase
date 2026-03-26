package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

// --- Apple client_secret JWT generation ---

func generateTestES256Key(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating test ES256 key: %v", err)
	}
	derBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshaling test key: %v", err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: derBytes,
	})
	return key, string(pemBlock)
}

func TestGenerateAppleClientSecret(t *testing.T) {
	t.Parallel()
	key, pemStr := generateTestES256Key(t)

	secret, err := GenerateAppleClientSecret(AppleClientSecretParams{
		TeamID:     "TEAM123456",
		ClientID:   "com.example.app",
		KeyID:      "KEY123",
		PrivateKey: pemStr,
	})
	testutil.NoError(t, err)
	testutil.True(t, secret != "", "client secret should not be empty")

	// Parse and verify the JWT.
	token, err := jwt.Parse(secret, func(token *jwt.Token) (interface{}, error) {
		return &key.PublicKey, nil
	})
	testutil.NoError(t, err)
	testutil.True(t, token.Valid, "JWT should be valid")

	// Verify claims.
	claims, ok := token.Claims.(jwt.MapClaims)
	testutil.True(t, ok, "claims should be MapClaims")
	testutil.Equal(t, "TEAM123456", claims["iss"].(string))
	testutil.Equal(t, "com.example.app", claims["sub"].(string))
	testutil.Equal(t, "https://appleid.apple.com", claims["aud"].(string))

	// Verify signing method.
	testutil.Equal(t, "ES256", token.Header["alg"].(string))
	testutil.Equal(t, "KEY123", token.Header["kid"].(string))

	// Verify expiry is ~6 months from now.
	exp, err := claims.GetExpirationTime()
	testutil.NoError(t, err)
	sixMonths := time.Now().Add(6 * 30 * 24 * time.Hour)
	testutil.True(t, exp.Time.After(time.Now().Add(5*30*24*time.Hour)),
		"expiry should be at least 5 months from now")
	testutil.True(t, exp.Time.Before(sixMonths.Add(24*time.Hour)),
		"expiry should be at most 6 months + 1 day from now")
}

func TestGenerateAppleClientSecret_InvalidPEM(t *testing.T) {
	t.Parallel()
	_, err := GenerateAppleClientSecret(AppleClientSecretParams{
		TeamID:     "TEAM123",
		ClientID:   "com.example.app",
		KeyID:      "KEY123",
		PrivateKey: "not-a-valid-pem",
	})
	testutil.NotNil(t, err)
	testutil.ErrorContains(t, err, "PEM")
}

func TestGenerateAppleClientSecret_WrongKeyType(t *testing.T) {
	t.Parallel()
	// RSA PEM instead of EC.
	rsaPEM := "-----BEGIN RSA PRIVATE KEY-----\nMIIBogIBAAJBANDiE2+Xi/WnO+s120NiiJhNyIButVXr94Nt/BZi4QGs\n-----END RSA PRIVATE KEY-----\n"
	_, err := GenerateAppleClientSecret(AppleClientSecretParams{
		TeamID:     "TEAM123",
		ClientID:   "com.example.app",
		KeyID:      "KEY123",
		PrivateKey: rsaPEM,
	})
	testutil.NotNil(t, err)
}

func TestGenerateAppleClientSecret_MissingParams(t *testing.T) {
	t.Parallel()
	_, pemStr := generateTestES256Key(t)

	tests := []struct {
		name   string
		params AppleClientSecretParams
		errMsg string
	}{
		{"missing team ID", AppleClientSecretParams{ClientID: "a", KeyID: "b", PrivateKey: pemStr}, "team_id"},
		{"missing client ID", AppleClientSecretParams{TeamID: "a", KeyID: "b", PrivateKey: pemStr}, "client_id"},
		{"missing key ID", AppleClientSecretParams{TeamID: "a", ClientID: "b", PrivateKey: pemStr}, "key_id"},
		{"missing private key", AppleClientSecretParams{TeamID: "a", ClientID: "b", KeyID: "c"}, "private_key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := GenerateAppleClientSecret(tt.params)
			testutil.NotNil(t, err)
			testutil.ErrorContains(t, err, tt.errMsg)
		})
	}
}

// --- Apple id_token verification ---

func TestVerifyAppleIDToken(t *testing.T) {
	t.Parallel()
	key, _ := generateTestES256Key(t)

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwks := buildTestJWKS(t, &key.PublicKey, "test-kid")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(jwksServer.Close)

	now := time.Now()
	tests := []struct {
		name      string
		claims    jwt.MapClaims
		audience  string
		wantErr   string
		wantSubID string
		wantEmail string
	}{
		{
			name: "valid token",
			claims: jwt.MapClaims{
				"iss": "https://appleid.apple.com", "aud": "com.example.app",
				"sub": "apple-user-001", "email": "user@icloud.com",
				"exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
			},
			audience: "com.example.app", wantSubID: "apple-user-001", wantEmail: "user@icloud.com",
		},
		{
			name: "expired token",
			claims: jwt.MapClaims{
				"iss": "https://appleid.apple.com", "aud": "com.example.app",
				"sub": "apple-user-001", "email": "user@icloud.com",
				"exp": now.Add(-time.Hour).Unix(), "iat": now.Add(-2 * time.Hour).Unix(),
			},
			audience: "com.example.app", wantErr: "expired",
		},
		{
			name: "wrong audience",
			claims: jwt.MapClaims{
				"iss": "https://appleid.apple.com", "aud": "com.wrong.app",
				"sub": "apple-user-001", "email": "user@icloud.com",
				"exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
			},
			audience: "com.example.app", wantErr: "audience",
		},
		{
			name: "wrong issuer",
			claims: jwt.MapClaims{
				"iss": "https://evil.com", "aud": "com.example.app",
				"sub": "apple-user-001", "email": "user@icloud.com",
				"exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
			},
			audience: "com.example.app", wantErr: "issuer",
		},
		{
			name: "missing sub",
			claims: jwt.MapClaims{
				"iss": "https://appleid.apple.com", "aud": "com.example.app",
				"email": "user@icloud.com",
				"exp":   now.Add(time.Hour).Unix(), "iat": now.Unix(),
			},
			audience: "com.example.app", wantErr: "sub",
		},
	}

	fetcher := NewAppleJWKSFetcher(jwksServer.URL, 24*time.Hour)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			token := jwt.NewWithClaims(jwt.SigningMethodES256, tt.claims)
			token.Header["kid"] = "test-kid"
			idToken, err := token.SignedString(key)
			testutil.NoError(t, err)

			info, err := VerifyAppleIDToken(idToken, tt.audience, fetcher)
			if tt.wantErr != "" {
				testutil.NotNil(t, err)
				testutil.ErrorContains(t, err, tt.wantErr)
				return
			}
			testutil.NoError(t, err)
			testutil.Equal(t, tt.wantSubID, info.ProviderUserID)
			testutil.Equal(t, tt.wantEmail, info.Email)
		})
	}
}

// --- JWKS fetcher caching ---

func TestAppleJWKSFetcher_CachesKeys(t *testing.T) {
	t.Parallel()
	key, _ := generateTestES256Key(t)

	fetchCount := 0
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		jwks := buildTestJWKS(t, &key.PublicKey, "cached-kid")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer jwksServer.Close()

	fetcher := NewAppleJWKSFetcher(jwksServer.URL, 24*time.Hour)

	// First fetch.
	k1, err := fetcher.GetKey("cached-kid")
	testutil.NoError(t, err)
	testutil.NotNil(t, k1)
	testutil.Equal(t, 1, fetchCount)

	// Second fetch — should use cache.
	k2, err := fetcher.GetKey("cached-kid")
	testutil.NoError(t, err)
	testutil.NotNil(t, k2)
	testutil.Equal(t, 1, fetchCount)
}

func TestAppleJWKSFetcher_RefreshesOnUnknownKid(t *testing.T) {
	t.Parallel()
	key1, _ := generateTestES256Key(t)
	key2, _ := generateTestES256Key(t)

	fetchCount := 0
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		// First fetch returns only kid-1, second adds kid-2 (simulating key rotation).
		keys := []interface{}{buildTestJWK(t, &key1.PublicKey, "kid-1")}
		if fetchCount >= 2 {
			keys = append(keys, buildTestJWK(t, &key2.PublicKey, "kid-2"))
		}
		jwks := map[string]interface{}{"keys": keys}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer jwksServer.Close()

	fetcher := NewAppleJWKSFetcher(jwksServer.URL, 24*time.Hour)

	// Fetch kid-1 — populates cache with only kid-1.
	k1, err := fetcher.GetKey("kid-1")
	testutil.NoError(t, err)
	testutil.NotNil(t, k1)
	testutil.Equal(t, 1, fetchCount)

	// Fetch kid-2 — not in cache, triggers refresh (key rotation).
	k2, err := fetcher.GetKey("kid-2")
	testutil.NoError(t, err)
	testutil.NotNil(t, k2)
	testutil.Equal(t, 2, fetchCount)
}

// --- Apple first-auth user info parsing ---

func TestParseAppleFirstAuthUserInfo(t *testing.T) {
	t.Parallel()
	userJSON := `{"name":{"firstName":"Jane","lastName":"Doe"},"email":"jane@icloud.com"}`
	info, err := ParseAppleUserPayload(userJSON)
	testutil.NoError(t, err)
	testutil.Equal(t, "jane@icloud.com", info.Email)
	testutil.Equal(t, "Jane Doe", info.Name)
}

func TestParseAppleFirstAuthUserInfo_NameOnly(t *testing.T) {
	t.Parallel()
	userJSON := `{"name":{"firstName":"Jane"}}`
	info, err := ParseAppleUserPayload(userJSON)
	testutil.NoError(t, err)
	testutil.Equal(t, "Jane", info.Name)
	testutil.Equal(t, "", info.Email)
}

func TestParseAppleFirstAuthUserInfo_EmptyJSON(t *testing.T) {
	t.Parallel()
	info, err := ParseAppleUserPayload("{}")
	testutil.NoError(t, err)
	testutil.Equal(t, "", info.Name)
	testutil.Equal(t, "", info.Email)
}

func TestParseAppleFirstAuthUserInfo_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := ParseAppleUserPayload("not-json")
	testutil.NotNil(t, err)
}

// --- Test helpers for JWKS ---

// buildTestJWKS builds a minimal JWKS response with one key.
func buildTestJWKS(t *testing.T, pub *ecdsa.PublicKey, kid string) map[string]interface{} {
	t.Helper()
	return map[string]interface{}{
		"keys": []interface{}{buildTestJWK(t, pub, kid)},
	}
}

// buildTestJWK builds a single JWK entry for an ECDSA P-256 key.
func buildTestJWK(t *testing.T, pub *ecdsa.PublicKey, kid string) map[string]interface{} {
	t.Helper()
	return map[string]interface{}{
		"kty": "EC",
		"kid": kid,
		"crv": "P-256",
		"alg": "ES256",
		"use": "sig",
		"x":   base64URLEncodeBigInt(pub.X),
		"y":   base64URLEncodeBigInt(pub.Y),
	}
}

// base64URLEncodeBigInt encodes a big.Int as base64url without padding,
// left-padded to 32 bytes for P-256 coordinate encoding.
func base64URLEncodeBigInt(n *big.Int) string {
	b := n.Bytes()
	if len(b) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(b):], b)
		b = padded
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
