package push

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestFCMProviderSendSuccess(t *testing.T) {
	var tokenCalls int32
	var sendCalls int32

	tokenURL := "https://oauth.example/token"
	baseURL := "https://fcm.example"

	creds := writeFCMServiceAccountFile(t, tokenURL)
	p, err := NewFCMProvider(creds, baseURL)
	testutil.NoError(t, err)

	p.httpClient = http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.String() {
			case tokenURL:
				atomic.AddInt32(&tokenCalls, 1)
				testutil.Equal(t, http.MethodPost, r.Method)
				testutil.Contains(t, r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
				return mockResponse(http.StatusOK, `{"access_token":"token-1","expires_in":3600,"token_type":"Bearer"}`, map[string]string{"Content-Type": "application/json"}), nil
			case baseURL + "/v1/projects/proj-123/messages:send":
				atomic.AddInt32(&sendCalls, 1)
				testutil.Equal(t, "Bearer token-1", r.Header.Get("Authorization"))
				testutil.Equal(t, http.MethodPost, r.Method)
				testutil.Contains(t, r.Header.Get("Content-Type"), "application/json")

				var payload map[string]any
				err := json.NewDecoder(r.Body).Decode(&payload)
				testutil.NoError(t, err)

				msg, ok := payload["message"].(map[string]any)
				testutil.True(t, ok)
				testutil.Equal(t, "device-token-1", msg["token"].(string))
				notification, ok := msg["notification"].(map[string]any)
				testutil.True(t, ok)
				testutil.Equal(t, "Hello", notification["title"].(string))
				testutil.Equal(t, "World", notification["body"].(string))
				data, ok := msg["data"].(map[string]any)
				testutil.True(t, ok)
				testutil.Equal(t, "v", data["k"].(string))

				return mockResponse(http.StatusOK, `{"name":"projects/proj-123/messages/msg-1"}`, map[string]string{"Content-Type": "application/json"}), nil
			default:
				return mockResponse(http.StatusNotFound, `not found`, nil), nil
			}
		}),
	}

	res, err := p.Send(t.Context(), "device-token-1", &Message{
		Title: "Hello",
		Body:  "World",
		Data:  map[string]string{"k": "v"},
	})
	testutil.NoError(t, err)
	testutil.NotNil(t, res)
	testutil.Equal(t, "projects/proj-123/messages/msg-1", res.MessageID)
	testutil.Equal(t, int32(1), atomic.LoadInt32(&tokenCalls))
	testutil.Equal(t, int32(1), atomic.LoadInt32(&sendCalls))
}

func TestFCMProviderAccessTokenCachingAndRefresh(t *testing.T) {
	var tokenCalls int32
	var sendCalls int32
	seenAuth := make([]string, 0, 3)

	tokenURL := "https://oauth.example/token"
	baseURL := "https://fcm.example"

	creds := writeFCMServiceAccountFile(t, tokenURL)
	p, err := NewFCMProvider(creds, baseURL)
	testutil.NoError(t, err)

	p.httpClient = http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.String() {
			case tokenURL:
				call := atomic.AddInt32(&tokenCalls, 1)
				token := "token-1"
				if call > 1 {
					token = "token-2"
				}
				return mockResponse(http.StatusOK, `{"access_token":"`+token+`","expires_in":3600,"token_type":"Bearer"}`, map[string]string{"Content-Type": "application/json"}), nil
			case baseURL + "/v1/projects/proj-123/messages:send":
				atomic.AddInt32(&sendCalls, 1)
				seenAuth = append(seenAuth, r.Header.Get("Authorization"))
				return mockResponse(http.StatusOK, `{"name":"projects/proj-123/messages/msg-ok"}`, map[string]string{"Content-Type": "application/json"}), nil
			default:
				return mockResponse(http.StatusNotFound, `not found`, nil), nil
			}
		}),
	}

	_, err = p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
	testutil.NoError(t, err)
	_, err = p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
	testutil.NoError(t, err)

	testutil.Equal(t, int32(1), atomic.LoadInt32(&tokenCalls))

	p.mu.Lock()
	p.tokenExpiry = time.Now().Add(4 * time.Minute)
	p.mu.Unlock()

	_, err = p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
	testutil.NoError(t, err)

	testutil.Equal(t, int32(2), atomic.LoadInt32(&tokenCalls))
	testutil.Equal(t, int32(3), atomic.LoadInt32(&sendCalls))
	testutil.Equal(t, "Bearer token-1", seenAuth[0])
	testutil.Equal(t, "Bearer token-1", seenAuth[1])
	testutil.Equal(t, "Bearer token-2", seenAuth[2])
}

func TestFCMProviderErrorMapping(t *testing.T) {
	tokenURL := "https://oauth.example/token"
	baseURL := "https://fcm.example"

	// Generate credentials once for all subtests — RSA key generation is expensive.
	creds := writeFCMServiceAccountFile(t, tokenURL)

	tests := []struct {
		name      string
		errorCode string
		message   string
		wantErr   error
	}{
		{name: "unregistered", errorCode: "UNREGISTERED", message: "Token not registered", wantErr: ErrUnregistered},
		{name: "invalid token", errorCode: "INVALID_ARGUMENT", message: "Invalid registration token", wantErr: ErrInvalidToken},
		{name: "payload too large", errorCode: "INVALID_ARGUMENT", message: "Message too big", wantErr: ErrPayloadTooLarge},
		{name: "quota exceeded", errorCode: "QUOTA_EXCEEDED", message: "Quota exceeded", wantErr: ErrProviderError},
		{name: "provider auth", errorCode: "THIRD_PARTY_AUTH_ERROR", message: "APNS cert invalid", wantErr: ErrProviderAuth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewFCMProvider(creds, baseURL)
			testutil.NoError(t, err)

			p.httpClient = http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					switch r.URL.String() {
					case tokenURL:
						return mockResponse(http.StatusOK, `{"access_token":"token-1","expires_in":3600,"token_type":"Bearer"}`, map[string]string{"Content-Type": "application/json"}), nil
					case baseURL + "/v1/projects/proj-123/messages:send":
						return mockResponse(http.StatusBadRequest, `{"error":{"code":400,"message":"`+tt.message+`","status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"`+tt.errorCode+`"}]}}`, nil), nil
					default:
						return mockResponse(http.StatusNotFound, `not found`, nil), nil
					}
				}),
			}

			_, err = p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
			testutil.True(t, errors.Is(err, tt.wantErr))
			testutil.Contains(t, err.Error(), tt.errorCode)
		})
	}
}

func TestFCMProviderAuthRetryOnce(t *testing.T) {
	var tokenCalls int32
	var sendCalls int32

	tokenURL := "https://oauth.example/token"
	baseURL := "https://fcm.example"

	creds := writeFCMServiceAccountFile(t, tokenURL)
	p, err := NewFCMProvider(creds, baseURL)
	testutil.NoError(t, err)

	p.httpClient = http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.String() {
			case tokenURL:
				call := atomic.AddInt32(&tokenCalls, 1)
				token := "token-1"
				if call > 1 {
					token = "token-2"
				}
				return mockResponse(http.StatusOK, `{"access_token":"`+token+`","expires_in":3600,"token_type":"Bearer"}`, map[string]string{"Content-Type": "application/json"}), nil
			case baseURL + "/v1/projects/proj-123/messages:send":
				call := atomic.AddInt32(&sendCalls, 1)
				if call == 1 {
					return mockResponse(http.StatusUnauthorized, `{"error":{"code":401,"message":"auth failed","status":"UNAUTHENTICATED","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"THIRD_PARTY_AUTH_ERROR"}]}}`, nil), nil
				}
				return mockResponse(http.StatusOK, `{"name":"projects/proj-123/messages/msg-2"}`, map[string]string{"Content-Type": "application/json"}), nil
			default:
				return mockResponse(http.StatusNotFound, `not found`, nil), nil
			}
		}),
	}

	res, err := p.Send(t.Context(), "device-token-1", &Message{Title: "A", Body: "B"})
	testutil.NoError(t, err)
	testutil.Equal(t, "projects/proj-123/messages/msg-2", res.MessageID)
	testutil.Equal(t, int32(2), atomic.LoadInt32(&tokenCalls))
	testutil.Equal(t, int32(2), atomic.LoadInt32(&sendCalls))
}

func writeFCMServiceAccountFile(t *testing.T, tokenURL string) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	pemKey := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyBytes,
	})

	payload := map[string]string{
		"type":         "service_account",
		"project_id":   "proj-123",
		"private_key":  string(pemKey),
		"client_email": "svc@example.iam.gserviceaccount.com",
		"token_uri":    tokenURL,
	}
	raw, err := json.Marshal(payload)
	testutil.NoError(t, err)

	path := filepath.Join(t.TempDir(), "service_account.json")
	err = os.WriteFile(path, raw, 0o600)
	testutil.NoError(t, err)
	return path
}

func TestFCMProviderNewRequiresProjectID(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	pemKey := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyBytes,
	})

	payload := map[string]string{
		"type":         "service_account",
		"private_key":  string(pemKey),
		"client_email": "svc@example.iam.gserviceaccount.com",
		"token_uri":    "https://oauth2.googleapis.com/token",
	}
	raw, err := json.Marshal(payload)
	testutil.NoError(t, err)

	path := filepath.Join(t.TempDir(), "service_account.json")
	err = os.WriteFile(path, raw, 0o600)
	testutil.NoError(t, err)

	_, err = NewFCMProvider(path, "")
	testutil.ErrorContains(t, err, "project_id")
}
