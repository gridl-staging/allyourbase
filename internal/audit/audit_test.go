//go:build integration

package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func setupAuditTestDB(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	testutil.NoError(t, err)

	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)
}

func TestAuditLoggerInsertsMutationValues(t *testing.T) {
	setupAuditTestDB(t)

	ctx := context.Background()
	cfg := config.Default().Audit
	cfg.Enabled = true
	cfg.AllTables = true

	logger := NewAuditLogger(cfg, sharedPG.Pool)

	userID := uuid.NewString()
	apiKeyID := uuid.NewString()
	entry := AuditEntry{
		TableName: "users",
		Operation: "INSERT",
		RecordID:  map[string]any{"id": "1", "tenant_id": "tenant-1"},
		NewValues: map[string]any{"id": "1", "email": "alice@example.com"},
	}
	claims := &auth.Claims{APIKeyID: apiKeyID}
	claims.Subject = userID
	ctx = auth.ContextWithClaims(ctx, claims)
	ctx = ContextWithIP(ctx, "203.0.113.10")

	err := logger.LogMutation(ctx, entry)
	testutil.NoError(t, err)

	var loggedUserID, loggedAPIKeyID *string
	var gotTableName, gotOperation string
	var gotRecordID, gotOldValues, gotNewValues []byte
	var gotIP *string
	err = sharedPG.Pool.QueryRow(ctx, `
		SELECT user_id::text, api_key_id::text, table_name, operation,
			record_id::text, old_values::text, new_values::text, host(ip_address)
		FROM _ayb_audit_log
		WHERE table_name = $1 AND operation = $2
		ORDER BY timestamp DESC
		LIMIT 1`, "users", "INSERT").Scan(
		&loggedUserID, &loggedAPIKeyID, &gotTableName, &gotOperation, &gotRecordID, &gotOldValues, &gotNewValues, &gotIP,
	)
	testutil.NoError(t, err)

	testutil.NotNil(t, loggedUserID)
	testutil.NotNil(t, loggedAPIKeyID)
	testutil.Equal(t, userID, *loggedUserID)
	testutil.Equal(t, apiKeyID, *loggedAPIKeyID)
	testutil.Equal(t, "users", gotTableName)
	testutil.Equal(t, "INSERT", gotOperation)
	testutil.NotNil(t, gotIP)
	testutil.Equal(t, "203.0.113.10", *gotIP)

	var rec map[string]any
	testutil.NoError(t, json.Unmarshal(gotRecordID, &rec))
	testutil.Equal(t, "1", rec["id"])

	testutil.Equal(t, true, gotOldValues == nil)

	var newVals map[string]any
	testutil.NoError(t, json.Unmarshal(gotNewValues, &newVals))
	testutil.Equal(t, "alice@example.com", newVals["email"])
}

func TestAuditLoggerSkipsNonAuditedTables(t *testing.T) {
	setupAuditTestDB(t)

	ctx := context.Background()
	cfg := config.Default().Audit
	cfg.Enabled = true
	cfg.AllTables = false
	cfg.Tables = []string{"audited"}

	logger := NewAuditLogger(cfg, sharedPG.Pool)
	ctx = ContextWithIP(ctx, "198.51.100.1")

	err := logger.LogMutation(ctx, AuditEntry{
		TableName: "not_audited",
		Operation: "INSERT",
		RecordID:  map[string]any{"id": "1"},
		NewValues: map[string]any{"id": "1"},
	})
	testutil.NoError(t, err)

	var notAuditedCount int
	err = sharedPG.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM _ayb_audit_log WHERE table_name = $1`, "not_audited").Scan(&notAuditedCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, notAuditedCount)

	err = logger.LogMutation(ctx, AuditEntry{
		TableName: "audited",
		Operation: "INSERT",
		RecordID:  map[string]any{"id": "2"},
		NewValues: map[string]any{"id": "2"},
	})
	testutil.NoError(t, err)

	var auditedCount int
	err = sharedPG.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM _ayb_audit_log WHERE table_name = $1`, "audited").Scan(&auditedCount)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, auditedCount)
}

func TestAuditLoggerAllTablesCapturesEverything(t *testing.T) {
	setupAuditTestDB(t)

	ctx := context.Background()
	cfg := config.Default().Audit
	cfg.Enabled = true
	cfg.AllTables = true

	logger := NewAuditLogger(cfg, sharedPG.Pool)

	for i := range 3 {
		err := logger.LogMutation(ctx, AuditEntry{
			TableName: fmt.Sprintf("t%d", i),
			Operation: "INSERT",
			RecordID:  map[string]any{"id": i},
			NewValues: map[string]any{"id": i},
			OldValues: nil,
		})
		testutil.NoError(t, err)
	}

	var count int
	err := sharedPG.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_audit_log`).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, count)
}
