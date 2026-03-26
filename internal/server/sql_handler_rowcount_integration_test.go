//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/server"
	"github.com/allyourbase/ayb/internal/testutil"
)

// TestAdminSQLRowCountForDML verifies that INSERT/UPDATE/DELETE statements
// return the correct rowCount from CommandTag.RowsAffected(), not
// len(resultRows) which is always 0 for DML without RETURNING.
func TestAdminSQLRowCountForDML(t *testing.T) {
	ctx := context.Background()
	createIntegrationTestSchema(t, ctx)

	// Create a scratch table for this test.
	_, err := sharedPG.Pool.Exec(ctx, `CREATE TABLE _test_rowcount (id serial PRIMARY KEY, val text)`)
	if err != nil {
		t.Fatalf("creating scratch table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = sharedPG.Pool.Exec(context.Background(), `DROP TABLE IF EXISTS _test_rowcount`)
	})

	logger := testutil.DiscardLogger()
	ch := schema.NewCacheHolder(sharedPG.Pool, logger)
	testutil.NoError(t, ch.Load(ctx))

	cfg := config.Default()
	cfg.Admin.Password = "testpass"
	srv := server.New(cfg, logger, ch, sharedPG.Pool, nil, nil)
	token := adminLogin(t, srv)

	// Helper to execute SQL via the admin endpoint and return the parsed response.
	execSQL := func(t *testing.T, query string) map[string]any {
		t.Helper()
		body := `{"query":` + jsonString(query) + `}`
		req := httptest.NewRequest(http.MethodPost, "/api/admin/sql/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)
		testutil.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	// INSERT 3 rows — rowCount should be 3, not 0.
	t.Run("INSERT returns affected row count", func(t *testing.T) {
		resp := execSQL(t, `INSERT INTO _test_rowcount (val) VALUES ('a'), ('b'), ('c')`)
		rowCount, ok := resp["rowCount"].(float64)
		testutil.True(t, ok, "expected rowCount in response")
		testutil.Equal(t, float64(3), rowCount)
	})

	// UPDATE 2 rows — rowCount should be 2.
	t.Run("UPDATE returns affected row count", func(t *testing.T) {
		resp := execSQL(t, `UPDATE _test_rowcount SET val = 'updated' WHERE val IN ('a', 'b')`)
		rowCount, ok := resp["rowCount"].(float64)
		testutil.True(t, ok, "expected rowCount in response")
		testutil.Equal(t, float64(2), rowCount)
	})

	// DELETE 1 row — rowCount should be 1.
	t.Run("DELETE returns affected row count", func(t *testing.T) {
		resp := execSQL(t, `DELETE FROM _test_rowcount WHERE val = 'c'`)
		rowCount, ok := resp["rowCount"].(float64)
		testutil.True(t, ok, "expected rowCount in response")
		testutil.Equal(t, float64(1), rowCount)
	})

	// SELECT — rowCount should still equal number of returned rows.
	t.Run("SELECT returns result row count", func(t *testing.T) {
		resp := execSQL(t, `SELECT * FROM _test_rowcount`)
		rowCount, ok := resp["rowCount"].(float64)
		testutil.True(t, ok, "expected rowCount in response")
		rows, ok := resp["rows"].([]any)
		testutil.True(t, ok, "expected rows array in response")
		testutil.Equal(t, float64(len(rows)), rowCount)
	})
}

// jsonString returns a JSON-encoded string value.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
