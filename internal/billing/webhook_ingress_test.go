package billing

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestWebhookHandler_ValidSignatureAccepted(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:         "tenant-1",
				Plan:             PlanStarter,
				PaymentStatus:    PaymentStatusPastDue,
				StripeCustomerID: "cus_current",
			},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_checkout", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_new",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, rec.updateStripeStateCount)
	testutil.Equal(t, 1, rec.updatePlanPaymentCount)
	testutil.Equal(t, "tenant-1", rec.updateStripeStateCalls[0].tenantID)
	testutil.Equal(t, "cus_new", rec.updateStripeStateCalls[0].customerID)
	testutil.Equal(t, "sub_1", rec.updateStripeStateCalls[0].subscriptionID)
	testutil.Equal(t, "tenant-1", rec.updatePlanPaymentCalls[0].tenantID)
	testutil.Equal(t, PlanStarter, rec.updatePlanPaymentCalls[0].plan)
	testutil.Equal(t, PaymentStatusActive, rec.updatePlanPaymentCalls[0].status)
}

func TestWebhookHandler_InvalidSignatureRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_invalid", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=bad", time.Now().Unix()))
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)
	testutil.Equal(t, http.StatusForbidden, w.Code)
}

func TestWebhookHandler_MissingSignatureRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_missing_sig", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWebhookHandler_SignatureHeaderMissingTimestampRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_missing_ts", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", "v1=abc123")
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Equal(t, 0, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_SignatureHeaderMissingV1Rejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_missing_v1", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d", time.Now().Unix()))
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Equal(t, 0, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_InvalidTimestampRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_bad_ts", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=not-a-number,v1=%s", computeStripeSignature("not-a-number", string(body), testStripeWebhookSecret)))
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Equal(t, 0, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_MissingRequestBodyRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", nil)
	req.Body = nil
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Equal(t, 0, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_ClosedRequestBodyRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_closed_body", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	testutil.NoError(t, req.Body.Close())
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Equal(t, 0, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_DuplicateEventIsNoop(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {TenantID: "tenant-1"},
		},
		hasProcessedEvents: map[string]bool{
			"evt_duplicate": true,
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_duplicate", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_DedupLookupFailureReturnsInternalServerError(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records:              map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}},
		hasProcessedEventErr: fmt.Errorf("query failed"),
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_dedup_err", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, 1, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_UnknownEventTypeAcknowledged(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_unknown", "customer.subscription.created", map[string]any{
		"id":     "sub_1",
		"status": "active",
		"items": map[string]any{
			"data": []map[string]any{
				{
					"price": map[string]any{
						"id": "price_pro",
					},
				},
			},
		},
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)
	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
}

func TestWebhookHandler_MissingEventIDRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "   ", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Equal(t, 0, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_MissingEventTypeRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_missing_type", "   ", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Equal(t, 0, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_OversizedBodyRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := bytes.Repeat([]byte("a"), stripeWebhookMaxBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	testutil.Equal(t, 0, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_UnmarshalFailureReturnsBadRequest(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{records: map[string]*BillingRecord{"tenant-1": {TenantID: "tenant-1"}}}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader([]byte("{bad json")))
	req.Header.Set("Stripe-Signature", webhookSignatureHeader(t, []byte("{bad json"), testStripeWebhookSecret))
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWebhookHandler_StaleTimestampRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {TenantID: "tenant-1", Plan: PlanStarter, StripeCustomerID: "cus_1"},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_stale", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata":     map[string]any{"tenant_id": "tenant-1"},
	})

	staleTS := time.Now().Add(-10 * time.Minute).Unix()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", webhookSignatureHeaderWithTimestamp(t, body, testStripeWebhookSecret, staleTS))
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_FutureTimestampRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {TenantID: "tenant-1", Plan: PlanStarter, StripeCustomerID: "cus_1"},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_future", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_1",
		"metadata":     map[string]any{"tenant_id": "tenant-1"},
	})

	futureTS := time.Now().Add(10 * time.Minute).Unix()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", webhookSignatureHeaderWithTimestamp(t, body, testStripeWebhookSecret, futureTS))
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusForbidden, w.Code)
	testutil.Equal(t, 0, rec.updateStripeStateCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}
