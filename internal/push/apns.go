// Package push apns.go provides the APNSProvider implementation for sending push notifications to iOS devices using the APNS HTTP API with JWT-based authentication and caching.
package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
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
	apnsProductionBaseURL = "https://api.push.apple.com"
	apnsSandboxBaseURL    = "https://api.sandbox.push.apple.com"
)

// APNSConfig configures APNS provider construction.
type APNSConfig struct {
	KeyFile     string
	TeamID      string
	KeyID       string
	BundleID    string
	Environment string // "production" or "sandbox"
	BaseURL     string // test override; if empty, derived from Environment
}

// APNSProvider sends push notifications via APNS HTTP API.
type APNSProvider struct {
	keyID      string
	teamID     string
	bundleID   string
	privateKey *ecdsa.PrivateKey
	baseURL    string
	httpClient http.Client

	mu        sync.Mutex
	jwt       string
	jwtExpiry time.Time
	now       func() time.Time
}

// NewAPNSProvider creates an APNS provider from a .p8 private key.
func NewAPNSProvider(cfg APNSConfig) (*APNSProvider, error) {
	if cfg.KeyFile == "" {
		return nil, fmt.Errorf("%w: apns key_file is required", ErrProviderAuth)
	}
	if cfg.TeamID == "" {
		return nil, fmt.Errorf("%w: apns team_id is required", ErrProviderAuth)
	}
	if cfg.KeyID == "" {
		return nil, fmt.Errorf("%w: apns key_id is required", ErrProviderAuth)
	}
	if cfg.BundleID == "" {
		return nil, fmt.Errorf("%w: apns bundle_id is required", ErrProviderAuth)
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		switch cfg.Environment {
		case "", "production":
			baseURL = apnsProductionBaseURL
		case "sandbox":
			baseURL = apnsSandboxBaseURL
		default:
			return nil, fmt.Errorf("%w: invalid apns environment %q", ErrProviderAuth, cfg.Environment)
		}
	}

	raw, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("%w: reading apns key file: %v", ErrProviderAuth, err)
	}

	privateKey, err := parseAPNSPrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: parsing apns key: %v", ErrProviderAuth, err)
	}

	return &APNSProvider{
		keyID:      cfg.KeyID,
		teamID:     cfg.TeamID,
		bundleID:   cfg.BundleID,
		privateKey: privateKey,
		baseURL:    baseURL,
		now:        time.Now,
	}, nil
}

func (p *APNSProvider) Send(ctx context.Context, token string, msg *Message) (*Result, error) {
	result, err := p.sendOnce(ctx, token, msg)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, ErrProviderAuth) {
		return nil, err
	}

	// Provider JWT could be stale. Refresh once.
	p.clearCachedJWT()
	return p.sendOnce(ctx, token, msg)
}

// sendOnce sends a push notification to a device via the APNS HTTP API. It constructs the payload from the message, validates that the "aps" key is not used in custom data, and returns the message ID on success.
func (p *APNSProvider) sendOnce(ctx context.Context, deviceToken string, msg *Message) (*Result, error) {
	if msg == nil {
		return nil, fmt.Errorf("%w: message is required", ErrProviderError)
	}

	jws, err := p.getJWT()
	if err != nil {
		return nil, err
	}

	// Reject data keys that would overwrite the reserved "aps" dictionary.
	if _, ok := msg.Data["aps"]; ok {
		return nil, fmt.Errorf("%w: data key %q is reserved by APNS", ErrInvalidPayload, "aps")
	}

	payload := map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{
				"title": msg.Title,
				"body":  msg.Body,
			},
		},
	}
	for k, v := range msg.Data {
		payload[k] = v
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal payload: %v", ErrProviderError, err)
	}

	endpoint := fmt.Sprintf("%s/3/device/%s", p.baseURL, url.PathEscape(deviceToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrProviderError, err)
	}
	req.Header.Set("Authorization", "bearer "+jws)
	req.Header.Set("apns-topic", p.bundleID)
	req.Header.Set("apns-push-type", "alert")
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

	if resp.StatusCode == http.StatusOK {
		messageID := resp.Header.Get("apns-id")
		if messageID == "" {
			messageID = "apns-accepted"
		}
		return &Result{MessageID: messageID}, nil
	}

	reason, sentinel := parseAPNSError(resp.StatusCode, respBody)
	return nil, fmt.Errorf("%w: apns %s", sentinel, reason)
}

// getJWT returns a JWT token for APNS authentication, reusing the cached token if still valid or generating a new ES256-signed token otherwise.
func (p *APNSProvider) getJWT() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	if p.jwt != "" && now.Add(10*time.Minute).Before(p.jwtExpiry) {
		return p.jwt, nil
	}

	claims := jwt.MapClaims{
		"iss": p.teamID,
		"iat": now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = p.keyID

	signed, err := token.SignedString(p.privateKey)
	if err != nil {
		return "", fmt.Errorf("%w: signing apns jwt: %v", ErrProviderAuth, err)
	}

	p.jwt = signed
	p.jwtExpiry = now.Add(time.Hour)
	return p.jwt, nil
}

func (p *APNSProvider) clearCachedJWT() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.jwt = ""
	p.jwtExpiry = time.Time{}
}

func parseAPNSPrivateKey(raw []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("invalid PEM block")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ECDSA")
	}
	return ecdsaKey, nil
}

func parseAPNSError(statusCode int, body []byte) (reason string, sentinel error) {
	reason = strings.TrimSpace(string(body))

	var parsed struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Reason != "" {
		reason = parsed.Reason
	}

	sentinel = mapAPNSError(reason, statusCode)
	return reason, sentinel
}

// mapAPNSError maps APNS error responses to provider-agnostic error types, checking the error reason string first and falling back to HTTP status code if unrecognized.
func mapAPNSError(reason string, statusCode int) error {
	switch reason {
	case "BadDeviceToken", "DeviceTokenNotForTopic":
		return ErrInvalidToken
	case "Unregistered", "ExpiredToken":
		return ErrUnregistered
	case "PayloadTooLarge":
		return ErrPayloadTooLarge
	case "ExpiredProviderToken", "InvalidProviderToken", "MissingProviderToken":
		return ErrProviderAuth
	case "TooManyRequests", "InternalServerError", "ServiceUnavailable", "Shutdown":
		return ErrProviderError
	}

	switch statusCode {
	case http.StatusGone:
		return ErrUnregistered
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrProviderAuth
	case http.StatusRequestEntityTooLarge:
		return ErrPayloadTooLarge
	default:
		return ErrProviderError
	}
}
