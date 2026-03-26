package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// auditRequestInfo is stored on request-scoped contexts.
type auditRequestInfo struct{}
type auditPrincipalInfo struct{}

// Execer is the minimal interface for executing SQL statements. It is
// satisfied by *pgxpool.Pool, pgx.Tx, and the api.Querier interface, so
// callers can pass the active transaction to LogMutationWithQuerier to ensure
// the audit insert commits or rolls back together with the mutation.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// execer is a package-internal alias kept for backward compatibility.
type execer = Execer

// AuditEntry captures a single mutation event.
type AuditEntry struct {
	TableName string
	RecordID  any
	Operation string // INSERT, UPDATE, DELETE
	OldValues any
	NewValues any
}

// Sink is the interface the API handler uses for audit logging.
// It is satisfied by *AuditLogger. Callers may substitute a test double.
type Sink interface {
	ShouldAudit(tableName string) bool
	LogMutationWithQuerier(ctx context.Context, q Execer, entry AuditEntry) error
}

// AuditLogger writes mutation audit events to _ayb_audit_log.
type AuditLogger struct {
	cfg config.AuditConfig
	db  execer
}

// NewAuditLogger constructs a logger. If cfg.Enabled is false, LogMutation no-ops.
func NewAuditLogger(cfg config.AuditConfig, db execer) *AuditLogger {
	return &AuditLogger{cfg: cfg, db: db}
}

// ContextWithIP stores request IP in context for audit logging.
func ContextWithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, auditRequestInfo{}, strings.TrimSpace(ip))
}

// ContextWithPrincipal stores a trusted audit principal identifier in context.
// This is used for non-user/admin-token flows that do not carry auth.Claims.
func ContextWithPrincipal(ctx context.Context, principal string) context.Context {
	return context.WithValue(ctx, auditPrincipalInfo{}, strings.TrimSpace(principal))
}

// PrincipalFromContext returns the trusted audit principal identifier if present.
func PrincipalFromContext(ctx context.Context) string {
	v := ctx.Value(auditPrincipalInfo{})
	if raw, ok := v.(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
}

// ShouldAudit reports whether a given table name should be logged.
func (l *AuditLogger) ShouldAudit(tableName string) bool {
	if !l.cfg.Enabled {
		return false
	}
	if l.cfg.AllTables {
		return true
	}
	if len(l.cfg.Tables) == 0 {
		return false
	}
	for _, name := range l.cfg.Tables {
		if name == tableName {
			return true
		}
	}
	return false
}

// LogMutation inserts a row in _ayb_audit_log using the logger's own db connection.
// For transactional correctness the caller should prefer LogMutationWithQuerier,
// passing the active transaction so the audit insert commits or rolls back together
// with the mutation.
func (l *AuditLogger) LogMutation(ctx context.Context, entry AuditEntry) error {
	if l == nil || l.db == nil {
		return nil
	}
	return l.LogMutationWithQuerier(ctx, l.db, entry)
}

// LogMutationWithQuerier inserts an audit row using the provided execer, which
// should be the same transaction as the surrounding mutation.  The audit insert
// therefore commits or rolls back atomically with the data change.
func (l *AuditLogger) LogMutationWithQuerier(ctx context.Context, q execer, entry AuditEntry) error {
	if l == nil || !l.ShouldAudit(entry.TableName) || q == nil {
		return nil
	}

	operation := strings.ToUpper(strings.TrimSpace(entry.Operation))
	if !isValidOperation(operation) {
		return fmt.Errorf("invalid audit operation: %s", entry.Operation)
	}

	claims := auth.ClaimsFromContext(ctx)
	var userID, apiKeyID any
	if claims != nil {
		userID = parseUUID(claims.Subject)
		apiKeyID = parseUUID(claims.APIKeyID)
	}
	// Admin-token and other non-user flows can still provide a trusted principal
	// through audit context, even when auth claims are unavailable.
	if userID == nil && apiKeyID == nil {
		apiKeyID = parseUUID(PrincipalFromContext(ctx))
	}
	ip := parseIP(contextIP(ctx))

	recordID, err := marshalJSONB(entry.RecordID)
	if err != nil {
		return err
	}
	oldValues, err := marshalJSONB(entry.OldValues)
	if err != nil {
		return err
	}
	newValues, err := marshalJSONB(entry.NewValues)
	if err != nil {
		return err
	}

	_, err = q.Exec(ctx, `
		INSERT INTO _ayb_audit_log (user_id, api_key_id, table_name, record_id, operation, old_values, new_values, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		userID, apiKeyID, entry.TableName, recordID, operation, oldValues, newValues, ip,
	)
	if err != nil {
		return fmt.Errorf("insert audit log entry: %w", err)
	}
	return nil
}

func parseUUID(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return id
}

func parseIP(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return nil
	}
	return ip.String()
}

func contextIP(ctx context.Context) string {
	v := ctx.Value(auditRequestInfo{})
	if raw, ok := v.(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
}

func marshalJSONB(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch value := v.(type) {
	case []byte:
		return value, nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal jsonb value: %w", err)
		}
		return data, nil
	}
}

func isValidOperation(v string) bool {
	switch v {
	case "INSERT", "UPDATE", "DELETE":
		return true
	default:
		return false
	}
}
