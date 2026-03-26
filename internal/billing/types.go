package billing

import (
	"context"
	"time"
)

// Plan is the tenant entitlement tier persisted in _ayb_billing.
type Plan string

const (
	PlanFree       Plan = "free"
	PlanStarter    Plan = "starter"
	PlanPro        Plan = "pro"
	PlanEnterprise Plan = "enterprise"
)

// PaymentStatus reflects normalized subscription/payment lifecycle state.
type PaymentStatus string

const (
	PaymentStatusUnpaid     PaymentStatus = "unpaid"
	PaymentStatusActive     PaymentStatus = "active"
	PaymentStatusPastDue    PaymentStatus = "past_due"
	PaymentStatusCanceled   PaymentStatus = "canceled"
	PaymentStatusTrialing   PaymentStatus = "trialing"
	PaymentStatusIncomplete PaymentStatus = "incomplete"
)

// Customer represents a tenant Stripe customer.
type Customer struct {
	TenantID         string
	StripeCustomerID string
}

// CheckoutSession represents a Stripe checkout session response returned to callers.
type CheckoutSession struct {
	TenantID             string
	ID                   string
	URL                  string
	StripeSubscriptionID string
	Plan                 Plan
}

// Subscription is a normalized billing subscription snapshot.
type Subscription struct {
	TenantID             string
	StripeCustomerID     string
	StripeSubscriptionID string
	Plan                 Plan
	PaymentStatus        PaymentStatus
}

// UsageReport carries tenant usage payloads intended for future sync flows.
// It is intentionally small here and expanded in Stage 3.
type UsageReport struct {
	TenantID            string
	RequestCount        int64
	DBBytesUsed         int64
	BandwidthBytes      int64
	FunctionInvocations int64
	RealtimePeakConns   int64
	JobRuns             int64
	PeriodStart         time.Time
	PeriodEnd           time.Time
}

// BillingRecord represents one row in _ayb_billing.
type BillingRecord struct {
	TenantID             string
	StripeCustomerID     string
	StripeSubscriptionID string
	Plan                 Plan
	PaymentStatus        PaymentStatus
	TrialStartAt         *time.Time
	TrialEndAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// PlanLimits contains monthly quota limits for a billing plan.
// Zero values represent custom/unlimited limits.
type PlanLimits struct {
	APIRequests         int64 `json:"apiRequests"`
	StorageBytesUsed    int64 `json:"storageBytesUsed"`
	BandwidthBytes      int64 `json:"bandwidthBytes"`
	FunctionInvocations int64 `json:"functionInvocations"`
}

// LimitsForPlan returns the monthly quota limits for a billing plan.
func LimitsForPlan(plan Plan) PlanLimits {
	free := PlanLimits{
		APIRequests:         50_000,
		StorageBytesUsed:    1_073_741_824, // 1 GB
		BandwidthBytes:      5_368_709_120, // 5 GB
		FunctionInvocations: 100_000,
	}

	switch plan {
	case PlanFree:
		return free
	case PlanStarter:
		return PlanLimits{
			APIRequests:         250_000,
			StorageBytesUsed:    5_368_709_120,  // 5 GB
			BandwidthBytes:      26_843_545_600, // 25 GB
			FunctionInvocations: 500_000,
		}
	case PlanPro:
		return PlanLimits{
			APIRequests:         1_000_000,
			StorageBytesUsed:    10_737_418_240, // 10 GB
			BandwidthBytes:      53_687_091_200, // 50 GB
			FunctionInvocations: 2_000_000,
		}
	case PlanEnterprise:
		return PlanLimits{}
	default:
		return free
	}
}

// BillingService implements lifecycle operations required for Stage 2-3.
type BillingService interface {
	CreateCustomer(ctx context.Context, tenantID string) (*Customer, error)
	CreateCheckoutSession(ctx context.Context, tenantID string, plan Plan, successURL, cancelURL string) (*CheckoutSession, error)
	GetSubscription(ctx context.Context, tenantID string) (*Subscription, error)
	CancelSubscription(ctx context.Context, tenantID string) (*Subscription, error)
	ReportUsage(ctx context.Context, tenantID string, usage UsageReport) error
}

// BillingRepository persists billing lifecycle state.
type BillingRepository interface {
	Create(ctx context.Context, tenantID string) (*BillingRecord, error)
	Get(ctx context.Context, tenantID string) (*BillingRecord, error)
	GetBySubscriptionID(ctx context.Context, subscriptionID string) (*BillingRecord, error)
	Upsert(ctx context.Context, rec *BillingRecord) error
	UpdatePlanAndPayment(ctx context.Context, tenantID string, plan Plan, status PaymentStatus) error
	UpdateStripeState(ctx context.Context, tenantID, customerID, subscriptionID string) error
	HasProcessedEvent(ctx context.Context, eventID string) (bool, error)
	RecordWebhookEvent(ctx context.Context, eventID, eventType string, payload []byte) error
	GetUsageSyncCheckpoint(ctx context.Context, tenantID string, usageDate string, metric string) (int64, error)
	UpsertUsageSyncCheckpoint(ctx context.Context, tenantID string, usageDate string, metric string, lastReportedValue int64) error
}
