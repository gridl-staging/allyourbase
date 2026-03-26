package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

// --- Discovery Document Validation ---

func TestValidateDiscoveryDocumentValid(t *testing.T) {
	t.Parallel()
	doc := &OIDCDiscoveryDocument{
		Issuer:                "https://idp.example.com",
		AuthorizationEndpoint: "https://idp.example.com/authorize",
		TokenEndpoint:         "https://idp.example.com/token",
		UserInfoEndpoint:      "https://idp.example.com/userinfo",
		JWKSURI:               "https://idp.example.com/.well-known/jwks.json",
	}
	testutil.Nil(t, ValidateDiscoveryDocument(doc))
}

func TestValidateDiscoveryDocumentMissingIssuer(t *testing.T) {
	t.Parallel()
	doc := &OIDCDiscoveryDocument{
		AuthorizationEndpoint: "https://idp.example.com/authorize",
		TokenEndpoint:         "https://idp.example.com/token",
	}
	testutil.ErrorContains(t, ValidateDiscoveryDocument(doc), "issuer")
}

func TestValidateDiscoveryDocumentMissingAuthEndpoint(t *testing.T) {
	t.Parallel()
	doc := &OIDCDiscoveryDocument{
		Issuer:        "https://idp.example.com",
		TokenEndpoint: "https://idp.example.com/token",
	}
	testutil.ErrorContains(t, ValidateDiscoveryDocument(doc), "authorization_endpoint")
}

func TestValidateDiscoveryDocumentMissingTokenEndpoint(t *testing.T) {
	t.Parallel()
	doc := &OIDCDiscoveryDocument{
		Issuer:                "https://idp.example.com",
		AuthorizationEndpoint: "https://idp.example.com/authorize",
	}
	testutil.ErrorContains(t, ValidateDiscoveryDocument(doc), "token_endpoint")
}

func TestValidateDiscoveryDocumentMissingUserInfoIsOK(t *testing.T) {
	t.Parallel()
	doc := &OIDCDiscoveryDocument{
		Issuer:                "https://idp.example.com",
		AuthorizationEndpoint: "https://idp.example.com/authorize",
		TokenEndpoint:         "https://idp.example.com/token",
	}
	// Missing userinfo_endpoint should NOT be an error.
	testutil.Nil(t, ValidateDiscoveryDocument(doc))
}

func TestDiscoveryDocumentWarningsMissingUserInfo(t *testing.T) {
	t.Parallel()
	doc := &OIDCDiscoveryDocument{
		Issuer:                "https://idp.example.com",
		AuthorizationEndpoint: "https://idp.example.com/authorize",
		TokenEndpoint:         "https://idp.example.com/token",
	}
	warnings := DiscoveryDocumentWarnings(doc)
	testutil.SliceLen(t, warnings, 1)
	testutil.Contains(t, warnings[0], "userinfo_endpoint")
}

func TestDiscoveryDocumentWarningsNoWarnings(t *testing.T) {
	t.Parallel()
	doc := &OIDCDiscoveryDocument{
		Issuer:                "https://idp.example.com",
		AuthorizationEndpoint: "https://idp.example.com/authorize",
		TokenEndpoint:         "https://idp.example.com/token",
		UserInfoEndpoint:      "https://idp.example.com/userinfo",
	}
	warnings := DiscoveryDocumentWarnings(doc)
	testutil.SliceLen(t, warnings, 0)
}

// --- Discovery Fetch ---

func TestFetchOIDCDiscovery(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, "/.well-known/openid-configuration", r.URL.Path)
		json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
			Issuer:                "https://idp.example.com",
			AuthorizationEndpoint: "https://idp.example.com/authorize",
			TokenEndpoint:         "https://idp.example.com/token",
			UserInfoEndpoint:      "https://idp.example.com/userinfo",
			JWKSURI:               "https://idp.example.com/.well-known/jwks.json",
			ScopesSupported:       []string{"openid", "profile", "email"},
		})
	}))
	defer srv.Close()

	doc, err := FetchOIDCDiscovery(srv.URL, srv.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "https://idp.example.com", doc.Issuer)
	testutil.Equal(t, "https://idp.example.com/authorize", doc.AuthorizationEndpoint)
	testutil.Equal(t, "https://idp.example.com/token", doc.TokenEndpoint)
	testutil.Equal(t, "https://idp.example.com/userinfo", doc.UserInfoEndpoint)
	testutil.Equal(t, "https://idp.example.com/.well-known/jwks.json", doc.JWKSURI)
	testutil.SliceLen(t, doc.ScopesSupported, 3)
}

func TestFetchOIDCDiscoveryNon200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchOIDCDiscovery(srv.URL, srv.Client())
	testutil.ErrorContains(t, err, "404")
}

func TestFetchOIDCDiscoveryInvalidJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	_, err := FetchOIDCDiscovery(srv.URL, srv.Client())
	testutil.ErrorContains(t, err, "parsing OIDC discovery")
}

func TestFetchOIDCDiscoveryMissingRequiredField(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Missing issuer
		json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": "https://idp.example.com/authorize",
			"token_endpoint":         "https://idp.example.com/token",
		})
	}))
	defer srv.Close()

	_, err := FetchOIDCDiscovery(srv.URL, srv.Client())
	testutil.ErrorContains(t, err, "issuer")
}

// --- Discovery Caching ---

func TestOIDCDiscoveryCacheHit(t *testing.T) {
	t.Parallel()
	var fetchCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
			Issuer:                "https://idp.example.com",
			AuthorizationEndpoint: "https://idp.example.com/authorize",
			TokenEndpoint:         "https://idp.example.com/token",
		})
	}))
	defer srv.Close()

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	// First fetch hits the server.
	doc1, err := cache.Get(srv.URL)
	testutil.NoError(t, err)
	testutil.Equal(t, "https://idp.example.com", doc1.Issuer)
	testutil.Equal(t, int32(1), fetchCount.Load())

	// Second fetch uses cache — no additional server hit.
	doc2, err := cache.Get(srv.URL)
	testutil.NoError(t, err)
	testutil.Equal(t, "https://idp.example.com", doc2.Issuer)
	testutil.Equal(t, int32(1), fetchCount.Load())
}

func TestOIDCDiscoveryCacheExpiry(t *testing.T) {
	t.Parallel()
	var fetchCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
			Issuer:                "https://idp.example.com",
			AuthorizationEndpoint: "https://idp.example.com/authorize",
			TokenEndpoint:         "https://idp.example.com/token",
		})
	}))
	defer srv.Close()

	cache := NewOIDCDiscoveryCache(1 * time.Millisecond)
	cache.SetHTTPClient(srv.Client())

	_, err := cache.Get(srv.URL)
	testutil.NoError(t, err)
	testutil.Equal(t, int32(1), fetchCount.Load())

	time.Sleep(5 * time.Millisecond)

	_, err = cache.Get(srv.URL)
	testutil.NoError(t, err)
	testutil.Equal(t, int32(2), fetchCount.Load())
}

func TestOIDCDiscoveryCacheDifferentIssuers(t *testing.T) {
	t.Parallel()
	var fetchCount atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/issuer1/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
			Issuer:                "https://issuer1.example.com",
			AuthorizationEndpoint: "https://issuer1.example.com/authorize",
			TokenEndpoint:         "https://issuer1.example.com/token",
		})
	})
	mux.HandleFunc("/issuer2/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
			Issuer:                "https://issuer2.example.com",
			AuthorizationEndpoint: "https://issuer2.example.com/authorize",
			TokenEndpoint:         "https://issuer2.example.com/token",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	doc1, err := cache.Get(srv.URL + "/issuer1")
	testutil.NoError(t, err)
	testutil.Equal(t, "https://issuer1.example.com", doc1.Issuer)

	doc2, err := cache.Get(srv.URL + "/issuer2")
	testutil.NoError(t, err)
	testutil.Equal(t, "https://issuer2.example.com", doc2.Issuer)

	// Both fetches hit the server.
	testutil.Equal(t, int32(2), fetchCount.Load())
}

func TestOAuthOIDCGoUnder500Lines(t *testing.T) {
	t.Parallel()
	assertFileUnder500Lines(t, "oauth_oidc.go")
}

func TestOIDCDiscoveryCacheInvalidate(t *testing.T) {
	t.Parallel()
	var fetchCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
			Issuer:                "https://idp.example.com",
			AuthorizationEndpoint: "https://idp.example.com/authorize",
			TokenEndpoint:         "https://idp.example.com/token",
		})
	}))
	defer srv.Close()

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	_, _ = cache.Get(srv.URL)
	testutil.Equal(t, int32(1), fetchCount.Load())

	cache.Invalidate(srv.URL)

	_, _ = cache.Get(srv.URL)
	testutil.Equal(t, int32(2), fetchCount.Load())
}

// --- JWKS Fetcher ---

func TestJWKSFetcherRSA(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	jwksJSON := buildRSAJWKS(t, "rsa-kid-1", &rsaKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	key, err := fetcher.GetKey("rsa-kid-1")
	testutil.NoError(t, err)
	testutil.NotNil(t, key)

	rsaPub, ok := key.(*rsa.PublicKey)
	testutil.True(t, ok, "expected *rsa.PublicKey")
	testutil.True(t, rsaPub.N.Cmp(rsaKey.PublicKey.N) == 0, "RSA N mismatch")
}

func TestJWKSFetcherEC(t *testing.T) {
	t.Parallel()
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testutil.NoError(t, err)

	jwksJSON := buildECJWKS(t, "ec-kid-1", &ecKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	key, err := fetcher.GetKey("ec-kid-1")
	testutil.NoError(t, err)
	testutil.NotNil(t, key)

	ecPub, ok := key.(*ecdsa.PublicKey)
	testutil.True(t, ok, "expected *ecdsa.PublicKey")
	testutil.True(t, ecPub.X.Cmp(ecKey.PublicKey.X) == 0, "EC X mismatch")
}

func TestJWKSFetcherCacheTTL(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	var fetchCount atomic.Int32
	jwksJSON := buildRSAJWKS(t, "rsa-kid-1", &rsaKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	_, err = fetcher.GetKey("rsa-kid-1")
	testutil.NoError(t, err)
	testutil.Equal(t, int32(1), fetchCount.Load())

	// Second call uses cache.
	_, err = fetcher.GetKey("rsa-kid-1")
	testutil.NoError(t, err)
	testutil.Equal(t, int32(1), fetchCount.Load())
}

func TestJWKSFetcherRefreshOnMiss(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	var fetchCount atomic.Int32
	jwksJSON := buildRSAJWKS(t, "rsa-kid-1", &rsaKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetchCount.Add(1)
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	// Fetch existing key.
	_, err = fetcher.GetKey("rsa-kid-1")
	testutil.NoError(t, err)
	testutil.Equal(t, int32(1), fetchCount.Load())

	// Request unknown key — triggers refresh (key still won't be found).
	_, err = fetcher.GetKey("nonexistent-kid")
	testutil.ErrorContains(t, err, "not found")
	testutil.Equal(t, int32(2), fetchCount.Load())
}

func TestJWKSFetcherNon200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	_, err := fetcher.GetKey("any-kid")
	testutil.ErrorContains(t, err, "500")
}

// --- OIDC id_token Verification ---

func TestVerifyOIDCIDTokenRSA256(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	issuer := "https://idp.example.com"
	clientID := "my-client"
	kid := "rsa-test-kid"

	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss":   issuer,
		"aud":   clientID,
		"sub":   "user-123",
		"email": "user@example.com",
		"name":  "Test User",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	jwksJSON := buildRSAJWKS(t, kid, &rsaKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	info, err := VerifyOIDCIDToken(idToken, issuer, clientID, "", fetcher)
	testutil.NoError(t, err)
	testutil.Equal(t, "user-123", info.ProviderUserID)
	testutil.Equal(t, "user@example.com", info.Email)
	testutil.Equal(t, "Test User", info.Name)
}

func TestVerifyOIDCIDTokenES256(t *testing.T) {
	t.Parallel()
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testutil.NoError(t, err)

	issuer := "https://idp.example.com"
	clientID := "my-client"
	kid := "ec-test-kid"

	idToken := signJWT(t, jwt.SigningMethodES256, ecKey, kid, jwt.MapClaims{
		"iss":   issuer,
		"aud":   clientID,
		"sub":   "user-456",
		"email": "ec@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	jwksJSON := buildECJWKS(t, kid, &ecKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	info, err := VerifyOIDCIDToken(idToken, issuer, clientID, "", fetcher)
	testutil.NoError(t, err)
	testutil.Equal(t, "user-456", info.ProviderUserID)
	testutil.Equal(t, "ec@example.com", info.Email)
}

func TestVerifyOIDCIDTokenPreferredUsername(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	issuer := "https://idp.example.com"
	clientID := "my-client"
	kid := "rsa-kid"

	// No "name" claim — should fall back to "preferred_username".
	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss":                issuer,
		"aud":                clientID,
		"sub":                "user-789",
		"preferred_username": "jdoe",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
	})

	jwksJSON := buildRSAJWKS(t, kid, &rsaKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	info, err := VerifyOIDCIDToken(idToken, issuer, clientID, "", fetcher)
	testutil.NoError(t, err)
	testutil.Equal(t, "jdoe", info.Name)
}

func TestVerifyOIDCIDTokenNonceMatch(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	issuer := "https://idp.example.com"
	clientID := "my-client"
	kid := "rsa-nonce-kid"

	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss":   issuer,
		"aud":   clientID,
		"sub":   "user-123",
		"nonce": "nonce-123",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	fetcher := NewJWKSFetcher("https://jwks.invalid", time.Hour)
	fetcher.keys[kid] = &rsaKey.PublicKey
	fetcher.fetchedAt = time.Now()

	_, err = VerifyOIDCIDToken(idToken, issuer, clientID, "nonce-123", fetcher)
	testutil.NoError(t, err)
}

func TestVerifyOIDCIDTokenNonceMismatch(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	issuer := "https://idp.example.com"
	clientID := "my-client"
	kid := "rsa-nonce-mismatch-kid"

	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss":   issuer,
		"aud":   clientID,
		"sub":   "user-123",
		"nonce": "nonce-other",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	fetcher := NewJWKSFetcher("https://jwks.invalid", time.Hour)
	fetcher.keys[kid] = &rsaKey.PublicKey
	fetcher.fetchedAt = time.Now()

	_, err = VerifyOIDCIDToken(idToken, issuer, clientID, "nonce-123", fetcher)
	testutil.ErrorContains(t, err, "nonce")
}

func TestVerifyOIDCIDTokenNonceMissingClaim(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	issuer := "https://idp.example.com"
	clientID := "my-client"
	kid := "rsa-nonce-missing-kid"

	// Token has no nonce claim at all.
	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss": issuer,
		"aud": clientID,
		"sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})

	fetcher := NewJWKSFetcher("https://jwks.invalid", time.Hour)
	fetcher.keys[kid] = &rsaKey.PublicKey
	fetcher.fetchedAt = time.Now()

	_, err = VerifyOIDCIDToken(idToken, issuer, clientID, "expected-nonce", fetcher)
	testutil.ErrorContains(t, err, "missing nonce claim")
}

func TestVerifyOIDCIDTokenExpired(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	kid := "rsa-kid"
	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss": "https://idp.example.com",
		"aud": "my-client",
		"sub": "user-123",
		"exp": time.Now().Add(-time.Hour).Unix(),
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	})

	jwksJSON := buildRSAJWKS(t, kid, &rsaKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	_, err = VerifyOIDCIDToken(idToken, "https://idp.example.com", "my-client", "", fetcher)
	testutil.ErrorContains(t, err, "verification failed")
}

func TestVerifyOIDCIDTokenWrongIssuer(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	kid := "rsa-kid"
	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss": "https://wrong-issuer.example.com",
		"aud": "my-client",
		"sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})

	jwksJSON := buildRSAJWKS(t, kid, &rsaKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	_, err = VerifyOIDCIDToken(idToken, "https://idp.example.com", "my-client", "", fetcher)
	testutil.ErrorContains(t, err, "verification failed")
}

func TestVerifyOIDCIDTokenWrongAudience(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	kid := "rsa-kid"
	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss": "https://idp.example.com",
		"aud": "wrong-client",
		"sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})

	jwksJSON := buildRSAJWKS(t, kid, &rsaKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	_, err = VerifyOIDCIDToken(idToken, "https://idp.example.com", "my-client", "", fetcher)
	testutil.ErrorContains(t, err, "verification failed")
}

func TestVerifyOIDCIDTokenMissingSub(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	kid := "rsa-kid"
	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss":   "https://idp.example.com",
		"aud":   "my-client",
		"email": "user@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	jwksJSON := buildRSAJWKS(t, kid, &rsaKey.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	}))
	defer srv.Close()

	fetcher := NewJWKSFetcher(srv.URL, time.Hour)
	fetcher.SetHTTPClient(srv.Client())

	_, err = VerifyOIDCIDToken(idToken, "https://idp.example.com", "my-client", "", fetcher)
	testutil.ErrorContains(t, err, "missing sub")
}

// --- OIDC UserInfo Parser ---

func TestParseOIDCUserInfoFull(t *testing.T) {
	t.Parallel()
	body := []byte(`{"sub":"uid-001","email":"u@example.com","name":"Jane Doe","preferred_username":"jdoe"}`)
	info, err := parseOIDCUserInfo(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "uid-001", info.ProviderUserID)
	testutil.Equal(t, "u@example.com", info.Email)
	testutil.Equal(t, "Jane Doe", info.Name)
}

func TestParseOIDCUserInfoFallbackToPreferredUsername(t *testing.T) {
	t.Parallel()
	body := []byte(`{"sub":"uid-002","email":"u@example.com","preferred_username":"jdoe"}`)
	info, err := parseOIDCUserInfo(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "jdoe", info.Name)
}

func TestParseOIDCUserInfoMissingSub(t *testing.T) {
	t.Parallel()
	body := []byte(`{"email":"u@example.com","name":"No Sub"}`)
	_, err := parseOIDCUserInfo(body)
	testutil.ErrorContains(t, err, "missing user ID")
}

func TestParseOIDCUserInfoMinimal(t *testing.T) {
	t.Parallel()
	body := []byte(`{"sub":"uid-003"}`)
	info, err := parseOIDCUserInfo(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "uid-003", info.ProviderUserID)
	testutil.Equal(t, "", info.Email)
	testutil.Equal(t, "", info.Name)
}

// --- Provider Registration ---

func TestRegisterOIDCProvider(t *testing.T) {
	t.Parallel()

	srv := newMockOIDCServer(t)
	defer srv.Close()

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	err := RegisterOIDCProvider("keycloak-test", OIDCProviderRegistration{
		IssuerURL:    srv.URL,
		ClientID:     "kc-client",
		ClientSecret: "kc-secret",
	}, cache)
	testutil.NoError(t, err)
	defer UnregisterOIDCProvider("keycloak-test")

	// Verify provider is in the registry.
	pc, ok := getProviderConfig("keycloak-test")
	testutil.True(t, ok, "keycloak-test should be registered")
	testutil.Contains(t, pc.AuthURL, "/authorize")
	testutil.Contains(t, pc.TokenURL, "/token")
	testutil.Equal(t, OAuthUserInfoSourceIDTokenWithEndpointFallback, pc.UserInfoSource)
	testutil.NotNil(t, pc.IDTokenUserInfoParser)

	// Verify default scopes applied.
	testutil.SliceLen(t, pc.Scopes, 3)
}

func TestRegisterOIDCProviderCustomScopes(t *testing.T) {
	t.Parallel()

	srv := newMockOIDCServer(t)
	defer srv.Close()

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	err := RegisterOIDCProvider("auth0-test", OIDCProviderRegistration{
		IssuerURL:    srv.URL,
		ClientID:     "a0-client",
		ClientSecret: "a0-secret",
		Scopes:       []string{"openid", "email"},
	}, cache)
	testutil.NoError(t, err)
	defer UnregisterOIDCProvider("auth0-test")

	pc, ok := getProviderConfig("auth0-test")
	testutil.True(t, ok, "auth0-test should be registered")
	testutil.SliceLen(t, pc.Scopes, 2)
}

func TestRegisterOIDCProviderNoUserInfoEndpointUsesIDTokenSource(t *testing.T) {
	t.Parallel()

	const issuerURL = "https://issuer-no-userinfo.example.com"
	const discoveryURL = issuerURL + "/.well-known/openid-configuration"

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != discoveryURL {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{}`)),
				}, nil
			}
			b, _ := json.Marshal(OIDCDiscoveryDocument{
				Issuer:                "https://idp.example.com",
				AuthorizationEndpoint: "https://idp.example.com/authorize",
				TokenEndpoint:         "https://idp.example.com/token",
				JWKSURI:               "https://idp.example.com/jwks",
			})
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(string(b))),
			}, nil
		}),
	})

	err := RegisterOIDCProvider("oidc-idtoken-only", OIDCProviderRegistration{
		IssuerURL:    issuerURL,
		ClientID:     "oidc-client",
		ClientSecret: "oidc-secret",
	}, cache)
	testutil.NoError(t, err)
	defer UnregisterOIDCProvider("oidc-idtoken-only")

	pc, ok := getProviderConfig("oidc-idtoken-only")
	testutil.True(t, ok, "oidc-idtoken-only should be registered")
	testutil.Equal(t, OAuthUserInfoSourceIDToken, pc.UserInfoSource)
}

func TestRegisterOIDCProviderConflictsWithBuiltIn(t *testing.T) {
	t.Parallel()

	cache := NewOIDCDiscoveryCache(time.Hour)
	err := RegisterOIDCProvider("google", OIDCProviderRegistration{
		IssuerURL: "https://accounts.google.com",
		ClientID:  "test",
	}, cache)
	testutil.ErrorContains(t, err, "conflicts with built-in")
}

func TestRegisterOIDCProviderRejectsEmptyName(t *testing.T) {
	t.Parallel()

	const issuerURL = "https://issuer-empty-name.example.com"
	const discoveryURL = issuerURL + "/.well-known/openid-configuration"

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != discoveryURL {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{}`)),
				}, nil
			}
			b, _ := json.Marshal(OIDCDiscoveryDocument{
				Issuer:                issuerURL,
				AuthorizationEndpoint: issuerURL + "/authorize",
				TokenEndpoint:         issuerURL + "/token",
				UserInfoEndpoint:      issuerURL + "/userinfo",
				JWKSURI:               issuerURL + "/jwks",
			})
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string(b))),
			}, nil
		}),
	})

	err := RegisterOIDCProvider("   ", OIDCProviderRegistration{
		IssuerURL:    issuerURL,
		ClientID:     "client",
		ClientSecret: "secret",
	}, cache)
	testutil.ErrorContains(t, err, "provider name is required")
}

func TestRegisterMultipleOIDCProviders(t *testing.T) {
	t.Parallel()

	srv := newMockOIDCServer(t)
	defer srv.Close()

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	err := RegisterOIDCProvider("oidc-provider-a", OIDCProviderRegistration{
		IssuerURL:    srv.URL,
		ClientID:     "client-a",
		ClientSecret: "secret-a",
	}, cache)
	testutil.NoError(t, err)
	defer UnregisterOIDCProvider("oidc-provider-a")

	err = RegisterOIDCProvider("oidc-provider-b", OIDCProviderRegistration{
		IssuerURL:    srv.URL,
		ClientID:     "client-b",
		ClientSecret: "secret-b",
	}, cache)
	testutil.NoError(t, err)
	defer UnregisterOIDCProvider("oidc-provider-b")

	_, okA := getProviderConfig("oidc-provider-a")
	_, okB := getProviderConfig("oidc-provider-b")
	testutil.True(t, okA, "oidc-provider-a should be registered")
	testutil.True(t, okB, "oidc-provider-b should be registered")
}

func TestRegisterOIDCProviderRefreshesDiscoveryAndJWKSOnVerificationFailure(t *testing.T) {
	t.Parallel()

	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)
	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	const issuer = "https://idp.example.com"
	const clientID = "oidc-client"

	oldJWKS := buildRSAJWKS(t, "old-kid", &oldKey.PublicKey)
	newJWKS := buildRSAJWKS(t, "new-kid", &newKey.PublicKey)

	idToken := signJWT(t, jwt.SigningMethodRS256, newKey, "new-kid", jwt.MapClaims{
		"iss":   issuer,
		"aud":   clientID,
		"sub":   "rotated-user",
		"email": "rotated@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	var discoveryFetches atomic.Int32
	var oldJWKSFetches atomic.Int32
	var newJWKSFetches atomic.Int32
	var useNewDiscovery atomic.Bool

	const issuerURL = "https://issuer-refresh.example.com"
	discoveryURL := issuerURL + "/.well-known/openid-configuration"
	oldJWKSURL := issuerURL + "/jwks-old"
	newJWKSURL := issuerURL + "/jwks-new"

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.String() {
			case discoveryURL:
				discoveryFetches.Add(1)
				jwksURI := oldJWKSURL
				if useNewDiscovery.Load() {
					jwksURI = newJWKSURL
				}
				b, _ := json.Marshal(OIDCDiscoveryDocument{
					Issuer:                issuer,
					AuthorizationEndpoint: issuerURL + "/authorize",
					TokenEndpoint:         issuerURL + "/token",
					UserInfoEndpoint:      issuerURL + "/userinfo",
					JWKSURI:               jwksURI,
				})
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(string(b))),
				}, nil
			case oldJWKSURL:
				oldJWKSFetches.Add(1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(string(oldJWKS))),
				}, nil
			case newJWKSURL:
				newJWKSFetches.Add(1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(string(newJWKS))),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{}`)),
				}, nil
			}
		}),
	}

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(httpClient)

	err = RegisterOIDCProvider("oidc-refresh-test", OIDCProviderRegistration{
		IssuerURL:    issuerURL,
		ClientID:     clientID,
		ClientSecret: "secret",
	}, cache)
	testutil.NoError(t, err)
	defer UnregisterOIDCProvider("oidc-refresh-test")

	useNewDiscovery.Store(true)

	pc, ok := getProviderConfig("oidc-refresh-test")
	testutil.True(t, ok, "oidc-refresh-test should be registered")
	testutil.NotNil(t, pc.IDTokenUserInfoParser)

	info, err := pc.IDTokenUserInfoParser(context.Background(), idToken)
	testutil.NoError(t, err)
	testutil.Equal(t, "rotated-user", info.ProviderUserID)
	testutil.Equal(t, "rotated@example.com", info.Email)

	testutil.True(t, discoveryFetches.Load() >= 2, "discovery should be fetched again after verification failure")
	testutil.True(t, oldJWKSFetches.Load() >= 1, "old JWKS should be queried during initial verification")
	testutil.True(t, newJWKSFetches.Load() >= 1, "new JWKS should be queried after discovery refresh")
}

func TestUnregisterOIDCProvider(t *testing.T) {
	t.Parallel()

	srv := newMockOIDCServer(t)
	defer srv.Close()

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	err := RegisterOIDCProvider("oidc-temp", OIDCProviderRegistration{
		IssuerURL:    srv.URL,
		ClientID:     "tmp-client",
		ClientSecret: "tmp-secret",
	}, cache)
	testutil.NoError(t, err)

	_, ok := getProviderConfig("oidc-temp")
	testutil.True(t, ok, "should be registered")

	UnregisterOIDCProvider("oidc-temp")

	_, ok = getProviderConfig("oidc-temp")
	testutil.False(t, ok, "should be unregistered")
}

func TestUnregisterOIDCProviderDoesNotRemoveBuiltIn(t *testing.T) {
	t.Parallel()

	t.Cleanup(func() {
		ResetProviderURLs("google")
		ResetOAuthUserInfoParser("google")
	})

	_, beforeExists := getProviderConfig("google")
	testutil.True(t, beforeExists, "google provider should exist before unregister attempt")

	UnregisterOIDCProvider("google")

	_, afterExists := getProviderConfig("google")
	testutil.True(t, afterExists, "google provider should not be removed by OIDC unregister")
}

// --- Authorization URL for OIDC provider ---

func TestOIDCAuthorizationURL(t *testing.T) {
	t.Parallel()

	srv := newMockOIDCServer(t)
	defer srv.Close()

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	err := RegisterOIDCProvider("oidc-auth-url-test", OIDCProviderRegistration{
		IssuerURL:    srv.URL,
		ClientID:     "url-client",
		ClientSecret: "url-secret",
	}, cache)
	testutil.NoError(t, err)
	defer UnregisterOIDCProvider("oidc-auth-url-test")

	u, err := AuthorizationURL("oidc-auth-url-test", OAuthClientConfig{
		ClientID:     "url-client",
		ClientSecret: "url-secret",
	}, "http://localhost/callback", "test-state")
	testutil.NoError(t, err)
	testutil.Contains(t, u, "/authorize")
	testutil.Contains(t, u, "client_id=url-client")
	testutil.Contains(t, u, "state=test-state")
}

func TestOIDCAuthorizationURLIncludesNonce(t *testing.T) {
	t.Parallel()

	pc := OAuthProviderConfig{
		AuthURL:        "https://idp.example.com/authorize",
		Scopes:         []string{"openid", "profile", "email"},
		DiscoveryURL:   "https://idp.example.com",
		UserInfoSource: OAuthUserInfoSourceIDTokenWithEndpointFallback,
	}
	authURL, err := authorizationURLWithConfig("oidc-auth-nonce-test", OAuthClientConfig{
		ClientID:     "nonce-client",
		ClientSecret: "nonce-secret",
	}, "http://localhost/callback", "nonce-state-123", pc)
	testutil.NoError(t, err)

	parsed, err := url.Parse(authURL)
	testutil.NoError(t, err)
	testutil.Equal(t, "nonce-state-123", parsed.Query().Get("nonce"))
}

// --- Integration: Full OIDC ExchangeCode Flow ---

func TestOIDCExchangeCodeIDTokenPath(t *testing.T) {
	t.Parallel()

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	kid := "oidc-rsa-kid"
	issuer := "https://idp.example.com"
	clientID := "my-oidc-client"

	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss":   issuer,
		"aud":   clientID,
		"sub":   "oidc-user-001",
		"email": "oidc@example.com",
		"name":  "OIDC User",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})

	jwksJSON := buildRSAJWKS(t, kid, &rsaKey.PublicKey)

	// Mock IdP server: discovery + JWKS + token endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		// Will fill in URLs after server starts.
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	})
	// Placeholder — will be replaced.
	srv := httptest.NewServer(nil)

	// Now build the real handler with the server URL.
	realMux := http.NewServeMux()
	realMux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
			Issuer:                issuer,
			AuthorizationEndpoint: srv.URL + "/authorize",
			TokenEndpoint:         srv.URL + "/token",
			UserInfoEndpoint:      srv.URL + "/userinfo",
			JWKSURI:               srv.URL + "/jwks",
		})
	})
	realMux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	})
	realMux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		testutil.Equal(t, "authorization_code", r.Form.Get("grant_type"))
		testutil.Equal(t, "test-code", r.Form.Get("code"))
		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "test-access-token",
			"id_token":     idToken,
			"token_type":   "Bearer",
		})
	})
	realMux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		// Should NOT be called when id_token has email.
		t.Error("userinfo endpoint should not be called when id_token has email")
	})
	srv.Config.Handler = realMux

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	err = RegisterOIDCProvider("oidc-flow-test", OIDCProviderRegistration{
		IssuerURL:    srv.URL,
		ClientID:     clientID,
		ClientSecret: "test-secret",
	}, cache)
	testutil.NoError(t, err)
	defer UnregisterOIDCProvider("oidc-flow-test")

	// Override the JWKS fetcher's HTTP client for the test.
	pc, _ := getProviderConfig("oidc-flow-test")

	info, err := exchangeCode(context.Background(), "oidc-flow-test",
		OAuthClientConfig{ClientID: clientID, ClientSecret: "test-secret"},
		"test-code", "http://localhost/callback", pc, srv.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "oidc-user-001", info.ProviderUserID)
	testutil.Equal(t, "oidc@example.com", info.Email)
	testutil.Equal(t, "OIDC User", info.Name)
}

func TestOIDCExchangeCodeFallbackToEndpoint(t *testing.T) {
	t.Parallel()

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	kid := "oidc-rsa-kid"
	issuer := "https://idp.example.com"
	clientID := "my-oidc-client"

	// id_token WITHOUT email — should trigger endpoint fallback.
	idToken := signJWT(t, jwt.SigningMethodRS256, rsaKey, kid, jwt.MapClaims{
		"iss": issuer,
		"aud": clientID,
		"sub": "oidc-user-002",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})

	jwksJSON := buildRSAJWKS(t, kid, &rsaKey.PublicKey)
	srv := httptest.NewServer(nil)

	realMux := http.NewServeMux()
	realMux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
			Issuer:                issuer,
			AuthorizationEndpoint: srv.URL + "/authorize",
			TokenEndpoint:         srv.URL + "/token",
			UserInfoEndpoint:      srv.URL + "/userinfo",
			JWKSURI:               srv.URL + "/jwks",
		})
	})
	realMux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(jwksJSON)
	})
	realMux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "test-access-token",
			"id_token":     idToken,
			"token_type":   "Bearer",
		})
	})
	realMux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		// Verify Bearer token is sent.
		testutil.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
		json.NewEncoder(w).Encode(map[string]string{
			"sub":   "oidc-user-002",
			"email": "fallback@example.com",
			"name":  "Fallback User",
		})
	})
	srv.Config.Handler = realMux

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	err = RegisterOIDCProvider("oidc-fallback-test", OIDCProviderRegistration{
		IssuerURL:    srv.URL,
		ClientID:     clientID,
		ClientSecret: "test-secret",
	}, cache)
	testutil.NoError(t, err)
	defer UnregisterOIDCProvider("oidc-fallback-test")

	pc, _ := getProviderConfig("oidc-fallback-test")

	info, err := exchangeCode(context.Background(), "oidc-fallback-test",
		OAuthClientConfig{ClientID: clientID, ClientSecret: "test-secret"},
		"test-code", "http://localhost/callback", pc, srv.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "oidc-user-002", info.ProviderUserID)
	testutil.Equal(t, "fallback@example.com", info.Email)
}

func TestOIDCExchangeCodeNoIDTokenUsesEndpoint(t *testing.T) {
	t.Parallel()

	issuer := "https://idp.example.com"
	clientID := "my-oidc-client"

	srv := httptest.NewServer(nil)

	realMux := http.NewServeMux()
	realMux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
			Issuer:                issuer,
			AuthorizationEndpoint: srv.URL + "/authorize",
			TokenEndpoint:         srv.URL + "/token",
			UserInfoEndpoint:      srv.URL + "/userinfo",
			JWKSURI:               srv.URL + "/jwks",
		})
	})
	realMux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"keys":[]}`))
	})
	realMux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		// No id_token in response — only access_token.
		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
		})
	})
	realMux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"sub":   "endpoint-user",
			"email": "endpoint@example.com",
			"name":  "Endpoint User",
		})
	})
	srv.Config.Handler = realMux

	cache := NewOIDCDiscoveryCache(time.Hour)
	cache.SetHTTPClient(srv.Client())

	err := RegisterOIDCProvider("oidc-notoken-test", OIDCProviderRegistration{
		IssuerURL:    srv.URL,
		ClientID:     clientID,
		ClientSecret: "test-secret",
	}, cache)
	testutil.NoError(t, err)
	defer UnregisterOIDCProvider("oidc-notoken-test")

	pc, _ := getProviderConfig("oidc-notoken-test")

	info, err := exchangeCode(context.Background(), "oidc-notoken-test",
		OAuthClientConfig{ClientID: clientID, ClientSecret: "test-secret"},
		"test-code", "http://localhost/callback", pc, srv.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "endpoint-user", info.ProviderUserID)
	testutil.Equal(t, "endpoint@example.com", info.Email)
	testutil.Equal(t, "Endpoint User", info.Name)
}

// --- Test Helpers ---

func newMockOIDCServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
				Issuer:                "https://idp.example.com",
				AuthorizationEndpoint: "https://idp.example.com/authorize",
				TokenEndpoint:         "https://idp.example.com/token",
				UserInfoEndpoint:      "https://idp.example.com/userinfo",
				JWKSURI:               "https://idp.example.com/.well-known/jwks.json",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	return srv
}

func signJWT(t *testing.T, method jwt.SigningMethod, key interface{}, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("signing JWT: %v", err)
	}
	return signed
}

func buildRSAJWKS(t *testing.T, kid string, pub *rsa.PublicKey) []byte {
	t.Helper()
	nBase64 := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	eBase64 := base64.RawURLEncoding.EncodeToString(eBytes)

	jwks := map[string]interface{}{
		"keys": []map[string]string{
			{
				"kty": "RSA",
				"kid": kid,
				"use": "sig",
				"alg": "RS256",
				"n":   nBase64,
				"e":   eBase64,
			},
		},
	}
	b, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("marshaling JWKS: %v", err)
	}
	return b
}

func buildECJWKS(t *testing.T, kid string, pub *ecdsa.PublicKey) []byte {
	t.Helper()
	byteLen := (pub.Curve.Params().BitSize + 7) / 8
	xBytes := pub.X.Bytes()
	yBytes := pub.Y.Bytes()
	// Pad to full byte length.
	for len(xBytes) < byteLen {
		xBytes = append([]byte{0}, xBytes...)
	}
	for len(yBytes) < byteLen {
		yBytes = append([]byte{0}, yBytes...)
	}

	jwks := map[string]interface{}{
		"keys": []map[string]string{
			{
				"kty": "EC",
				"kid": kid,
				"crv": "P-256",
				"x":   base64.RawURLEncoding.EncodeToString(xBytes),
				"y":   base64.RawURLEncoding.EncodeToString(yBytes),
			},
		},
	}
	b, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("marshaling JWKS: %v", err)
	}
	return b
}

// Unused import guard — these are used in test helper functions.
var _ = fmt.Sprintf

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
