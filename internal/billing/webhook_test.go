package billing

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestWebhookHandler_RoutesCheckoutSessionCompleted(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {TenantID: "tenant-1", Plan: PlanStarter},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())
	body := webhookEventBody(t, "evt_c1", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_123",
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
	testutil.Equal(t, "tenant-1", rec.updatePlanPaymentCalls[0].tenantID)
	testutil.Equal(t, PlanStarter, rec.updatePlanPaymentCalls[0].plan)
	testutil.Equal(t, PaymentStatusActive, rec.updatePlanPaymentCalls[0].status)
}

func TestWebhookHandler_CheckoutSessionCompleted_MissingIdentifiersRejected(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		object map[string]any
	}{
		{
			name: "missing customer",
			object: map[string]any{
				"id":           "cs_1",
				"subscription": "sub_123",
				"metadata": map[string]any{
					"tenant_id": "tenant-1",
				},
			},
		},
		{
			name: "missing subscription",
			object: map[string]any{
				"id":       "cs_1",
				"customer": "cus_1",
				"metadata": map[string]any{
					"tenant_id": "tenant-1",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &fakeBillingRepo{
				records: map[string]*BillingRecord{
					"tenant-1": {TenantID: "tenant-1", Plan: PlanStarter},
				},
			}
			h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

			body := webhookEventBody(t, "evt_missing_ids", "checkout.session.completed", tc.object)
			req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
			w := httptest.NewRecorder()
			h.HandleWebhook(w, req)

			testutil.Equal(t, http.StatusInternalServerError, w.Code)
			testutil.Equal(t, 0, rec.updateStripeStateCount)
			testutil.Equal(t, 0, rec.updatePlanPaymentCount)
			testutil.Equal(t, 0, rec.recordWebhookEventCount)
		})
	}
}

func TestWebhookHandler_RoutesSubscriptionUpdated(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:             "tenant-1",
				StripeSubscriptionID: "sub_123",
				Plan:                 PlanStarter,
				PaymentStatus:        PaymentStatusUnpaid,
			},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_sub_updated", "customer.subscription.updated", map[string]any{
		"id":     "sub_123",
		"status": "past_due",
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
	testutil.Equal(t, 1, rec.updatePlanPaymentCount)
	testutil.Equal(t, "tenant-1", rec.updatePlanPaymentCalls[0].tenantID)
	testutil.Equal(t, PlanPro, rec.updatePlanPaymentCalls[0].plan)
	testutil.Equal(t, PaymentStatusPastDue, rec.updatePlanPaymentCalls[0].status)
}

func TestWebhookHandler_SubscriptionUpdatedMissingPriceItemRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:             "tenant-1",
				StripeSubscriptionID: "sub_123",
				Plan:                 PlanStarter,
				PaymentStatus:        PaymentStatusUnpaid,
			},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_sub_missing_price", "customer.subscription.updated", map[string]any{
		"id":     "sub_123",
		"status": "active",
		"items": map[string]any{
			"data": []map[string]any{},
		},
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, 1, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_SubscriptionUpdatedUnknownPriceRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:             "tenant-1",
				StripeSubscriptionID: "sub_123",
				Plan:                 PlanStarter,
				PaymentStatus:        PaymentStatusUnpaid,
			},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_sub_unknown_price", "customer.subscription.updated", map[string]any{
		"id":     "sub_123",
		"status": "active",
		"items": map[string]any{
			"data": []map[string]any{
				{
					"price": map[string]any{
						"id": "price_unknown",
					},
				},
			},
		},
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, 1, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_RoutesInvoicePaymentFailed(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:             "tenant-1",
				StripeSubscriptionID: "sub_123",
				Plan:                 PlanPro,
				PaymentStatus:        PaymentStatusActive,
			},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_inv_fail", "invoice.payment_failed", map[string]any{
		"id":           "in_1",
		"subscription": "sub_123",
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, rec.updatePlanPaymentCount)
	testutil.Equal(t, "tenant-1", rec.updatePlanPaymentCalls[0].tenantID)
	testutil.Equal(t, PlanPro, rec.updatePlanPaymentCalls[0].plan)
	testutil.Equal(t, PaymentStatusPastDue, rec.updatePlanPaymentCalls[0].status)
}

func TestWebhookHandler_InvoicePaymentFailedMissingSubscriptionRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:             "tenant-1",
				StripeSubscriptionID: "sub_123",
				Plan:                 PlanPro,
				PaymentStatus:        PaymentStatusActive,
			},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_inv_fail_missing_sub", "invoice.payment_failed", map[string]any{
		"id":           "in_1",
		"subscription": "   ",
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, 1, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_RoutesInvoicePaid(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:             "tenant-1",
				StripeSubscriptionID: "sub_123",
				Plan:                 PlanPro,
				PaymentStatus:        PaymentStatusPastDue,
			},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_inv_paid", "invoice.paid", map[string]any{
		"id":           "in_1",
		"subscription": "sub_123",
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Equal(t, 1, rec.updatePlanPaymentCount)
	testutil.Equal(t, "tenant-1", rec.updatePlanPaymentCalls[0].tenantID)
	testutil.Equal(t, PlanPro, rec.updatePlanPaymentCalls[0].plan)
	testutil.Equal(t, PaymentStatusActive, rec.updatePlanPaymentCalls[0].status)
}

func TestWebhookHandler_InvoicePaidMissingSubscriptionRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:             "tenant-1",
				StripeSubscriptionID: "sub_123",
				Plan:                 PlanPro,
				PaymentStatus:        PaymentStatusPastDue,
			},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_inv_paid_missing_sub", "invoice.paid", map[string]any{
		"id":           "in_1",
		"subscription": "   ",
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, 1, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_RoutesSubscriptionDeleted(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:             "tenant-1",
				StripeSubscriptionID: "sub_123",
				Plan:                 PlanPro,
				PaymentStatus:        PaymentStatusActive,
			},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())
	body := webhookEventBody(t, "evt_sub_deleted", "customer.subscription.deleted", map[string]any{
		"id":     "sub_123",
		"status": "canceled",
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
	testutil.Equal(t, 1, rec.updatePlanPaymentCount)
	testutil.Equal(t, "tenant-1", rec.updatePlanPaymentCalls[0].tenantID)
	testutil.Equal(t, PlanFree, rec.updatePlanPaymentCalls[0].plan)
	testutil.Equal(t, PaymentStatusCanceled, rec.updatePlanPaymentCalls[0].status)
}

func TestWebhookHandler_SubscriptionDeletedMissingIDRejected(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {
				TenantID:             "tenant-1",
				StripeSubscriptionID: "sub_123",
				Plan:                 PlanPro,
				PaymentStatus:        PaymentStatusActive,
			},
		},
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())
	body := webhookEventBody(t, "evt_sub_deleted_missing_id", "customer.subscription.deleted", map[string]any{
		"id":     "   ",
		"status": "canceled",
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

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, 1, rec.hasProcessedEventCount)
	testutil.Equal(t, 0, rec.recordWebhookEventCount)
	testutil.Equal(t, 0, rec.updatePlanPaymentCount)
}

func TestWebhookHandler_RecordWebhookEventFailureReturnsInternalServerError(t *testing.T) {
	t.Parallel()

	rec := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {TenantID: "tenant-1", Plan: PlanStarter},
		},
		recordWebhookErr: fmt.Errorf("write failed"),
	}
	h := NewWebhookHandler(rec, defaultBillingCfg(), testStripeWebhookSecret, testNoopLogger())

	body := webhookEventBody(t, "evt_record_err", "checkout.session.completed", map[string]any{
		"id":           "cs_1",
		"customer":     "cus_1",
		"subscription": "sub_123",
		"metadata": map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	req := newWebhookRequestWithBody(t, body, testStripeWebhookSecret)
	w := httptest.NewRecorder()
	h.HandleWebhook(w, req)

	testutil.Equal(t, http.StatusInternalServerError, w.Code)
	testutil.Equal(t, 1, rec.hasProcessedEventCount)
	testutil.Equal(t, 1, rec.recordWebhookEventCount)
	testutil.Equal(t, 1, rec.updateStripeStateCount)
	testutil.Equal(t, 1, rec.updatePlanPaymentCount)
}
