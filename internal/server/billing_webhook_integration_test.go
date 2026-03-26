//go:build integration

package server_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

const webhookTestSecret = "whsec_test_secret"

func stripeSignature(t *testing.T, payload []byte, secret string, timestamp int64) string {
	t.Helper()
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(fmt.Sprintf("%d", timestamp)))
	h.Write([]byte("."))
	h.Write(payload)
	return "t=" + strconv.FormatInt(timestamp, 10) + ",v1=" + hex.EncodeToString(h.Sum(nil))
}

func setupBillingWebhookIntegrationTest(t *testing.T) (context.Context, *billing.Store, string) {
	t.Helper()

	ctx := context.Background()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)
	ensureIntegrationMigrations(t, ctx)

	cfg := config.Default()
	cfg.Billing.Provider = "stripe"
	cfg.Billing.StripeSecretKey = "sk_test_123"
	cfg.Billing.StripeWebhookSecret = webhookTestSecret
	cfg.Billing.StripeStarterPriceID = "price_starter"
	cfg.Billing.StripeProPriceID = "price_pro"
	cfg.Billing.StripeEnterprisePriceID = "price_enterprise"
	cfg.Billing.UsageSyncIntervalSecs = 3600
	cfg.Billing.StripeMeterAPIRequests = "meter_api_requests"
	cfg.Billing.StripeMeterStorageBytes = "meter_storage_bytes"
	cfg.Billing.StripeMeterBandwidthBytes = "meter_bandwidth_bytes"
	cfg.Billing.StripeMeterFunctionInvs = "meter_function_invocations"

	srv := server.New(cfg, testutil.DiscardLogger(), nil, sharedPG.Pool, nil, nil)
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)

	return ctx, billing.NewStore(sharedPG.Pool), ts.URL
}

// ensureBillingTestTenant inserts a tenant row into _ayb_tenants so that
// billing records satisfy the foreign key constraint on tenant_id.
func ensureBillingTestTenant(t *testing.T, ctx context.Context, tenantID string) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx,
		`INSERT INTO _ayb_tenants (id, name, slug) VALUES ($1, $2, $3)`,
		tenantID, "billing-test-"+tenantID, "billing-test-"+tenantID,
	)
	testutil.NoError(t, err)
}

func postStripeWebhookEvent(t *testing.T, baseURL string, event map[string]any) {
	t.Helper()

	payload, err := json.Marshal(event)
	testutil.NoError(t, err)

	timestamp := time.Now().Unix()
	sig := stripeSignature(t, payload, webhookTestSecret, timestamp)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/webhooks/stripe", bytes.NewReader(payload))
	testutil.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", sig)

	resp, err := http.DefaultClient.Do(req)
	testutil.NoError(t, err)
	defer resp.Body.Close()
	testutil.StatusCode(t, http.StatusOK, resp.StatusCode)
}

// TestBillingWebhookIntegration_CheckoutSessionCompleted proves that a checkout.session.completed
// webhook event drives persisted billing state transitions end-to-end.
func TestBillingWebhookIntegration_CheckoutSessionCompleted(t *testing.T) {
	ctx, store, baseURL := setupBillingWebhookIntegrationTest(t)

	tenantID := "00000000-0000-0000-0000-000000000001"
	ensureBillingTestTenant(t, ctx, tenantID)
	_, err := store.Create(ctx, tenantID)
	testutil.NoError(t, err)

	err = store.UpdatePlanAndPayment(ctx, tenantID, billing.PlanPro, billing.PaymentStatusIncomplete)
	testutil.NoError(t, err)

	event := map[string]any{
		"id":   "evt_test_checkout_completed",
		"type": "checkout.session.completed",
		"data": map[string]any{
			"object": map[string]any{
				"id":           "cs_test",
				"customer":     "cus_test",
				"subscription": "sub_test",
				"metadata": map[string]any{
					"tenant_id": tenantID,
				},
			},
		},
	}
	postStripeWebhookEvent(t, baseURL, event)

	rec, err := store.Get(ctx, tenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, "cus_test", rec.StripeCustomerID)
	testutil.Equal(t, "sub_test", rec.StripeSubscriptionID)
	testutil.Equal(t, billing.PlanPro, rec.Plan)
	testutil.Equal(t, billing.PaymentStatusActive, rec.PaymentStatus)
}

// TestBillingWebhookIntegration_SubscriptionUpdated proves that a customer.subscription.updated
// webhook event syncs plan and payment status from Stripe.
func TestBillingWebhookIntegration_SubscriptionUpdated(t *testing.T) {
	ctx, store, baseURL := setupBillingWebhookIntegrationTest(t)

	tenantID := "00000000-0000-0000-0000-000000000002"
	ensureBillingTestTenant(t, ctx, tenantID)
	_, err := store.Create(ctx, tenantID)
	testutil.NoError(t, err)

	err = store.UpdateStripeState(ctx, tenantID, "cus_test", "sub_existing")
	testutil.NoError(t, err)
	err = store.UpdatePlanAndPayment(ctx, tenantID, billing.PlanStarter, billing.PaymentStatusActive)
	testutil.NoError(t, err)

	event := map[string]any{
		"id":   "evt_test_sub_updated",
		"type": "customer.subscription.updated",
		"data": map[string]any{
			"object": map[string]any{
				"id":       "sub_existing",
				"status":   "past_due",
				"customer": "cus_test",
				"items": map[string]any{
					"data": []map[string]any{
						{
							"price": map[string]any{
								"id": "price_enterprise",
							},
						},
					},
				},
			},
		},
	}
	postStripeWebhookEvent(t, baseURL, event)

	rec, err := store.Get(ctx, tenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, billing.PlanEnterprise, rec.Plan)
	testutil.Equal(t, billing.PaymentStatusPastDue, rec.PaymentStatus)
}

// TestBillingWebhookIntegration_SubscriptionDeleted proves that a customer.subscription.deleted
// webhook event downgrades to free and sets status to canceled.
func TestBillingWebhookIntegration_SubscriptionDeleted(t *testing.T) {
	ctx, store, baseURL := setupBillingWebhookIntegrationTest(t)

	tenantID := "00000000-0000-0000-0000-000000000003"
	ensureBillingTestTenant(t, ctx, tenantID)
	_, err := store.Create(ctx, tenantID)
	testutil.NoError(t, err)

	err = store.UpdateStripeState(ctx, tenantID, "cus_test", "sub_delete_me")
	testutil.NoError(t, err)
	err = store.UpdatePlanAndPayment(ctx, tenantID, billing.PlanPro, billing.PaymentStatusActive)
	testutil.NoError(t, err)

	event := map[string]any{
		"id":   "evt_test_sub_deleted",
		"type": "customer.subscription.deleted",
		"data": map[string]any{
			"object": map[string]any{
				"id":       "sub_delete_me",
				"status":   "canceled",
				"customer": "cus_test",
			},
		},
	}
	postStripeWebhookEvent(t, baseURL, event)

	rec, err := store.Get(ctx, tenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, billing.PlanFree, rec.Plan)
	testutil.Equal(t, billing.PaymentStatusCanceled, rec.PaymentStatus)
}

// TestBillingWebhookIntegration_InvoicePaymentFailed proves that an invoice.payment_failed
// webhook event updates payment status to past_due without changing the plan.
func TestBillingWebhookIntegration_InvoicePaymentFailed(t *testing.T) {
	ctx, store, baseURL := setupBillingWebhookIntegrationTest(t)

	tenantID := "00000000-0000-0000-0000-000000000004"
	ensureBillingTestTenant(t, ctx, tenantID)
	_, err := store.Create(ctx, tenantID)
	testutil.NoError(t, err)

	err = store.UpdateStripeState(ctx, tenantID, "cus_test", "sub_invoice_failed")
	testutil.NoError(t, err)
	err = store.UpdatePlanAndPayment(ctx, tenantID, billing.PlanPro, billing.PaymentStatusActive)
	testutil.NoError(t, err)

	event := map[string]any{
		"id":   "evt_test_invoice_failed",
		"type": "invoice.payment_failed",
		"data": map[string]any{
			"object": map[string]any{
				"id":           "inv_test",
				"subscription": "sub_invoice_failed",
				"customer":     "cus_test",
			},
		},
	}
	postStripeWebhookEvent(t, baseURL, event)

	rec, err := store.Get(ctx, tenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, billing.PlanPro, rec.Plan)
	testutil.Equal(t, billing.PaymentStatusPastDue, rec.PaymentStatus)
}

// TestBillingWebhookIntegration_InvoicePaid proves that an invoice.paid
// webhook event updates payment status to active without changing the plan.
func TestBillingWebhookIntegration_InvoicePaid(t *testing.T) {
	ctx, store, baseURL := setupBillingWebhookIntegrationTest(t)

	tenantID := "00000000-0000-0000-0000-000000000005"
	ensureBillingTestTenant(t, ctx, tenantID)
	_, err := store.Create(ctx, tenantID)
	testutil.NoError(t, err)

	err = store.UpdateStripeState(ctx, tenantID, "cus_test", "sub_invoice_paid")
	testutil.NoError(t, err)
	err = store.UpdatePlanAndPayment(ctx, tenantID, billing.PlanStarter, billing.PaymentStatusPastDue)
	testutil.NoError(t, err)

	event := map[string]any{
		"id":   "evt_test_invoice_paid",
		"type": "invoice.paid",
		"data": map[string]any{
			"object": map[string]any{
				"id":           "inv_test_paid",
				"subscription": "sub_invoice_paid",
				"customer":     "cus_test",
			},
		},
	}
	postStripeWebhookEvent(t, baseURL, event)

	rec, err := store.Get(ctx, tenantID)
	testutil.NoError(t, err)
	testutil.Equal(t, billing.PlanStarter, rec.Plan)
	testutil.Equal(t, billing.PaymentStatusActive, rec.PaymentStatus)
}
