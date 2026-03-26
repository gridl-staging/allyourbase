package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/jackc/pgx/v5/pgxpool"
)

// orgAuditQuerier queries tenant audit events filtered by org_id.
type orgAuditQuerier interface {
	QueryOrgAuditEvents(ctx context.Context, query orgAuditQuery) ([]tenant.TenantAuditEvent, error)
}

type orgAuditQuery struct {
	OrgID   string
	From    *time.Time
	To      *time.Time
	Action  string
	Result  string
	ActorID string
	Limit   int
	Offset  int
}

// dbOrgAuditQuerier implements orgAuditQuerier using the database pool.
type dbOrgAuditQuerier struct {
	pool *pgxpool.Pool
}

// handleAdminOrgAudit returns audit events for all tenants in an org.
func handleAdminOrgAudit(store tenant.OrgStore, querier orgAuditQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if querier == nil {
			httputil.WriteError(w, http.StatusInternalServerError, "audit service not configured")
			return
		}

		org, ok := lookupOrg(r, w, store)
		if !ok {
			return
		}

		filters, err := parseOrgAuditFilters(r, org.ID)
		if err != nil {
			if parseErr, ok := err.(*auditParseError); ok {
				httputil.WriteError(w, parseErr.code, parseErr.message)
			} else {
				httputil.WriteError(w, http.StatusBadRequest, err.Error())
			}
			return
		}

		events, err := querier.QueryOrgAuditEvents(r.Context(), filters)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to query audit events")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, tenantAuditListResult{
			Items:  events,
			Count:  len(events),
			Limit:  filters.Limit,
			Offset: filters.Offset,
		})
	}
}

// parseOrgAuditFilters extracts audit query params for org-scoped queries.
func parseOrgAuditFilters(r *http.Request, orgID string) (orgAuditQuery, error) {
	filters, err := parseSharedAuditFilters(r)
	if err != nil {
		return orgAuditQuery{}, err
	}
	return orgAuditQuery{
		OrgID:   orgID,
		From:    filters.from,
		To:      filters.to,
		Action:  filters.action,
		Result:  filters.result,
		ActorID: filters.actorID,
		Limit:   filters.limit,
		Offset:  filters.offset,
	}, nil
}

// QueryOrgAuditEvents returns tenant audit events for a given org.
func (q *dbOrgAuditQuerier) QueryOrgAuditEvents(ctx context.Context, query orgAuditQuery) ([]tenant.TenantAuditEvent, error) {
	sql := `SELECT id, tenant_id, actor_id, action, result, metadata, host(ip_address), created_at
		FROM _ayb_tenant_audit_events
		WHERE org_id = $1`
	args := []any{query.OrgID}
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

	rows, err := q.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("querying org audit events: %w", err)
	}
	defer rows.Close()

	var items []tenant.TenantAuditEvent
	for rows.Next() {
		var e tenant.TenantAuditEvent
		var metaBytes []byte
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ActorID, &e.Action, &e.Result,
			&metaBytes, &e.IPAddress, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning org audit event: %w", err)
		}
		if metaBytes != nil {
			e.Metadata = metaBytes
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating org audit events: %w", err)
	}
	if items == nil {
		items = []tenant.TenantAuditEvent{}
	}
	return items, nil
}
