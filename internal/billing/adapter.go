// Package billing StripeHTTPAdapter implements the StripeAdapter interface to communicate with the Stripe API via HTTP with Bearer token authentication, idempotency key support, and JSON response handling.
package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// StripeAdapter defines the methods needed by BillingService for Stripe lifecycle calls.
type StripeAdapter interface {
	CreateCustomer(ctx context.Context, tenantID string) (*stripeCustomerResponse, error)
	CreateCheckoutSession(ctx context.Context, tenantID, customerID, priceID, successURL, cancelURL string) (*stripeCheckoutSessionResponse, error)
	GetSubscription(ctx context.Context, subscriptionID string) (*stripeSubscriptionResponse, error)
	CancelSubscription(ctx context.Context, subscriptionID string) (*stripeSubscriptionResponse, error)
	SendMeterEvent(ctx context.Context, eventName string, customerID string, value int64, identifier string) error
}

type doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type StripeHTTPAdapter struct {
	apiKey  string
	baseURL string
	doer    doer
}

type StripeAdapterConfig struct {
	BaseURL string
	Client  doer
}

// NewStripeHTTPAdapter creates a Stripe adapter with optional overrides for testability.
func NewStripeHTTPAdapter(apiKey string, cfg StripeAdapterConfig) *StripeHTTPAdapter {
	baseURL := "https://api.stripe.com"
	if strings.TrimSpace(cfg.BaseURL) != "" {
		baseURL = strings.TrimRight(cfg.BaseURL, "/")
	}
	var client doer = http.DefaultClient
	if cfg.Client != nil {
		client = cfg.Client
	}
	return &StripeHTTPAdapter{
		apiKey:  apiKey,
		baseURL: baseURL,
		doer:    client,
	}
}

func (a *StripeHTTPAdapter) CreateCustomer(ctx context.Context, tenantID string) (*stripeCustomerResponse, error) {
	payload := url.Values{
		"description": []string{"tenant:" + tenantID},
	}
	var out stripeCustomerResponse
	if err := a.post(ctx, "/v1/customers", payload, stripeIDempotencyKey("customer", tenantID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateCheckoutSession creates a subscription checkout session for the given customer and price, returning the session URL and ID for payment collection.
func (a *StripeHTTPAdapter) CreateCheckoutSession(
	ctx context.Context,
	tenantID string,
	customerID string,
	priceID string,
	successURL string,
	cancelURL string,
) (*stripeCheckoutSessionResponse, error) {
	payload := url.Values{
		"mode":                    []string{"subscription"},
		"customer":                []string{customerID},
		"line_items[0][price]":    []string{priceID},
		"line_items[0][quantity]": []string{"1"},
		"payment_method_types[0]": []string{"card"},
		"success_url":             []string{successURL},
		"cancel_url":              []string{cancelURL},
		"metadata[tenant_id]":     []string{tenantID},
	}
	var out stripeCheckoutSessionResponse
	if err := a.post(ctx, "/v1/checkout/sessions", payload, stripeIDempotencyKey("checkout", tenantID, priceID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *StripeHTTPAdapter) GetSubscription(ctx context.Context, subscriptionID string) (*stripeSubscriptionResponse, error) {
	var out stripeSubscriptionResponse
	if err := a.get(ctx, "/v1/subscriptions/"+url.PathEscape(subscriptionID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *StripeHTTPAdapter) CancelSubscription(ctx context.Context, subscriptionID string) (*stripeSubscriptionResponse, error) {
	var out stripeSubscriptionResponse
	if err := a.request(ctx, http.MethodDelete, "/v1/subscriptions/"+url.PathEscape(subscriptionID), nil, "", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *StripeHTTPAdapter) SendMeterEvent(ctx context.Context, eventName string, customerID string, value int64, identifier string) error {
	payload := url.Values{
		"event_name":                  []string{eventName},
		"payload[stripe_customer_id]": []string{customerID},
		"payload[value]":              []string{fmt.Sprintf("%d", value)},
		"identifier":                  []string{identifier},
	}
	return a.post(ctx, "/v1/billing/meter_events", payload, identifier, nil)
}

func (a *StripeHTTPAdapter) get(ctx context.Context, path string, payload url.Values, out any) error {
	return a.request(ctx, http.MethodGet, path, payload, "", out)
}

func (a *StripeHTTPAdapter) post(ctx context.Context, path string, payload url.Values, idempotencyKey string, out any) error {
	return a.request(ctx, http.MethodPost, path, payload, idempotencyKey, out)
}

// request executes an HTTP request to a Stripe API endpoint with Bearer token authentication and an optional idempotency key, returning stripeError on non-2xx responses and decoding successful JSON responses into out.
func (a *StripeHTTPAdapter) request(ctx context.Context, method, path string, payload url.Values, idempotencyKey string, out any) error {
	path = ensureLeadingSlash(path)
	requestURL := a.baseURL + path

	var body io.Reader
	if method == http.MethodPost && payload != nil {
		body = bytes.NewBufferString(payload.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("build stripe request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := a.doer.Do(req)
	if err != nil {
		return fmt.Errorf("stripe request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return &stripeError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(bodyBytes)),
		}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode stripe response: %w", err)
	}
	return nil
}

func ensureLeadingSlash(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

type stripeError struct {
	StatusCode int
	Body       string
}

func (e *stripeError) Error() string {
	return fmt.Sprintf("stripe error %d: %s", e.StatusCode, e.Body)
}

func stripeIDempotencyKey(parts ...string) string {
	return fmt.Sprintf("%s:%s", billingIDPrefix, strings.Join(parts, ":"))
}

const billingIDPrefix = "ayb-billing"

type stripeCustomerResponse struct {
	ID string `json:"id"`
}

type stripeCheckoutSessionResponse struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	Customer     string `json:"customer"`
	Subscription string `json:"subscription"`
}

type stripeSubscriptionResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Customer string `json:"customer"`
	Items    struct {
		Data []struct {
			Price struct {
				ID string `json:"id"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}
