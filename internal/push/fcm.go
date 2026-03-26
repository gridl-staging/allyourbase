// Package push FCMProvider sends push notifications via Firebase Cloud Messaging HTTP v1, managing OAuth token authentication and mapping FCM error responses.
package push

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	fcmDefaultBaseURL  = "https://fcm.googleapis.com"
	fcmDefaultTokenURL = "https://oauth2.googleapis.com/token"
	fcmTokenScope      = "https://www.googleapis.com/auth/firebase.messaging"
)

type fcmServiceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type fcmTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// FCMProvider sends push notifications via Firebase Cloud Messaging HTTP v1.
type FCMProvider struct {
	projectID       string
	credentialsFile string
	baseURL         string
	tokenURL        string
	httpClient      http.Client

	clientEmail string
	privateKey  *rsa.PrivateKey

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
	now         func() time.Time
}

// NewFCMProvider creates a provider from a Firebase service-account JSON file.
// If baseURL is empty, the production FCM API endpoint is used.
func NewFCMProvider(credentialsFile, baseURL string) (*FCMProvider, error) {
	if credentialsFile == "" {
		return nil, fmt.Errorf("%w: credentials file is required", ErrProviderAuth)
	}
	if baseURL == "" {
		baseURL = fcmDefaultBaseURL
	}

	raw, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("%w: reading credentials file: %v", ErrProviderAuth, err)
	}

	var creds fcmServiceAccount
	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil, fmt.Errorf("%w: parsing credentials JSON: %v", ErrProviderAuth, err)
	}
	if creds.ProjectID == "" {
		return nil, fmt.Errorf("%w: project_id is required", ErrProviderAuth)
	}
	if creds.ClientEmail == "" {
		return nil, fmt.Errorf("%w: client_email is required", ErrProviderAuth)
	}
	if creds.PrivateKey == "" {
		return nil, fmt.Errorf("%w: private_key is required", ErrProviderAuth)
	}

	privateKey, err := parseRSAPrivateKey(creds.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("%w: parsing private key: %v", ErrProviderAuth, err)
	}

	tokenURL := creds.TokenURI
	if tokenURL == "" {
		tokenURL = fcmDefaultTokenURL
	}

	return &FCMProvider{
		projectID:       creds.ProjectID,
		credentialsFile: credentialsFile,
		baseURL:         strings.TrimRight(baseURL, "/"),
		tokenURL:        tokenURL,
		clientEmail:     creds.ClientEmail,
		privateKey:      privateKey,
		now:             time.Now,
	}, nil
}

func (p *FCMProvider) Send(ctx context.Context, token string, msg *Message) (*Result, error) {
	result, err := p.sendWithCachedToken(ctx, token, msg)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, ErrProviderAuth) {
		return nil, err
	}

	// Token could be expired/revoked. Clear and fetch a fresh token once.
	p.clearCachedToken()
	return p.sendWithCachedToken(ctx, token, msg)
}

// sends a device notification via the FCM HTTP v1 API, building the payload from the message and returning the FCM message ID on success.
func (p *FCMProvider) sendWithCachedToken(ctx context.Context, deviceToken string, msg *Message) (*Result, error) {
	if msg == nil {
		return nil, fmt.Errorf("%w: message is required", ErrProviderError)
	}

	accessToken, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"message": map[string]any{
			"token": deviceToken,
			"notification": map[string]string{
				"title": msg.Title,
				"body":  msg.Body,
			},
		},
	}
	if len(msg.Data) > 0 {
		body["message"].(map[string]any)["data"] = msg.Data
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal payload: %v", ErrProviderError, err)
	}

	endpoint := fmt.Sprintf("%s/v1/projects/%s/messages:send", p.baseURL, p.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrProviderError, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: send request: %v", ErrProviderError, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrProviderError, err)
	}

	if resp.StatusCode >= 300 {
		code, message, sentinel := parseFCMError(resp.StatusCode, respBody)
		return nil, fmt.Errorf("%w: fcm %s: %s", sentinel, code, message)
	}

	var success struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(respBody, &success); err != nil {
		return nil, fmt.Errorf("%w: parse response: %v", ErrProviderError, err)
	}
	if success.Name == "" {
		return nil, fmt.Errorf("%w: missing message id", ErrProviderError)
	}

	return &Result{MessageID: success.Name}, nil
}

// returns a valid OAuth access token for FCM, maintaining a cached token with automatic refresh when it expires or will expire within 5 minutes.
func (p *FCMProvider) getAccessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	if p.accessToken != "" && now.Add(5*time.Minute).Before(p.tokenExpiry) {
		return p.accessToken, nil
	}

	assertion, err := p.buildJWTAssertion(now)
	if err != nil {
		return "", fmt.Errorf("%w: building oauth assertion: %v", ErrProviderAuth, err)
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("%w: build oauth request: %v", ErrProviderAuth, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: send oauth request: %v", ErrProviderAuth, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: read oauth response: %v", ErrProviderAuth, err)
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: oauth status %d: %s", ErrProviderAuth, resp.StatusCode, string(body))
	}

	var tokenResp fcmTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("%w: parse oauth response: %v", ErrProviderAuth, err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("%w: oauth response missing access_token", ErrProviderAuth)
	}
	if tokenResp.ExpiresIn <= 0 {
		tokenResp.ExpiresIn = 3600
	}

	p.accessToken = tokenResp.AccessToken
	p.tokenExpiry = now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return p.accessToken, nil
}

func (p *FCMProvider) buildJWTAssertion(now time.Time) (string, error) {
	claims := jwt.MapClaims{
		"iss":   p.clientEmail,
		"scope": fcmTokenScope,
		"aud":   p.tokenURL,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(p.privateKey)
}

func (p *FCMProvider) clearCachedToken() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.accessToken = ""
	p.tokenExpiry = time.Time{}
}

// decodes a PEM-encoded RSA private key, attempting PKCS8 format first and falling back to PKCS1 for compatibility.
func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, fmt.Errorf("invalid PEM block")
	}

	// Google service-account keys are commonly PKCS8.
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
		return rsaKey, nil
	}

	// Accept PKCS1 for tests/custom setups.
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// parses an FCM API error response from the given status code and body, extracting and returning the error code, message, and a mapped sentinel error.
func parseFCMError(statusCode int, body []byte) (code, message string, sentinel error) {
	code = fmt.Sprintf("HTTP_%d", statusCode)
	message = strings.TrimSpace(string(body))

	var parsed struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Error.Message != "" {
			message = parsed.Error.Message
		}
		for _, detail := range parsed.Error.Details {
			if detail.ErrorCode != "" {
				code = detail.ErrorCode
				break
			}
		}
		if code == fmt.Sprintf("HTTP_%d", statusCode) && parsed.Error.Status != "" {
			code = parsed.Error.Status
		}
	}

	sentinel = mapFCMError(code, message, statusCode)
	return code, message, sentinel
}

// maps FCM error codes and HTTP status codes to sentinel errors, translating Firebase-specific errors to the push package's standard error types.
func mapFCMError(code, message string, statusCode int) error {
	switch code {
	case "UNREGISTERED":
		return ErrUnregistered
	case "INVALID_ARGUMENT":
		if isPayloadTooLarge(message) {
			return ErrPayloadTooLarge
		}
		return ErrInvalidToken
	case "SENDER_ID_MISMATCH":
		return ErrInvalidToken
	case "THIRD_PARTY_AUTH_ERROR", "UNAUTHENTICATED":
		return ErrProviderAuth
	case "QUOTA_EXCEEDED", "UNAVAILABLE", "INTERNAL":
		return ErrProviderError
	}

	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrProviderAuth
	case http.StatusRequestEntityTooLarge:
		return ErrPayloadTooLarge
	default:
		return ErrProviderError
	}
}

func isPayloadTooLarge(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "message too big") ||
		strings.Contains(lower, "payload too large") ||
		strings.Contains(lower, "payload size")
}
