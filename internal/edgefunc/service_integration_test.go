//go:build integration

package edgefunc_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/allyourbase/ayb/internal/edgefunc"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestServiceInvoke_WithPostgresQueryExecutor(t *testing.T) {
	ctx := context.Background()
	table := createQueryExecutorTestTable(t)

	_, err := testPool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (name, age) VALUES ('alice', 30)`, sqlutil.QuoteIdent(table)))
	testutil.NoError(t, err)

	store := edgefunc.NewPostgresStore(testPool)
	logStore := edgefunc.NewPostgresLogStore(testPool)
	pool := edgefunc.NewPool(2)
	defer pool.Close()

	qe := edgefunc.NewPostgresQueryExecutor(testPool, []string{table})
	svc := edgefunc.NewService(store, pool, logStore, edgefunc.WithServiceQueryExecutor(qe))

	fnName := "test-service-db-" + table[len(table)-6:]
	deployed, err := svc.Deploy(ctx, fnName, fmt.Sprintf(`function handler(req) { const rows = ayb.db.from("%s").select("name").eq("age", 30).execute(); return { statusCode: 200, body: rows[0].name }; }`, table), edgefunc.DeployOptions{})
	testutil.NoError(t, err)
	t.Cleanup(func() {
		_ = store.Delete(context.Background(), deployed.ID)
	})

	resp, err := svc.Invoke(ctx, fnName, edgefunc.Request{Method: "GET", Path: "/test"})
	testutil.NoError(t, err)
	testutil.Equal(t, 200, resp.StatusCode)
	testutil.Equal(t, "alice", string(resp.Body))

	logs, err := logStore.ListByFunction(ctx, deployed.ID, edgefunc.LogListOptions{Page: 1, PerPage: 10})
	testutil.NoError(t, err)
	testutil.SliceLen(t, logs, 1)
	testutil.Equal(t, "success", logs[0].Status)
}
