package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNewAPNSProviderEnvironmentDefaults(t *testing.T) {
	keyFile := writeAPNSKeyFile(t)

	prod, err := NewAPNSProvider(APNSConfig{
		KeyFile:     keyFile,
		TeamID:      "TEAM123",
		KeyID:       "KEY123",
		BundleID:    "com.example.app",
		Environment: "production",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, apnsProductionBaseURL, prod.baseURL)

	sandbox, err := NewAPNSProvider(APNSConfig{
		KeyFile:     keyFile,
		TeamID:      "TEAM123",
		KeyID:       "KEY123",
		BundleID:    "com.example.app",
		Environment: "sandbox",
	})
	testutil.NoError(t, err)
	testutil.Equal(t, apnsSandboxBaseURL, sandbox.baseURL)

	_, err = NewAPNSProvider(APNSConfig{
		KeyFile:     keyFile,
		TeamID:      "TEAM123",
		KeyID:       "KEY123",
		BundleID:    "com.example.app",
		Environment: "invalid",
	})
	testutil.ErrorContains(t, err, "environment")
}

func TestAPNSProviderSendSuccess(t *testing.T) {
	var sendCalls int32
	baseURL := "https://apns.example"

	keyFile := writeAPNSKeyFile(t)
	p, err := NewAPNSProvider(APNSConfig{
		KeyFile:     keyFile,
		TeamID:      "TEAM123",
		KeyID:       "KEY123",
		BundleID:    "com.example.app",
		Environment: "production",
		BaseURL:     baseURL,
	})
	testutil.NoError(t, err)

	p.httpClient = http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&sendCalls, 1)
			testutil.Equal(t, http.MethodPost, r.Method)
			testutil.Equal(t, baseURL+"/3/device/device-token-1", r.URL.String())
			testutil.Equal(t, "com.example.app", r.Header.Get("apns-topic"))
			testutil.Equal(t, "alert", r.Header.Get("apns-push-type"))
			testutil.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "bearer "))

			var payload map[string]any
			err := json.NewDecoder(r.Body).Decode(&payload)
			testutil.NoError(t, err)
			aps, ok := payload["aps"].(map[string]any)
			testutil.True(t, ok)
			alert, ok := aps["alert"].(map[string]any)
			testutil.True(t, ok)
			testutil.Equal(t, "Hello", alert["title"].(string))
			testutil.Equal(t, "World", alert["body"].(string))
			testutil.Equal(t, "v", payload["k"].(string))

			return mockResponse(http.StatusOK, "", map[string]string{"apns-id": "apns-msg-1"}), nil
		}),
	}

	res, err := p.Send(t.Context(), "device-token-1", &Message{
		Title: "Hello",
		Body:  "World",
		Data:  map[string]string{"k": "v"},
	})
	testutil.NoError(t, err)
	testutil.Equal(t, int32(1), atomic.LoadInt32(&sendCalls))
	testutil.Equal(t, "apns-msg-1", res.MessageID)
}

func TestAPNSProviderJWTCachingAndRefresh(t *testing.T) {
	var sendCalls int32
	authHeaders := make([]string, 0, 3)
	baseURL := "https://apns.example"

	keyFile := writeAPNSKeyFile(t)
	p, err := NewAPNSProvider(APNSConfig{
		KeyFile:     keyFile,
		TeamID:      "TEAM123",
		KeyID:       "KEY123",
		BundleID:    "com.example.app",
		Environment: "production",
		BaseURL:     baseURL,
	})
	testutil.NoError(t, err)

	p.httpClient = http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&sendCalls, 1)
			authHeaders = append(authHeaders, r.Header.Get("Authorization"))
			return mockResponse(http.StatusOK, "", map[string]string{"apns-id": "apns-msg-ok"}), nil
		}),
	}

	now := time.Unix(1_700_000_000, 0)
	p.now = func() time.Time { return now }

	_, err = p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
	testutil.NoError(t, err)
	_, err = p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
	testutil.NoError(t, err)

	now = now.Add(51 * time.Minute)
	_, err = p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
	testutil.NoError(t, err)

	testutil.Equal(t, int32(3), atomic.LoadInt32(&sendCalls))
	testutil.Equal(t, authHeaders[0], authHeaders[1])
	testutil.True(t, authHeaders[2] != authHeaders[1])
}

func TestAPNSProviderErrorMapping(t *testing.T) {
	baseURL := "https://apns.example"

	// Generate key once for all subtests — ECDSA key generation is expensive.
	keyFile := writeAPNSKeyFile(t)

	tests := []struct {
		name    string
		reason  string
		status  int
		wantErr error
	}{
		{name: "bad device token", reason: "BadDeviceToken", status: http.StatusBadRequest, wantErr: ErrInvalidToken},
		{name: "device token not for topic", reason: "DeviceTokenNotForTopic", status: http.StatusBadRequest, wantErr: ErrInvalidToken},
		{name: "unregistered", reason: "Unregistered", status: http.StatusGone, wantErr: ErrUnregistered},
		{name: "expired token", reason: "ExpiredToken", status: http.StatusGone, wantErr: ErrUnregistered},
		{name: "too many requests", reason: "TooManyRequests", status: http.StatusTooManyRequests, wantErr: ErrProviderError},
		{name: "payload too large", reason: "PayloadTooLarge", status: http.StatusRequestEntityTooLarge, wantErr: ErrPayloadTooLarge},
		{name: "expired provider token", reason: "ExpiredProviderToken", status: http.StatusForbidden, wantErr: ErrProviderAuth},
		{name: "invalid provider token", reason: "InvalidProviderToken", status: http.StatusForbidden, wantErr: ErrProviderAuth},
		{name: "missing provider token", reason: "MissingProviderToken", status: http.StatusForbidden, wantErr: ErrProviderAuth},
		{name: "unknown reason", reason: "WhateverElse", status: http.StatusBadGateway, wantErr: ErrProviderError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewAPNSProvider(APNSConfig{
				KeyFile:     keyFile,
				TeamID:      "TEAM123",
				KeyID:       "KEY123",
				BundleID:    "com.example.app",
				Environment: "production",
				BaseURL:     baseURL,
			})
			testutil.NoError(t, err)

			p.httpClient = http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return mockResponse(tt.status, `{"reason":"`+tt.reason+`"}`, nil), nil
				}),
			}

			_, err = p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
			testutil.True(t, errors.Is(err, tt.wantErr))
			testutil.Contains(t, err.Error(), tt.reason)
		})
	}
}

func TestAPNSProviderErrorMappingFallbackByStatus(t *testing.T) {
	baseURL := "https://apns.example"
	keyFile := writeAPNSKeyFile(t)
	p, err := NewAPNSProvider(APNSConfig{
		KeyFile:     keyFile,
		TeamID:      "TEAM123",
		KeyID:       "KEY123",
		BundleID:    "com.example.app",
		Environment: "production",
		BaseURL:     baseURL,
	})
	testutil.NoError(t, err)

	p.httpClient = http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return mockResponse(http.StatusGone, "", nil), nil
		}),
	}

	_, err = p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
	testutil.True(t, errors.Is(err, ErrUnregistered))
}

func TestAPNSProviderAuthRetryOnce(t *testing.T) {
	var sendCalls int32
	authHeaders := make([]string, 0, 2)
	baseURL := "https://apns.example"

	keyFile := writeAPNSKeyFile(t)
	p, err := NewAPNSProvider(APNSConfig{
		KeyFile:     keyFile,
		TeamID:      "TEAM123",
		KeyID:       "KEY123",
		BundleID:    "com.example.app",
		Environment: "production",
		BaseURL:     baseURL,
	})
	testutil.NoError(t, err)

	p.httpClient = http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			call := atomic.AddInt32(&sendCalls, 1)
			authHeaders = append(authHeaders, r.Header.Get("Authorization"))
			if call == 1 {
				return mockResponse(http.StatusForbidden, `{"reason":"ExpiredProviderToken"}`, nil), nil
			}
			return mockResponse(http.StatusOK, "", map[string]string{"apns-id": "apns-msg-2"}), nil
		}),
	}

	now := time.Unix(1_700_000_000, 0)
	p.now = func() time.Time {
		now = now.Add(time.Second)
		return now
	}

	res, err := p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
	testutil.NoError(t, err)
	testutil.Equal(t, "apns-msg-2", res.MessageID)
	testutil.Equal(t, int32(2), atomic.LoadInt32(&sendCalls))
	testutil.True(t, authHeaders[0] != authHeaders[1])
}

func TestAPNSProviderRejectsReservedDataKeys(t *testing.T) {
	baseURL := "https://apns.example"
	keyFile := writeAPNSKeyFile(t)
	p, err := NewAPNSProvider(APNSConfig{
		KeyFile:     keyFile,
		TeamID:      "TEAM123",
		KeyID:       "KEY123",
		BundleID:    "com.example.app",
		Environment: "production",
		BaseURL:     baseURL,
	})
	testutil.NoError(t, err)

	p.httpClient = http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			t.Fatal("send should not be called when data contains reserved key")
			return nil, nil
		}),
	}

	// "aps" is a reserved APNS key — must not be allowed in user data
	_, err = p.Send(t.Context(), "device-token-1", &Message{
		Title: "A",
		Body:  "B",
		Data:  map[string]string{"aps": "malicious"},
	})
	testutil.True(t, errors.Is(err, ErrInvalidPayload), "expected ErrInvalidPayload for reserved key 'aps', got %v", err)
}

func writeAPNSKeyFile(t *testing.T) string {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testutil.NoError(t, err)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	testutil.NoError(t, err)

	pemBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8,
	})

	path := filepath.Join(t.TempDir(), "AuthKey_TEST.p8")
	err = os.WriteFile(path, pemBlock, 0o600)
	testutil.NoError(t, err)
	return path
}
