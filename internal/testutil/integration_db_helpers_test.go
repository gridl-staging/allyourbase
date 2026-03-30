//go:build integration

package testutil

import (
	"context"
	"strings"
	"testing"
)

func TestIntegration_GetTestPoolReturnsUsablePool(t *testing.T) {
	pool := GetTestPool(t)

	var one int
	if err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("query with integration test pool: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 returned %d, want 1", one)
	}
}

func TestIntegration_execSQLIncludesStatementOnError(t *testing.T) {
	pool := GetTestPool(t)
	badSQL := "SELECT definitely_missing_column FROM definitely_missing_table"

	err := execSQL(context.Background(), pool, badSQL)
	if err == nil {
		t.Fatal("execSQL error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), badSQL) {
		t.Fatalf("execSQL error should include statement text; got %q", err.Error())
	}
}
