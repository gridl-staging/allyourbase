package billing

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeStripeDoer struct {
	requests []recordedRequest
	status   int
	body     string
}

type recordedRequest struct {
	method string
	path   string
	url    string
	header http.Header
	body   []byte
}

func (f *fakeStripeDoer) Do(req *http.Request) (*http.Response, error) {
	var payload []byte
	if req.Body != nil {
		payload, _ = io.ReadAll(req.Body)
	}
	f.requests = append(f.requests, recordedRequest{
		method: req.Method,
		path:   req.URL.Path,
		url:    req.URL.String(),
		header: req.Header.Clone(),
		body:   payload,
	})
	return &http.Response{
		StatusCode: f.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(f.body)),
	}, nil
}

func newTestAdapter(t *testing.T, status int, body string) (*StripeHTTPAdapter, *fakeStripeDoer) {
	t.Helper()
	d := &fakeStripeDoer{status: status, body: body}
	return NewStripeHTTPAdapter("sk_test_123", StripeAdapterConfig{
		Client: d,
	}), d
}

func parseForm(t *testing.T, raw []byte) url.Values {
	t.Helper()
	v, err := url.ParseQuery(string(raw))
	testutil.NoError(t, err)
	return v
}

func TestStripeAdapter_CreateCustomer_BuildsRequest(t *testing.T) {
	t.Parallel()

	adapter, doer := newTestAdapter(t, http.StatusOK, `{"id":"cus_abc"}`)
	_, err := adapter.CreateCustomer(context.Background(), "tenant-1")
	testutil.NoError(t, err)

	testutil.Equal(t, 1, len(doer.requests))
	req := doer.requests[0]
	testutil.Equal(t, http.MethodPost, req.method)
	testutil.Equal(t, "/v1/customers", req.path)
	testutil.Equal(t, "Bearer sk_test_123", req.header.Get("Authorization"))
	testutil.Equal(t, "ayb-billing:customer:tenant-1", req.header.Get("Idempotency-Key"))
	payload := parseForm(t, req.body)
	testutil.Equal(t, "tenant:tenant-1", payload.Get("description"))
}

func TestStripeAdapter_CreateCheckoutSession_BuildsRequestShape(t *testing.T) {
	t.Parallel()

	adapter, doer := newTestAdapter(t, http.StatusOK, `{"id":"cs_test","url":"https://pay.stripe.com/cs","subscription":"sub_abc"}`)
	_, err := adapter.CreateCheckoutSession(context.Background(), "tenant-1", "cus_tenant_1", "price_starter", "https://example.com/success", "https://example.com/cancel")
	testutil.NoError(t, err)

	req := doer.requests[0]
	testutil.Equal(t, http.MethodPost, req.method)
	testutil.Equal(t, "/v1/checkout/sessions", req.path)
	testutil.Equal(t, "ayb-billing:checkout:tenant-1:price_starter", req.header.Get("Idempotency-Key"))
	payload := parseForm(t, req.body)
	testutil.Equal(t, "cus_tenant_1", payload.Get("customer"))
	testutil.Equal(t, "subscription", payload.Get("mode"))
	testutil.Equal(t, "price_starter", payload.Get("line_items[0][price]"))
	testutil.Equal(t, "1", payload.Get("line_items[0][quantity]"))
	testutil.Equal(t, "card", payload.Get("payment_method_types[0]"))
	testutil.Equal(t, "https://example.com/success", payload.Get("success_url"))
	testutil.Equal(t, "https://example.com/cancel", payload.Get("cancel_url"))
	testutil.Equal(t, "tenant-1", payload.Get("metadata[tenant_id]"))
}

func TestStripeAdapter_GetSubscription_BuildsRequest(t *testing.T) {
	t.Parallel()

	body := `{"id":"sub_abc","status":"active","customer":"cus_123","items":{"data":[{"price":{"id":"price_pro"}}]}}`
	adapter, doer := newTestAdapter(t, http.StatusOK, body)
	_, err := adapter.GetSubscription(context.Background(), "sub_abc")
	testutil.NoError(t, err)

	testutil.Equal(t, 1, len(doer.requests))
	req := doer.requests[0]
	testutil.Equal(t, http.MethodGet, req.method)
	testutil.Equal(t, "/v1/subscriptions/sub_abc", req.path)
}

func TestStripeAdapter_CancelSubscription_BuildsRequest(t *testing.T) {
	t.Parallel()

	adapter, doer := newTestAdapter(t, http.StatusOK, `{"id":"sub_abc","status":"canceled","customer":"cus_123","items":{"data":[{"price":{"id":"price_starter"}}]}}`)
	_, err := adapter.CancelSubscription(context.Background(), "sub_abc")
	testutil.NoError(t, err)

	testutil.Equal(t, 1, len(doer.requests))
	req := doer.requests[0]
	testutil.Equal(t, http.MethodDelete, req.method)
	testutil.Equal(t, "/v1/subscriptions/sub_abc", req.path)
	testutil.Equal(t, "", req.header.Get("Idempotency-Key"))
	testutil.Equal(t, 0, len(req.body))
}

func TestStripeAdapter_PropagatesAPIError(t *testing.T) {
	t.Parallel()

	errorPayload := `{"error":"invalid_request_error"}`
	adapter, doer := newTestAdapter(t, http.StatusBadRequest, errorPayload)
	_, err := adapter.GetSubscription(context.Background(), "sub_bad")
	testutil.ErrorContains(t, err, "stripe error 400")
	testutil.Equal(t, errorPayload, doer.body)
	testutil.Equal(t, 1, len(doer.requests))
}

func TestStripeAdapter_UsesCustomBaseURL(t *testing.T) {
	t.Parallel()

	adapter, doer := newTestAdapter(t, http.StatusOK, `{"id":"cus_abc"}`)
	adapter.baseURL = "https://api.test.stripe.local"
	_, err := adapter.CreateCustomer(context.Background(), "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, "https://api.test.stripe.local/v1/customers", doer.requests[0].url)
}

func TestStripeAdapter_SendMeterEvent_BuildsRequest(t *testing.T) {
	t.Parallel()

	adapter, doer := newTestAdapter(t, http.StatusOK, `{}`)
	err := adapter.SendMeterEvent(context.Background(), "api_requests", "cus_123", 42, "ayb:tenant-1:2024-01-15:api_requests:42")
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(doer.requests))

	req := doer.requests[0]
	testutil.Equal(t, http.MethodPost, req.method)
	testutil.Equal(t, "/v1/billing/meter_events", req.path)
	testutil.Equal(t, "Bearer sk_test_123", req.header.Get("Authorization"))
	testutil.Equal(t, "ayb:tenant-1:2024-01-15:api_requests:42", req.header.Get("Idempotency-Key"))
	testutil.Equal(t, "application/x-www-form-urlencoded", req.header.Get("Content-Type"))

	decoded, err := url.ParseQuery(string(req.body))
	testutil.NoError(t, err)
	testutil.Equal(t, "api_requests", decoded.Get("event_name"))
	testutil.Equal(t, "cus_123", decoded.Get("payload[stripe_customer_id]"))
	testutil.Equal(t, "42", decoded.Get("payload[value]"))
	testutil.Equal(t, "ayb:tenant-1:2024-01-15:api_requests:42", decoded.Get("identifier"))
}
