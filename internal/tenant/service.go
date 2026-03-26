// Package tenant Service provides tenant lifecycle operations, maintenance state management, and usage tracking for multi-tenant systems.
package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors returned by the tenant service.
var (
	ErrTenantNotFound         = errors.New("tenant not found")
	ErrTenantNotInOrg         = errors.New("tenant not found in org")
	ErrTenantSlugTaken        = errors.New("tenant slug is already taken")
	ErrInvalidStateTransition = errors.New("invalid state transition")
	ErrMembershipNotFound     = errors.New("membership not found")
	ErrMembershipExists       = errors.New("membership already exists")
	ErrTenantNameRequired     = errors.New("tenant name is required")
	ErrInvalidRole            = errors.New("invalid role")
	ErrIdempotencyConflict    = errors.New("idempotency key conflict")
	ErrTenantUnderMaintenance = errors.New("tenant under maintenance")
	ErrTenantBreakerOpen      = errors.New("tenant circuit breaker open")
)

// MembershipRole constants
const (
	MemberRoleOwner  = "owner"
	MemberRoleAdmin  = "admin"
	MemberRoleMember = "member"
	MemberRoleViewer = "viewer"
)

// Service handles tenant CRUD and lifecycle operations.
type Service struct {
	pool              *pgxpool.Pool
	logger            *slog.Logger
	schemaProvisioner *SchemaProvisioner
}

// NewService creates a new tenant Service.
func NewService(pool *pgxpool.Pool, logger *slog.Logger) *Service {
	return &Service{
		pool:              pool,
		logger:            logger,
		schemaProvisioner: NewSchemaProvisioner(pool, logger),
	}
}

const tenantColumns = `id, name, slug, isolation_mode, plan_tier, region, org_id, org_metadata, state, idempotency_key, created_at, updated_at`

// scanTenant scans a single tenant row. Works with pgx.Row (from QueryRow)
// and pgx.Rows (from Query, after calling rows.Next()).
func scanTenant(row pgx.Row) (*Tenant, error) {
	var t Tenant
	var orgMeta []byte
	var state string
	err := row.Scan(
		&t.ID, &t.Name, &t.Slug, &t.IsolationMode, &t.PlanTier, &t.Region,
		&t.OrgID, &orgMeta, &state, &t.IdempotencyKey, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.OrgMetadata = json.RawMessage(orgMeta)
	t.State = TenantState(state)
	return &t, nil
}

// CreateTenant creates a new tenant record. Returns ErrTenantNameRequired if
// name is empty, or ErrTenantSlugTaken if the slug conflicts with an existing tenant.
// If idempotencyKey is non-empty and a tenant with that key already exists, the
// existing tenant is returned instead of creating a duplicate.
func (s *Service) CreateTenant(ctx context.Context, name, slug, isolationMode, planTier, region string, orgMetadata json.RawMessage, idempotencyKey string) (*Tenant, error) {
	if name == "" {
		return nil, ErrTenantNameRequired
	}
	isolationMode = NormalizeIsolationMode(isolationMode)
	if orgMetadata == nil {
		orgMetadata = json.RawMessage("{}")
	}

	var idemKey *string
	if idempotencyKey != "" {
		idemKey = &idempotencyKey
	}

	t, err := scanTenant(s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_tenants (name, slug, isolation_mode, plan_tier, region, org_metadata, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+tenantColumns,
		name, slug, isolationMode, planTier, region, []byte(orgMetadata), idemKey,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// If the conflict is on idempotency_key, return the existing tenant.
			if idempotencyKey != "" && pgErr.ConstraintName == "idx_ayb_tenants_idempotency_key" {
				return s.getTenantByIdempotencyKey(ctx, idempotencyKey)
			}
			return nil, ErrTenantSlugTaken
		}
		return nil, fmt.Errorf("creating tenant: %w", err)
	}

	if t.IsolationMode == "schema" {
		if err := s.schemaProvisioner.ProvisionSchema(ctx, t.Slug); err != nil {
			if s.logger != nil {
				s.logger.Error("tenant schema provisioning failed", "tenant_id", t.ID, "slug", t.Slug, "error", err)
			}
			return t, fmt.Errorf("provisioning tenant schema: %w", err)
		}
	}

	s.logger.Info("tenant created", "tenant_id", t.ID, "slug", slug)
	return t, nil
}

// DeleteTenantSchema drops the schema for a schema-isolated tenant.
func (s *Service) DeleteTenantSchema(ctx context.Context, slug string) error {
	if s.schemaProvisioner == nil {
		return nil
	}
	if err := s.schemaProvisioner.DropSchema(ctx, slug); err != nil {
		return fmt.Errorf("deleting tenant schema: %w", err)
	}
	return nil
}

// getTenantByIdempotencyKey retrieves a tenant by its idempotency key.
func (s *Service) getTenantByIdempotencyKey(ctx context.Context, key string) (*Tenant, error) {
	t, err := scanTenant(s.pool.QueryRow(ctx,
		`SELECT `+tenantColumns+` FROM _ayb_tenants WHERE idempotency_key = $1`,
		key,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("getting tenant by idempotency key: %w", err)
	}
	return t, nil
}

// GetTenant retrieves a tenant by ID. Returns ErrTenantNotFound if not found.
func (s *Service) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	t, err := scanTenant(s.pool.QueryRow(ctx,
		`SELECT `+tenantColumns+` FROM _ayb_tenants WHERE id = $1`,
		id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("getting tenant: %w", err)
	}
	return t, nil
}

// GetTenantBySlug retrieves a tenant by slug. Returns ErrTenantNotFound if not found.
func (s *Service) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	t, err := scanTenant(s.pool.QueryRow(ctx,
		`SELECT `+tenantColumns+` FROM _ayb_tenants WHERE slug = $1`,
		slug,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("getting tenant by slug: %w", err)
	}
	return t, nil
}

// TransitionState atomically moves a tenant from fromState to newState.
//
// The transition is validated against the lifecycle state machine before the
// UPDATE is issued. The UPDATE uses an optimistic WHERE state = fromState
// clause to prevent TOCTOU races: if another request changed the state
// concurrently, RowsAffected == 0 and a follow-up SELECT disambiguates
// ErrTenantNotFound from ErrInvalidStateTransition.
func (s *Service) TransitionState(ctx context.Context, id string, fromState, newState TenantState) (*Tenant, error) {
	if !IsValidTransition(fromState, newState) {
		return nil, fmt.Errorf("%w: %s -> %s", ErrInvalidStateTransition, fromState, newState)
	}

	t, err := scanTenant(s.pool.QueryRow(ctx,
		`UPDATE _ayb_tenants
		 SET state = $3, updated_at = NOW()
		 WHERE id = $1 AND state = $2
		 RETURNING `+tenantColumns,
		id, string(fromState), string(newState),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Disambiguate: missing tenant vs. concurrent state change.
			_, lookupErr := s.GetTenant(ctx, id)
			if errors.Is(lookupErr, ErrTenantNotFound) {
				return nil, ErrTenantNotFound
			}
			return nil, fmt.Errorf("%w: state was changed by another request", ErrInvalidStateTransition)
		}
		return nil, fmt.Errorf("transitioning tenant state: %w", err)
	}

	s.logger.Info("tenant state transitioned", "tenant_id", id, "from", fromState, "to", newState)
	return t, nil
}

// ListTenants returns a paginated list of tenants ordered by creation time.
func (s *Service) ListTenants(ctx context.Context, page, perPage int) (*TenantListResult, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	var totalItems int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_tenants`).Scan(&totalItems); err != nil {
		return nil, fmt.Errorf("counting tenants: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+tenantColumns+` FROM _ayb_tenants ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing tenants: %w", err)
	}
	defer rows.Close()

	items := []Tenant{}
	for rows.Next() {
		t, scanErr := scanTenant(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scanning tenant: %w", scanErr)
		}
		items = append(items, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tenants: %w", err)
	}

	totalPages := totalItems / perPage
	if totalItems%perPage != 0 {
		totalPages++
	}

	return &TenantListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}, nil
}

// InsertAuditEvent appends an immutable event to _ayb_tenant_audit_events.
// No update or delete methods are provided; the table is append-only.
func (s *Service) InsertAuditEvent(ctx context.Context, tenantID string, actorID *string, action, result string, metadata json.RawMessage, ipAddress *string) error {
	if metadata == nil {
		metadata = json.RawMessage("{}")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_tenant_audit_events (tenant_id, actor_id, action, result, metadata, ip_address)
		 VALUES ($1, $2::uuid, $3, $4, $5, $6::inet)`,
		tenantID, actorID, action, result, []byte(metadata), ipAddress,
	)
	if err != nil {
		return fmt.Errorf("inserting tenant audit event: %w", err)
	}
	return nil
}

// TenantOrgID returns the current org_id for a tenant.
func (s *Service) TenantOrgID(ctx context.Context, tenantID string) (*string, error) {
	t, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return t.OrgID, nil
}

// InsertAuditEventWithOrg appends an audit event with an optional org_id column.
func (s *Service) InsertAuditEventWithOrg(ctx context.Context, tenantID string, actorID *string, action, result string, metadata json.RawMessage, ipAddress *string, orgID *string) error {
	if metadata == nil {
		metadata = json.RawMessage("{}")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_tenant_audit_events (tenant_id, actor_id, action, result, metadata, ip_address, org_id)
		 VALUES ($1, $2::uuid, $3, $4, $5, $6::inet, $7)`,
		tenantID, actorID, action, result, []byte(metadata), ipAddress, orgID,
	)
	if err != nil {
		return fmt.Errorf("inserting tenant audit event with org: %w", err)
	}
	return nil
}

// QueryAuditEvents retrieves audit events for a tenant with optional filters.
// Results are ordered by created_at DESC, id DESC (newest first with stable tie-breaker).
func (s *Service) QueryAuditEvents(ctx context.Context, query AuditQuery) ([]TenantAuditEvent, error) {
	sql := `SELECT id, tenant_id, actor_id, action, result, metadata, host(ip_address), created_at
		FROM _ayb_tenant_audit_events
		WHERE tenant_id = $1`
	args := []any{query.TenantID}
	argNum := 2

	if query.From != nil {
		sql += fmt.Sprintf(" AND created_at >= $%d", argNum)
		args = append(args, *query.From)
		argNum++
	}
	if query.To != nil {
		sql += fmt.Sprintf(" AND created_at <= $%d", argNum)
		args = append(args, *query.To)
		argNum++
	}
	if query.Action != "" {
		sql += fmt.Sprintf(" AND action = $%d", argNum)
		args = append(args, query.Action)
		argNum++
	}
	if query.Result != "" {
		sql += fmt.Sprintf(" AND result = $%d", argNum)
		args = append(args, query.Result)
		argNum++
	}
	if query.ActorID != "" {
		sql += fmt.Sprintf(" AND actor_id = $%d::uuid", argNum)
		args = append(args, query.ActorID)
		argNum++
	}

	sql += " ORDER BY created_at DESC, id DESC"

	if query.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, query.Limit)
		argNum++
	}
	if query.Offset > 0 {
		sql += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, query.Offset)
	}

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("querying audit events: %w", err)
	}
	defer rows.Close()

	return scanAuditEvents(rows)
}

// scanAuditEvents unmarshals multiple tenant audit event rows from a pgx.Rows query result into TenantAuditEvent structs, converting metadata bytes to JSON, and returns an empty slice if no rows are present.
func scanAuditEvents(rows pgx.Rows) ([]TenantAuditEvent, error) {
	var items []TenantAuditEvent
	for rows.Next() {
		var e TenantAuditEvent
		var metaBytes []byte
		err := rows.Scan(
			&e.ID, &e.TenantID, &e.ActorID, &e.Action, &e.Result, &metaBytes, &e.IPAddress, &e.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		e.Metadata = json.RawMessage(metaBytes)
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if items == nil {
		items = []TenantAuditEvent{}
	}
	return items, nil
}

// IsValidRole checks if the given role is valid.
func IsValidRole(role string) bool {
	switch role {
	case MemberRoleOwner, MemberRoleAdmin, MemberRoleMember, MemberRoleViewer:
		return true
	default:
		return false
	}
}

// UpdateTenant updates tenant fields (name, orgMetadata).
// Returns ErrTenantNotFound if the tenant doesn't exist.
func (s *Service) UpdateTenant(ctx context.Context, id string, name string, orgMetadata json.RawMessage) (*Tenant, error) {
	t, err := scanTenant(s.pool.QueryRow(ctx,
		`UPDATE _ayb_tenants
		 SET name = COALESCE(NULLIF($2, ''), name), org_metadata = COALESCE($3::jsonb, org_metadata), updated_at = NOW()
		 WHERE id = $1
		 RETURNING `+tenantColumns,
		id, name, []byte(orgMetadata),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("updating tenant: %w", err)
	}

	s.logger.Info("tenant updated", "tenant_id", id)
	return t, nil
}
