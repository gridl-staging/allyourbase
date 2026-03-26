package tenant

import (
	"encoding/json"
	"time"
)

// TenantState represents the lifecycle state of a tenant.
type TenantState string

const (
	TenantStateProvisioning TenantState = "provisioning"
	TenantStateActive       TenantState = "active"
	TenantStateSuspended    TenantState = "suspended"
	TenantStateDeleting     TenantState = "deleting"
	TenantStateDeleted      TenantState = "deleted"
)

// validTransitions maps each state to the set of valid target states.
var validTransitions = map[TenantState]map[TenantState]bool{
	TenantStateProvisioning: {TenantStateActive: true},
	TenantStateActive:       {TenantStateSuspended: true, TenantStateDeleting: true},
	TenantStateSuspended:    {TenantStateActive: true, TenantStateDeleting: true},
	TenantStateDeleting:     {TenantStateDeleted: true},
	TenantStateDeleted:      {},
}

// IsValidTransition reports whether transitioning from from to to is allowed
// by the tenant lifecycle state machine.
func IsValidTransition(from, to TenantState) bool {
	return validTransitions[from][to]
}

// NormalizeIsolationMode maps legacy and default values to the canonical persisted mode.
// Empty string (unset) and the transitional legacy value "database" are both treated as "shared".
func NormalizeIsolationMode(mode string) string {
	if mode == "" || mode == "database" {
		return "shared"
	}
	return mode
}

// Tenant represents a tenant organisation.
type Tenant struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	IsolationMode  string          `json:"isolationMode"`
	PlanTier       string          `json:"planTier"`
	Region         string          `json:"region"`
	OrgID          *string         `json:"orgId,omitempty"`
	OrgMetadata    json.RawMessage `json:"orgMetadata"`
	State          TenantState     `json:"state"`
	IdempotencyKey *string         `json:"idempotencyKey,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// TenantMembership represents a user's membership in a tenant with a role.
type TenantMembership struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenantId"`
	UserID    string    `json:"userId"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// TenantQuotas holds hard and soft resource limits for a tenant.
// Nil pointer fields mean unlimited (no constraint enforced).
type TenantQuotas struct {
	ID                      string    `json:"id"`
	TenantID                string    `json:"tenantId"`
	DBSizeBytesHard         *int64    `json:"dbSizeBytesHard"`
	DBSizeBytesSoft         *int64    `json:"dbSizeBytesSoft"`
	RequestRateRPSHard      *int      `json:"requestRateRpsHard"`
	RequestRateRPSSoft      *int      `json:"requestRateRpsSoft"`
	RealtimeConnectionsHard *int      `json:"realtimeConnectionsHard"`
	RealtimeConnectionsSoft *int      `json:"realtimeConnectionsSoft"`
	JobConcurrencyHard      *int      `json:"jobConcurrencyHard"`
	JobConcurrencySoft      *int      `json:"jobConcurrencySoft"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

// TenantUsageDaily holds daily usage metrics for a tenant, used for billing
// and quota enforcement.
type TenantUsageDaily struct {
	ID                      string    `json:"id"`
	TenantID                string    `json:"tenantId"`
	Date                    time.Time `json:"date"`
	RequestCount            int64     `json:"requestCount"`
	DBBytesUsed             int64     `json:"dbBytesUsed"`
	BandwidthBytes          int64     `json:"bandwidthBytes"`
	FunctionInvocations     int64     `json:"functionInvocations"`
	RealtimePeakConnections int       `json:"realtimePeakConnections"`
	JobRuns                 int       `json:"jobRuns"`
	CreatedAt               time.Time `json:"createdAt"`
}

// TenantAuditEvent is an immutable log entry for a tenant lifecycle action.
type TenantAuditEvent struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenantId"`
	ActorID   *string         `json:"actorId"`
	Action    string          `json:"action"`
	Result    string          `json:"result"`
	Metadata  json.RawMessage `json:"metadata"`
	IPAddress *string         `json:"ipAddress"`
	CreatedAt time.Time       `json:"createdAt"`
}

// TenantListResult is a paginated list of tenants.
type TenantListResult struct {
	Items      []Tenant `json:"items"`
	Page       int      `json:"page"`
	PerPage    int      `json:"perPage"`
	TotalItems int      `json:"totalItems"`
	TotalPages int      `json:"totalPages"`
}

// AuditQuery holds filter parameters for querying audit events.
type AuditQuery struct {
	TenantID string
	From     *time.Time
	To       *time.Time
	Action   string
	Result   string
	ActorID  string
	Limit    int
	Offset   int
}

// TenantMaintenanceState holds the maintenance mode state for a tenant.
type TenantMaintenanceState struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenantId"`
	Enabled   bool       `json:"enabled"`
	Reason    *string    `json:"reason,omitempty"`
	EnabledAt *time.Time `json:"enabledAt,omitempty"`
	EnabledBy *string    `json:"enabledBy,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// BreakerState represents the state of a tenant circuit breaker.
type BreakerState string

const (
	BreakerStateClosed   BreakerState = "closed"
	BreakerStateOpen     BreakerState = "open"
	BreakerStateHalfOpen BreakerState = "half_open"
)

// TenantCircuitBreakerState holds the circuit breaker state for a tenant.
type TenantCircuitBreakerState struct {
	ID                  string       `json:"id"`
	TenantID            string       `json:"tenantId"`
	State               BreakerState `json:"state"`
	ConsecutiveFailures int          `json:"consecutiveFailures"`
	OpenedAt            *time.Time   `json:"openedAt,omitempty"`
	HalfOpenProbes      int          `json:"halfOpenProbes"`
	LastFailureAt       *time.Time   `json:"lastFailureAt,omitempty"`
	CreatedAt           time.Time    `json:"createdAt"`
	UpdatedAt           time.Time    `json:"updatedAt"`
}
