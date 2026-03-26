//go:build integration

package edgefunc_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestDBTriggerStoreCreate_DoesNotInstallPGTrigger(t *testing.T) {
	ctx := context.Background()
	functionID := createDBTriggerTestFunction(t)
	table := createDBTriggerTargetTable(t)

	store := edgefunc.NewDBTriggerPostgresStore(testPool)
	trigger, err := store.CreateDBTrigger(ctx, &edgefunc.DBTrigger{
		FunctionID: functionID,
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
		Enabled:    true,
	})
	testutil.NoError(t, err)

	_, exists := lookupPGTriggerState(t, trigger.ID, "public", table)
	testutil.False(t, exists, "expected metadata-only store create to leave PG trigger absent")
}

func TestDBTriggerServiceCreate_InstallsPGTrigger(t *testing.T) {
	ctx := context.Background()
	functionID := createDBTriggerTestFunction(t)
	table := createDBTriggerTargetTable(t)

	store := edgefunc.NewDBTriggerPostgresStore(testPool)
	svc := edgefunc.NewDBTriggerService(store)

	trigger, err := svc.Create(ctx, edgefunc.CreateDBTriggerInput{
		FunctionID: functionID,
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)

	enabled, exists := lookupPGTriggerState(t, trigger.ID, "public", table)
	testutil.True(t, exists, "expected service create to install PG trigger")
	testutil.Equal(t, "O", enabled) // O = enabled
}

func TestDBTriggerServiceSetEnabled_UpdatesPGTriggerState(t *testing.T) {
	ctx := context.Background()
	functionID := createDBTriggerTestFunction(t)
	table := createDBTriggerTargetTable(t)

	store := edgefunc.NewDBTriggerPostgresStore(testPool)
	svc := edgefunc.NewDBTriggerService(store)

	trigger, err := svc.Create(ctx, edgefunc.CreateDBTriggerInput{
		FunctionID: functionID,
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert},
	})
	testutil.NoError(t, err)

	enabled, exists := lookupPGTriggerState(t, trigger.ID, "public", table)
	testutil.True(t, exists, "expected PG trigger to exist after create")
	testutil.Equal(t, "O", enabled)

	_, err = svc.SetEnabled(ctx, trigger.ID, false)
	testutil.NoError(t, err)

	enabled, exists = lookupPGTriggerState(t, trigger.ID, "public", table)
	testutil.True(t, exists, "expected PG trigger to exist after disable")
	testutil.Equal(t, "D", enabled) // D = disabled

	_, err = svc.SetEnabled(ctx, trigger.ID, true)
	testutil.NoError(t, err)

	enabled, exists = lookupPGTriggerState(t, trigger.ID, "public", table)
	testutil.True(t, exists, "expected PG trigger to exist after re-enable")
	testutil.Equal(t, "O", enabled)
}

func TestDBTriggerServiceDelete_RemovesPGTrigger(t *testing.T) {
	ctx := context.Background()
	functionID := createDBTriggerTestFunction(t)
	table := createDBTriggerTargetTable(t)

	store := edgefunc.NewDBTriggerPostgresStore(testPool)
	svc := edgefunc.NewDBTriggerService(store)

	trigger, err := svc.Create(ctx, edgefunc.CreateDBTriggerInput{
		FunctionID: functionID,
		TableName:  table,
		Schema:     "public",
		Events:     []edgefunc.DBTriggerEvent{edgefunc.DBEventInsert, edgefunc.DBEventUpdate},
	})
	testutil.NoError(t, err)

	_, exists := lookupPGTriggerState(t, trigger.ID, "public", table)
	testutil.True(t, exists, "expected PG trigger to exist before delete")

	err = svc.Delete(ctx, trigger.ID)
	testutil.NoError(t, err)

	_, exists = lookupPGTriggerState(t, trigger.ID, "public", table)
	testutil.False(t, exists, "expected PG trigger to be removed on delete")
}

func createDBTriggerTestFunction(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	name := "test-db-trigger-fn-" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	store := edgefunc.NewPostgresStore(testPool)
	fn, err := store.Create(ctx, &edgefunc.EdgeFunction{
		Name:       name,
		EntryPoint: "handler",
		Source:     "function handler() { return { status: 200 }; }",
		CompiledJS: "function handler() { return { status: 200 }; }",
	})
	testutil.NoError(t, err)

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM _ayb_edge_functions WHERE id = $1`, fn.ID)
	})
	return fn.ID.String()
}

func createDBTriggerTargetTable(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	table := "test_db_trigger_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	_, err := testPool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE %s (id BIGSERIAL PRIMARY KEY, value TEXT)`,
		sqlutil.QuoteIdent(table),
	))
	testutil.NoError(t, err)

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), fmt.Sprintf(`DROP TABLE IF EXISTS %s`, sqlutil.QuoteIdent(table)))
	})
	return table
}

func lookupPGTriggerState(t *testing.T, triggerID, schemaName, tableName string) (string, bool) {
	t.Helper()

	ctx := context.Background()
	triggerName := edgefunc.TriggerName(triggerID)

	var enabled string
	err := testPool.QueryRow(ctx, `
		SELECT t.tgenabled::text
		FROM pg_trigger t
		JOIN pg_class c ON c.oid = t.tgrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE t.tgname = $1
		  AND n.nspname = $2
		  AND c.relname = $3
		  AND t.tgisinternal = false
	`, triggerName, schemaName, tableName).Scan(&enabled)
	if errorsIsNoRows(err) {
		return "", false
	}
	testutil.NoError(t, err)
	return enabled, true
}

func errorsIsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
