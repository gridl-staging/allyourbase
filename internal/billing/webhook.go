// Package billing implements Stripe webhook handlers that validate incoming webhook signatures and route events to update billing records with subscription and payment status changes.
package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/httputil"
)

// stripeWebhookTimestampTolerance is the maximum age of a Stripe webhook
// timestamp before the signature is rejected. Matches Stripe's default of
// 5 minutes to prevent replay attacks using captured payloads.
const stripeWebhookTimestampTolerance = 5 * time.Minute

// webhookNow returns the current time. Overridden in tests.
var webhookNow = time.Now

const stripeWebhookMaxBodyBytes = 64 * 1024

// WebhookHandler handles Stripe webhook events.
type WebhookHandler struct {
	repo          BillingRepository
	cfg           config.BillingConfig
	webhookSecret string
	logger        *slog.Logger
}

// NewWebhookHandler creates a new handler for processing Stripe webhooks.
func NewWebhookHandler(repo BillingRepository, cfg config.BillingConfig, webhookSecret string, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		repo:          repo,
		cfg:           cfg,
		webhookSecret: webhookSecret,
		logger:        logger,
	}
}

type stripeWebhookEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

type stripeWebhookSignature struct {
	timestamp  string
	signatures []string
}

type stripeCheckoutSessionData struct {
	ID           string            `json:"id"`
	Customer     string            `json:"customer"`
	Subscription string            `json:"subscription"`
	Metadata     map[string]string `json:"metadata"`
}

type stripeSubscriptionData struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Items  struct {
		Data []struct {
			Price struct {
				ID string `json:"id"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}

type stripeInvoiceData struct {
	Subscription string `json:"subscription"`
}

var (
	errMissingStripeSignature = errors.New("missing stripe signature")
	errInvalidStripeSignature = errors.New("invalid stripe signature")
	errStaleStripeTimestamp   = errors.New("stale stripe webhook timestamp")
)

// HandleWebhook processes Stripe webhook payloads.
func (h *WebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.repo == nil {
		http.Error(w, "handler not configured", http.StatusInternalServerError)
		return
	}

	body, err := h.readBody(w, r)
	if err != nil {
		if errors.Is(err, errBodyTooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid webhook body", http.StatusBadRequest)
		return
	}

	if err := h.verifyStripeSignature(r, body); err != nil {
		status := http.StatusForbidden
		if errors.Is(err, errMissingStripeSignature) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}

	var event stripeWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(event.ID) == "" {
		http.Error(w, "missing event id", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(event.Type) == "" {
		http.Error(w, "missing event type", http.StatusBadRequest)
		return
	}

	alreadyProcessed, err := h.repo.HasProcessedEvent(r.Context(), event.ID)
	if err != nil {
		h.loggerError("failed checking webhook dedup", "error", err, "event_id", event.ID)
		http.Error(w, "failed webhook dedup", http.StatusInternalServerError)
		return
	}
	if alreadyProcessed {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	err = h.routeWebhookEvent(r.Context(), event)
	if err != nil {
		h.loggerError("failed handling stripe webhook", "error", err, "event_id", event.ID, "event_type", event.Type)
		http.Error(w, "failed handling webhook", http.StatusInternalServerError)
		return
	}

	if err := h.repo.RecordWebhookEvent(r.Context(), event.ID, event.Type, body); err != nil {
		h.loggerError("failed to record stripe webhook event", "error", err, "event_id", event.ID, "event_type", event.Type)
		http.Error(w, "failed recording webhook event", http.StatusInternalServerError)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// routeWebhookEvent dispatches webhook events to their respective handlers based on event type, returning nil for unhandled event types.
func (h *WebhookHandler) routeWebhookEvent(ctx context.Context, event stripeWebhookEvent) error {
	switch event.Type {
	case "checkout.session.completed":
		return h.handleCheckoutSessionCompleted(ctx, event)
	case "customer.subscription.updated":
		return h.handleSubscriptionUpdated(ctx, event)
	case "customer.subscription.deleted":
		return h.handleSubscriptionDeleted(ctx, event)
	case "invoice.payment_failed":
		return h.handleInvoicePaymentFailed(ctx, event)
	case "invoice.paid":
		return h.handleInvoicePaid(ctx, event)
	default:
		if h.logger != nil {
			h.logger.Debug("unhandled stripe webhook event", "event_id", event.ID, "event_type", event.Type)
		}
		return nil
	}
}

// handleCheckoutSessionCompleted processes a checkout session completion webhook, validating required metadata and customer information, updating Stripe state, and marking the billing record as active.
func (h *WebhookHandler) handleCheckoutSessionCompleted(ctx context.Context, event stripeWebhookEvent) error {
	var data stripeCheckoutSessionData
	if err := json.Unmarshal(event.Data.Object, &data); err != nil {
		return fmt.Errorf("decode checkout.session.completed data: %w", err)
	}
	tenantID := strings.TrimSpace(data.Metadata["tenant_id"])
	if tenantID == "" {
		return fmt.Errorf("missing tenant_id in checkout.session.completed metadata")
	}
	customerID := strings.TrimSpace(data.Customer)
	if customerID == "" {
		return fmt.Errorf("missing customer id in checkout.session.completed")
	}
	subscriptionID := strings.TrimSpace(data.Subscription)
	if subscriptionID == "" {
		return fmt.Errorf("missing subscription id in checkout.session.completed")
	}
	if err := h.repo.UpdateStripeState(ctx, tenantID, customerID, subscriptionID); err != nil {
		return fmt.Errorf("update stripe state: %w", err)
	}
	rec, err := h.repo.Get(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("get billing record for checkout.session.completed: %w", err)
	}
	return h.repo.UpdatePlanAndPayment(ctx, tenantID, rec.Plan, PaymentStatusActive)
}

// handleSubscriptionUpdated processes subscription update webhooks, mapping the new price to a billing plan and updating the billing record's plan and payment status accordingly.
func (h *WebhookHandler) handleSubscriptionUpdated(ctx context.Context, event stripeWebhookEvent) error {
	var data stripeSubscriptionData
	if err := json.Unmarshal(event.Data.Object, &data); err != nil {
		return fmt.Errorf("decode customer.subscription.updated data: %w", err)
	}
	rec, err := h.repo.GetBySubscriptionID(ctx, strings.TrimSpace(data.ID))
	if err != nil {
		return fmt.Errorf("lookup billing record for subscription update: %w", err)
	}
	if len(data.Items.Data) == 0 {
		return fmt.Errorf("subscription update missing price item")
	}
	plan, err := mapPlanFromPriceID(h.cfg, data.Items.Data[0].Price.ID)
	if err != nil {
		return fmt.Errorf("map plan from price: %w", err)
	}
	return h.repo.UpdatePlanAndPayment(ctx, rec.TenantID, plan, mapPaymentStatus(data.Status))
}

func (h *WebhookHandler) handleSubscriptionDeleted(ctx context.Context, event stripeWebhookEvent) error {
	var data stripeSubscriptionData
	if err := json.Unmarshal(event.Data.Object, &data); err != nil {
		return fmt.Errorf("decode customer.subscription.deleted data: %w", err)
	}
	rec, err := h.repo.GetBySubscriptionID(ctx, strings.TrimSpace(data.ID))
	if err != nil {
		return fmt.Errorf("lookup billing record for subscription deletion: %w", err)
	}
	return h.repo.UpdatePlanAndPayment(ctx, rec.TenantID, PlanFree, PaymentStatusCanceled)
}

func (h *WebhookHandler) handleInvoicePaymentFailed(ctx context.Context, event stripeWebhookEvent) error {
	var data stripeInvoiceData
	if err := json.Unmarshal(event.Data.Object, &data); err != nil {
		return fmt.Errorf("decode invoice.payment_failed data: %w", err)
	}
	rec, err := h.repo.GetBySubscriptionID(ctx, strings.TrimSpace(data.Subscription))
	if err != nil {
		return fmt.Errorf("lookup billing record for invoice payment failed: %w", err)
	}
	return h.repo.UpdatePlanAndPayment(ctx, rec.TenantID, rec.Plan, PaymentStatusPastDue)
}

func (h *WebhookHandler) handleInvoicePaid(ctx context.Context, event stripeWebhookEvent) error {
	var data stripeInvoiceData
	if err := json.Unmarshal(event.Data.Object, &data); err != nil {
		return fmt.Errorf("decode invoice.paid data: %w", err)
	}
	rec, err := h.repo.GetBySubscriptionID(ctx, strings.TrimSpace(data.Subscription))
	if err != nil {
		return fmt.Errorf("lookup billing record for invoice paid: %w", err)
	}
	return h.repo.UpdatePlanAndPayment(ctx, rec.TenantID, rec.Plan, PaymentStatusActive)
}

// verifyStripeSignature validates the incoming webhook request by parsing the Stripe-Signature header and verifying the HMAC-SHA256 signature against the webhook secret.
func (h *WebhookHandler) verifyStripeSignature(r *http.Request, body []byte) error {
	if h == nil {
		return fmt.Errorf("invalid webhook handler")
	}

	sigHeader := strings.TrimSpace(r.Header.Get("Stripe-Signature"))
	parsed, err := parseStripeSignature(sigHeader)
	if err != nil {
		return err
	}
	ts, err := strconv.ParseInt(parsed.timestamp, 10, 64)
	if err != nil {
		return err
	}
	if age := webhookNow().Sub(time.Unix(ts, 0)); age > stripeWebhookTimestampTolerance || age < -stripeWebhookTimestampTolerance {
		return errStaleStripeTimestamp
	}

	expectedHex := computeStripeSignature(parsed.timestamp, string(body), h.webhookSecret)
	expected := []byte(expectedHex)
	for _, signature := range parsed.signatures {
		if hmac.Equal(expected, []byte(signature)) {
			return nil
		}
	}
	return errInvalidStripeSignature
}

// parseStripeSignature extracts the timestamp and version 1 signatures from a Stripe-Signature header value, returning an error if required fields are missing.
func parseStripeSignature(header string) (stripeWebhookSignature, error) {
	if strings.TrimSpace(header) == "" {
		return stripeWebhookSignature{}, errMissingStripeSignature
	}

	parts := strings.Split(header, ",")
	sig := stripeWebhookSignature{}
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			sig.timestamp = kv[1]
		case "v1":
			sig.signatures = append(sig.signatures, kv[1])
		}
	}
	if sig.timestamp == "" {
		return stripeWebhookSignature{}, fmt.Errorf("invalid stripe signature header")
	}
	if len(sig.signatures) == 0 {
		return stripeWebhookSignature{}, errInvalidStripeSignature
	}
	return sig, nil
}

func computeStripeSignature(timestamp, body, secret string) string {
	msg := timestamp + "." + body
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

var errBodyTooLarge = errors.New("request body too large")

// readBody reads the HTTP request body with a maximum size limit of 64 KB, returning an error if the body exceeds this limit or cannot be read.
func (h *WebhookHandler) readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("invalid request")
	}
	if r.Body == nil {
		return nil, fmt.Errorf("missing request body")
	}
	var reader io.Reader
	if w != nil {
		reader = http.MaxBytesReader(w, r.Body, stripeWebhookMaxBodyBytes)
	} else {
		reader = io.LimitReader(r.Body, stripeWebhookMaxBodyBytes+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		if errors.Is(err, http.ErrBodyReadAfterClose) {
			return nil, err
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, errBodyTooLarge
		}
		return nil, fmt.Errorf("invalid webhook body")
	}
	if len(body) > stripeWebhookMaxBodyBytes {
		return nil, errBodyTooLarge
	}
	return body, nil
}

func (h *WebhookHandler) loggerError(msg string, args ...any) {
	if h == nil || h.logger == nil {
		return
	}
	h.logger.Error(msg, args...)
}
