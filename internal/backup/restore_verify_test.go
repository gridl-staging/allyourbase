package backup

import (
	"context"
	"errors"
	"testing"
)

func TestRestoreVerifierAllChecksPass(t *testing.T) {
	t.Parallel()

	verifier := NewRestoreVerifier("postgres://primary", "postgres://recovery")
	verifier.pingFn = func(ctx context.Context, connURL string) error {
		_ = ctx
		_ = connURL
		return nil
	}
	verifier.listTablesFn = func(ctx context.Context, connURL string) ([]tableSchema, error) {
		_ = ctx
		_ = connURL
		return []tableSchema{
			{Schema: "public", Table: "users", Definition: "id:uuid;email:text"},
			{Schema: "public", Table: "orders", Definition: "id:uuid;user_id:uuid"},
		}, nil
	}
	verifier.countTableFn = func(ctx context.Context, connURL, fullyQualifiedTable string) (int64, error) {
		_ = ctx
		if connURL == "postgres://primary" {
			if fullyQualifiedTable == "public.users" {
				return 100, nil
			}
			if fullyQualifiedTable == "public.orders" {
				return 50, nil
			}
		}
		if connURL == "postgres://recovery" {
			if fullyQualifiedTable == "public.users" {
				return 98, nil
			}
			if fullyQualifiedTable == "public.orders" {
				return 50, nil
			}
		}
		return 0, nil
	}

	result, err := verifier.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected Passed=true, got false: %+v", result)
	}
	if !result.ConnectivityCheck.Passed || !result.SchemaCheck.Passed {
		t.Fatalf("expected connectivity/schema pass, got %+v", result)
	}
	if len(result.RowCountChecks) != 2 {
		t.Fatalf("row check count = %d; want 2", len(result.RowCountChecks))
	}
}

func TestRestoreVerifierSchemaMismatchDetected(t *testing.T) {
	t.Parallel()

	verifier := NewRestoreVerifier("postgres://primary", "postgres://recovery")
	verifier.pingFn = func(context.Context, string) error { return nil }
	verifier.listTablesFn = func(ctx context.Context, connURL string) ([]tableSchema, error) {
		_ = ctx
		if connURL == "postgres://primary" {
			return []tableSchema{{Schema: "public", Table: "users", Definition: "id:uuid;email:text"}}, nil
		}
		return []tableSchema{{Schema: "public", Table: "users", Definition: "id:uuid;email:varchar"}}, nil
	}
	verifier.countTableFn = func(context.Context, string, string) (int64, error) { return 1, nil }

	result, err := verifier.Verify(context.Background())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if result.Passed {
		t.Fatalf("expected Passed=false on schema mismatch")
	}
	if result.SchemaCheck.Passed {
		t.Fatalf("expected schema check to fail")
	}
}

func TestRestoreVerifierConnectivityFailure(t *testing.T) {
	t.Parallel()

	verifier := NewRestoreVerifier("postgres://primary", "postgres://recovery")
	verifier.pingFn = func(ctx context.Context, connURL string) error {
		_ = ctx
		if connURL == "postgres://recovery" {
			return errors.New("connection refused")
		}
		return nil
	}

	_, err := verifier.Verify(context.Background())
	if err == nil {
		t.Fatal("expected connectivity error")
	}
}
