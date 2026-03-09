package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const twilioDefaultBaseURL = "https://api.twilio.com"

// TwilioProvider sends SMS via the Twilio REST API.
type TwilioProvider struct {
	accountSID string
	authToken  string
	fromNumber string
	baseURL    string
	client     http.Client
}

// NewTwilioProvider creates a TwilioProvider. If baseURL is empty, the Twilio
// production API is used (useful for tests that pass an httptest server URL).
func NewTwilioProvider(accountSID, authToken, fromNumber, baseURL string) *TwilioProvider {
	if baseURL == "" {
		baseURL = twilioDefaultBaseURL
	}
	return &TwilioProvider{
		accountSID: accountSID,
		authToken:  authToken,
		fromNumber: fromNumber,
		baseURL:    baseURL,
	}
}

func (p *TwilioProvider) Send(ctx context.Context, to, body string) (*SendResult, error) {
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", p.baseURL, p.accountSID)

	form := url.Values{}
	form.Set("To", to)
	form.Set("From", p.fromNumber)
	form.Set("Body", body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("twilio: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.accountSID, p.authToken)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twilio: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("twilio: read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		var errResp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return nil, fmt.Errorf("twilio: error %d: %s", errResp.Code, errResp.Message)
		}
		return nil, fmt.Errorf("twilio: error %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("twilio: parse response: %w", err)
	}

	return &SendResult{
		MessageID: parsed.SID,
		Status:    parsed.Status,
	}, nil
}
