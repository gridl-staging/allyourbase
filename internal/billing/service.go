// Package billing provides Stripe-integrated subscription and usage tracking services for managing tenant billing state.
package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/allyourbase/ayb/internal/config"
)

var (
	ErrSubscriptionNotFound   = errors.New("billing subscription not found")
	ErrUnsupportedPlan        = errors.New("unsupported billing plan")
	ErrStripeSubscriptionFail = errors.New("stripe subscription flow failed")
	ErrUnknownStripePriceID   = errors.New("unknown stripe price id")
)

// NewNoopBillingService builds a disabled-mode service that never mutates state.
func NewNoopBillingService() BillingService {
	return &noopBillingService{}
}

// NewStripeBillingService builds a Stripe-backed implementation using the provided
// repository and transport adapter.
func NewStripeBillingService(repo BillingRepository, cfg config.BillingConfig, adapter StripeAdapter, logger *slog.Logger) BillingService {
	return &stripeBillingService{
		repo:    repo,
		cfg:     cfg,
		adapter: adapter,
		logger:  logger,
	}
}

type stripeBillingService struct {
	repo    BillingRepository
	cfg     config.BillingConfig
	adapter StripeAdapter
	logger  *slog.Logger
}

type noopBillingService struct{}

func (s *noopBillingService) CreateCustomer(ctx context.Context, tenantID string) (*Customer, error) {
	return &Customer{TenantID: tenantID}, nil
}

func (s *noopBillingService) CreateCheckoutSession(ctx context.Context, tenantID string, plan Plan, successURL, cancelURL string) (*CheckoutSession, error) {
	return &CheckoutSession{
		TenantID: tenantID,
		Plan:     PlanFree,
	}, nil
}

func (s *noopBillingService) GetSubscription(ctx context.Context, tenantID string) (*Subscription, error) {
	return &Subscription{
		TenantID:      tenantID,
		Plan:          PlanFree,
		PaymentStatus: PaymentStatusUnpaid,
	}, nil
}

func (s *noopBillingService) CancelSubscription(ctx context.Context, tenantID string) (*Subscription, error) {
	return &Subscription{
		TenantID:      tenantID,
		Plan:          PlanFree,
		PaymentStatus: PaymentStatusUnpaid,
	}, nil
}

func (s *noopBillingService) ReportUsage(ctx context.Context, tenantID string, usage UsageReport) error {
	return nil
}

// CreateCustomer creates a Stripe customer for the tenant if one doesn't already exist, persists the Stripe customer ID to the billing record, and returns the customer details.
func (s *stripeBillingService) CreateCustomer(ctx context.Context, tenantID string) (*Customer, error) {
	rec, err := s.recordOrCreate(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if rec.StripeCustomerID != "" {
		return &Customer{TenantID: tenantID, StripeCustomerID: rec.StripeCustomerID}, nil
	}

	stripeCustomer, err := s.adapter.CreateCustomer(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("create stripe customer: %w", err)
	}

	rec.StripeCustomerID = stripeCustomer.ID
	if err := s.repo.UpdateStripeState(ctx, tenantID, rec.StripeCustomerID, rec.StripeSubscriptionID); err != nil {
		return nil, fmt.Errorf("persist stripe customer id: %w", err)
	}
	return &Customer{TenantID: tenantID, StripeCustomerID: stripeCustomer.ID}, nil
}

// CreateCheckoutSession initializes a Stripe checkout session to allow the tenant to subscribe to the specified plan. It requires an existing Stripe customer created via CreateCustomer, and persists the plan choice and incomplete payment status to the billing record.
func (s *stripeBillingService) CreateCheckoutSession(ctx context.Context, tenantID string, plan Plan, successURL, cancelURL string) (*CheckoutSession, error) {
	rec, err := s.recordOrCreate(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if rec.StripeCustomerID == "" {
		return nil, errors.New("missing stripe customer for tenant; call CreateCustomer first")
	}

	priceID, err := stripePriceForPlan(plan, s.cfg)
	if err != nil {
		return nil, err
	}

	session, err := s.adapter.CreateCheckoutSession(ctx, tenantID, rec.StripeCustomerID, priceID, successURL, cancelURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStripeSubscriptionFail, err)
	}
	if session.Subscription != "" {
		rec.StripeSubscriptionID = session.Subscription
	}
	rec.Plan = plan
	rec.PaymentStatus = PaymentStatusIncomplete
	if err := s.repo.Upsert(ctx, rec); err != nil {
		return nil, fmt.Errorf("persist checkout session state: %w", err)
	}

	if s.logger != nil {
		s.logger.Debug("created checkout session",
			"tenant_id", tenantID,
			"plan", plan,
			"session_id", session.ID,
		)
	}

	return &CheckoutSession{
		TenantID:             tenantID,
		ID:                   session.ID,
		URL:                  session.URL,
		StripeSubscriptionID: rec.StripeSubscriptionID,
		Plan:                 plan,
	}, nil
}

// GetSubscription returns the tenant's subscription and payment state. If no Stripe subscription exists, it returns the locally stored plan and status. Otherwise it fetches the current subscription details from Stripe, maps the remote status and plan, and updates the local record.
func (s *stripeBillingService) GetSubscription(ctx context.Context, tenantID string) (*Subscription, error) {
	rec, err := s.recordOrCreate(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if rec.StripeSubscriptionID == "" {
		plan := rec.Plan
		if plan == "" {
			plan = PlanFree
		}
		paymentStatus := rec.PaymentStatus
		if paymentStatus == "" {
			paymentStatus = PaymentStatusUnpaid
		}
		return &Subscription{
			TenantID:      tenantID,
			Plan:          plan,
			PaymentStatus: paymentStatus,
		}, nil
	}

	sub, err := s.adapter.GetSubscription(ctx, rec.StripeSubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("get stripe subscription: %w", err)
	}
	paymentStatus := mapPaymentStatus(sub.Status)
	plan, err := mapPlanFromPriceID(s.cfg, firstPriceID(sub))
	if err != nil {
		return nil, err
	}
	if err := s.persistPlanPayment(ctx, tenantID, plan, paymentStatus); err != nil {
		return nil, fmt.Errorf("persist subscription status: %w", err)
	}

	rec.Plan = plan
	rec.PaymentStatus = paymentStatus
	return &Subscription{
		TenantID:             tenantID,
		StripeCustomerID:     sub.Customer,
		StripeSubscriptionID: sub.ID,
		Plan:                 rec.Plan,
		PaymentStatus:        rec.PaymentStatus,
	}, nil
}

// CancelSubscription cancels the tenant's active Stripe subscription, reverts the plan to Free, and marks the payment status as Canceled. It returns an error if no subscription exists.
func (s *stripeBillingService) CancelSubscription(ctx context.Context, tenantID string) (*Subscription, error) {
	rec, err := s.recordOrCreate(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if rec.StripeSubscriptionID == "" {
		return nil, ErrSubscriptionNotFound
	}
	sub, err := s.adapter.CancelSubscription(ctx, rec.StripeSubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("cancel stripe subscription: %w", err)
	}

	if err := s.persistPlanPayment(ctx, tenantID, PlanFree, PaymentStatusCanceled); err != nil {
		return nil, fmt.Errorf("persist canceled plan: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("subscription canceled", "tenant_id", tenantID, "subscription_id", rec.StripeSubscriptionID)
	}

	return &Subscription{
		TenantID:             tenantID,
		StripeCustomerID:     rec.StripeCustomerID,
		StripeSubscriptionID: sub.ID,
		Plan:                 PlanFree,
		PaymentStatus:        PaymentStatusCanceled,
	}, nil
}

func (s *stripeBillingService) persistPlanPayment(ctx context.Context, tenantID string, plan Plan, status PaymentStatus) error {
	return s.repo.UpdatePlanAndPayment(ctx, tenantID, plan, status)
}

// recordOrCreate retrieves the billing record for the tenant, creating one if it doesn't exist. It handles concurrent creation races by retrying the get if a conflict occurs.
func (s *stripeBillingService) recordOrCreate(ctx context.Context, tenantID string) (*BillingRecord, error) {
	rec, err := s.repo.Get(ctx, tenantID)
	if err == nil {
		return rec, nil
	}
	if !errors.Is(err, ErrBillingRecordNotFound) {
		return nil, err
	}
	rec, createErr := s.repo.Create(ctx, tenantID)
	if createErr == nil {
		return rec, nil
	}
	if errors.Is(createErr, ErrBillingConflict) {
		return s.repo.Get(ctx, tenantID)
	}
	return nil, fmt.Errorf("create tenant billing record: %w", createErr)
}

// ReportUsage reports usage metrics to Stripe metering, including API requests, storage bytes, bandwidth bytes, and function invocations for the specified period. It only sends positive deltas and requires a Stripe customer. Checkpoints prevent duplicate reports of the same usage.
func (s *stripeBillingService) ReportUsage(ctx context.Context, tenantID string, usage UsageReport) error {
	rec, err := s.repo.Get(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("get billing record: %w", err)
	}
	if rec.StripeCustomerID == "" {
		s.logDebug("skipping usage report: no stripe customer id", "tenant_id", tenantID)
		return nil
	}

	usageDate := usage.PeriodEnd.Format("2006-01-02")
	metrics := []struct {
		name           string
		value          int64
		meterEventName string
	}{
		{"api_requests", usage.RequestCount, s.cfg.StripeMeterAPIRequests},
		{"storage_bytes", usage.DBBytesUsed, s.cfg.StripeMeterStorageBytes},
		{"bandwidth_bytes", usage.BandwidthBytes, s.cfg.StripeMeterBandwidthBytes},
		{"function_invocations", usage.FunctionInvocations, s.cfg.StripeMeterFunctionInvs},
	}

	for _, m := range metrics {
		lastReported, err := s.repo.GetUsageSyncCheckpoint(ctx, tenantID, usageDate, m.name)
		if err != nil {
			s.logError("failed to get usage checkpoint", "tenant_id", tenantID, "metric", m.name, "error", err)
			continue
		}

		delta := m.value - lastReported
		if delta <= 0 {
			s.logDebug("skipping usage report: no positive delta", "tenant_id", tenantID, "metric", m.name, "delta", delta)
			continue
		}

		identifier := fmt.Sprintf("ayb:%s:%s:%s:%d", tenantID, usageDate, m.name, m.value)
		if err := s.adapter.SendMeterEvent(ctx, m.meterEventName, rec.StripeCustomerID, delta, identifier); err != nil {
			s.logError("failed to send meter event", "tenant_id", tenantID, "metric", m.name, "error", err)
			continue
		}

		if err := s.repo.UpsertUsageSyncCheckpoint(ctx, tenantID, usageDate, m.name, m.value); err != nil {
			s.logError("failed to update usage checkpoint", "tenant_id", tenantID, "metric", m.name, "error", err)
		}
	}
	return nil
}

func (s *stripeBillingService) logDebug(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Debug(msg, args...)
	}
}

func (s *stripeBillingService) logError(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Error(msg, args...)
	}
}

func stripePriceForPlan(plan Plan, cfg config.BillingConfig) (string, error) {
	switch plan {
	case PlanStarter:
		return cfg.StripeStarterPriceID, nil
	case PlanPro:
		return cfg.StripeProPriceID, nil
	case PlanEnterprise:
		return cfg.StripeEnterprisePriceID, nil
	case PlanFree:
		// Free is a local/default state and does not map to a Stripe Checkout price.
		return "", ErrUnsupportedPlan
	default:
		return "", ErrUnsupportedPlan
	}
}

// mapPaymentStatus converts a Stripe subscription status string to a PaymentStatus constant, returning PaymentStatusIncomplete for unknown values.
func mapPaymentStatus(raw string) PaymentStatus {
	switch raw {
	case "active":
		return PaymentStatusActive
	case "past_due":
		return PaymentStatusPastDue
	case "canceled":
		return PaymentStatusCanceled
	case "trialing":
		return PaymentStatusTrialing
	case "incomplete":
		return PaymentStatusIncomplete
	case "unpaid":
		return PaymentStatusUnpaid
	default:
		return PaymentStatusIncomplete
	}
}

func mapPlanFromPriceID(cfg config.BillingConfig, priceID string) (Plan, error) {
	switch priceID {
	case cfg.StripeStarterPriceID:
		return PlanStarter, nil
	case cfg.StripeProPriceID:
		return PlanPro, nil
	case cfg.StripeEnterprisePriceID:
		return PlanEnterprise, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownStripePriceID, priceID)
	}
}

func firstPriceID(sub *stripeSubscriptionResponse) string {
	if sub == nil || len(sub.Items.Data) == 0 {
		return ""
	}
	return sub.Items.Data[0].Price.ID
}
