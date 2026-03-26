package audit

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/jackc/pgx/v5/pgconn"
)

type captureExec struct {
	calls      int
	lastSQL    string
	lastArgs   []any
	lastErr    error
	commandTag pgconn.CommandTag
}

func (c *captureExec) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.calls++
	c.lastSQL = sql
	c.lastArgs = args
	if c.lastErr != nil {
		return pgconn.CommandTag{}, c.lastErr
	}
	return c.commandTag, nil
}

type captureExecConcurrent struct {
	mu   sync.Mutex
	args [][]any
}

func (c *captureExecConcurrent) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	_ = ctx
	_ = sql
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := append([]any(nil), args...)
	c.args = append(c.args, cp)
	return pgconn.CommandTag{}, nil
}

func (c *captureExecConcurrent) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.args)
}

func decodeJSONArg(t *testing.T, v any) map[string]any {
	t.Helper()
	b, ok := v.([]byte)
	if !ok {
		t.Fatalf("expected []byte JSON arg, got %T", v)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("decode json arg: %v", err)
	}
	return got
}

func TestShouldAuditTables(t *testing.T) {
	t.Run("all tables enabled", func(t *testing.T) {
		logger := NewAuditLogger(
			config.AuditConfig{Enabled: true, AllTables: true},
			&captureExec{},
		)
		if got, want := logger.ShouldAudit("users"), true; got != want {
			t.Fatalf("all tables: got %v, want %v", got, want)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		logger := NewAuditLogger(config.AuditConfig{Enabled: false}, &captureExec{})
		if got, want := logger.ShouldAudit("users"), false; got != want {
			t.Fatalf("disabled: got %v, want %v", got, want)
		}
	})

	t.Run("table allowlist", func(t *testing.T) {
		logger := NewAuditLogger(config.AuditConfig{
			Enabled: true,
			Tables:  []string{"users", "posts"},
		}, &captureExec{})
		if got, want := logger.ShouldAudit("users"), true; got != want {
			t.Fatalf("users: got %v, want %v", got, want)
		}
		if got, want := logger.ShouldAudit("logs"), false; got != want {
			t.Fatalf("logs: got %v, want %v", got, want)
		}
	})
}

func TestLogMutationHonorsDisabledAuditAndInvalidOperation(t *testing.T) {
	t.Run("disabled no-op", func(t *testing.T) {
		execer := &captureExec{}
		logger := NewAuditLogger(config.AuditConfig{}, execer)

		err := logger.LogMutation(context.Background(), AuditEntry{
			TableName: "users",
			Operation: "INSERT",
		})
		if err != nil {
			t.Fatalf("disabled audit should no-op, got %v", err)
		}
		if execer.calls != 0 {
			t.Fatalf("disabled audit should not call db, got %d calls", execer.calls)
		}
	})

	t.Run("invalid operation", func(t *testing.T) {
		execer := &captureExec{}
		logger := NewAuditLogger(config.AuditConfig{Enabled: true, AllTables: true}, execer)
		err := logger.LogMutation(context.Background(), AuditEntry{
			TableName: "users",
			Operation: "DROP TABLE",
		})
		if err == nil {
			t.Fatal("expected error for invalid operation")
		}
		if execer.calls != 0 {
			t.Fatalf("invalid op should not call db, got %d calls", execer.calls)
		}
	})
}

func TestLogMutationExtractsClaimsAndContextIP(t *testing.T) {
	execer := &captureExec{}
	logger := NewAuditLogger(config.AuditConfig{
		Enabled:   true,
		AllTables: true,
	}, execer)

	claims := &auth.Claims{APIKeyID: "b5fcb3ca-4e5d-4a9a-9b8d-f9e5d7b6f4f5"}
	claims.Subject = "a5a8e9d7-4f0d-4f4b-9d8c-ef9f5f9b2d3a"
	ctx := auth.ContextWithClaims(context.Background(), claims)
	ctx = ContextWithIP(ctx, "203.0.113.10")

	err := logger.LogMutation(ctx, AuditEntry{
		TableName: "users",
		Operation: "INSERT",
		RecordID:  map[string]any{"id": "1"},
		NewValues: map[string]any{"name": "alice"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if execer.calls != 1 {
		t.Fatalf("expected one exec call, got %d", execer.calls)
	}

	// 8 args: user_id, api_key_id, table_name, record_id, operation, old_values, new_values, ip_address
	if len(execer.lastArgs) != 8 {
		t.Fatalf("expected 8 args, got %d", len(execer.lastArgs))
	}
	if execer.lastArgs[0] == nil {
		t.Fatalf("expected user id arg, got %#v", execer.lastArgs[0])
	}
	if execer.lastArgs[1] == nil {
		t.Fatalf("expected api key id arg, got %#v", execer.lastArgs[1])
	}
	if execer.lastArgs[7] == nil {
		t.Fatalf("expected ip arg, got %#v", execer.lastArgs[7])
	}
}

func TestLogMutationUsesContextPrincipalWhenClaimsMissing(t *testing.T) {
	execer := &captureExec{}
	logger := NewAuditLogger(config.AuditConfig{
		Enabled:   true,
		AllTables: true,
	}, execer)

	ctx := ContextWithPrincipal(context.Background(), "d8d7cc9b-10f3-4f4f-8aa7-fb9f6d8fcf48")
	err := logger.LogMutation(ctx, AuditEntry{
		TableName: "users",
		Operation: "INSERT",
		RecordID:  map[string]any{"id": "1"},
		NewValues: map[string]any{"name": "alice"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if execer.calls != 1 {
		t.Fatalf("expected one exec call, got %d", execer.calls)
	}
	if len(execer.lastArgs) != 8 {
		t.Fatalf("expected 8 args, got %d", len(execer.lastArgs))
	}
	if execer.lastArgs[0] != nil {
		t.Fatalf("expected nil user id arg, got %#v", execer.lastArgs[0])
	}
	if execer.lastArgs[1] == nil {
		t.Fatalf("expected principal-backed api key id arg, got %#v", execer.lastArgs[1])
	}
}

func TestMarshalJSONB(t *testing.T) {
	raw, err := marshalJSONB(map[string]any{"a": "b", "n": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok := raw.([]byte)
	if !ok {
		t.Fatalf("expected []byte payload, got %T", raw)
	}

	got := map[string]any{}
	if err := json.Unmarshal(raw.([]byte), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["a"] != "b" {
		t.Fatalf("expected a=b, got %v", got["a"])
	}
}

func TestLogMutationOperationsCaptureExpectedOldNewValues(t *testing.T) {
	t.Parallel()

	type tc struct {
		name      string
		operation string
		oldValues any
		newValues any
	}

	tests := []tc{
		{
			name:      "insert captures new values only",
			operation: "INSERT",
			oldValues: nil,
			newValues: map[string]any{"id": "1", "status": "new"},
		},
		{
			name:      "update captures old and new values",
			operation: "UPDATE",
			oldValues: map[string]any{"id": "1", "status": "old"},
			newValues: map[string]any{"id": "1", "status": "new"},
		},
		{
			name:      "delete captures old values only",
			operation: "DELETE",
			oldValues: map[string]any{"id": "1", "status": "gone"},
			newValues: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			execer := &captureExec{}
			logger := NewAuditLogger(config.AuditConfig{Enabled: true, AllTables: true}, execer)

			err := logger.LogMutation(context.Background(), AuditEntry{
				TableName: "orders",
				Operation: tt.operation,
				RecordID:  map[string]any{"id": "1"},
				OldValues: tt.oldValues,
				NewValues: tt.newValues,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if execer.calls != 1 {
				t.Fatalf("expected one exec call, got %d", execer.calls)
			}
			if len(execer.lastArgs) != 8 {
				t.Fatalf("expected 8 SQL args, got %d", len(execer.lastArgs))
			}
			if gotOp, ok := execer.lastArgs[4].(string); !ok || gotOp != tt.operation {
				t.Fatalf("expected operation %q, got %#v", tt.operation, execer.lastArgs[4])
			}

			gotRecord := decodeJSONArg(t, execer.lastArgs[3])
			if gotRecord["id"] != "1" {
				t.Fatalf("unexpected record id payload: %#v", gotRecord)
			}

			if tt.oldValues == nil {
				if execer.lastArgs[5] != nil {
					t.Fatalf("expected nil old values, got %#v", execer.lastArgs[5])
				}
			} else {
				gotOld := decodeJSONArg(t, execer.lastArgs[5])
				wantOld := tt.oldValues.(map[string]any)
				if gotOld["status"] != wantOld["status"] {
					t.Fatalf("old values mismatch: got %#v want %#v", gotOld, wantOld)
				}
			}

			if tt.newValues == nil {
				if execer.lastArgs[6] != nil {
					t.Fatalf("expected nil new values, got %#v", execer.lastArgs[6])
				}
			} else {
				gotNew := decodeJSONArg(t, execer.lastArgs[6])
				wantNew := tt.newValues.(map[string]any)
				if gotNew["status"] != wantNew["status"] {
					t.Fatalf("new values mismatch: got %#v want %#v", gotNew, wantNew)
				}
			}
		})
	}
}

func TestLogMutationConcurrentCallsDoNotLoseEntries(t *testing.T) {
	t.Parallel()

	execer := &captureExecConcurrent{}
	logger := NewAuditLogger(config.AuditConfig{Enabled: true, AllTables: true}, execer)

	const total = 64
	var wg sync.WaitGroup
	wg.Add(total)

	for i := 0; i < total; i++ {
		i := i
		go func() {
			defer wg.Done()
			err := logger.LogMutation(context.Background(), AuditEntry{
				TableName: "orders",
				Operation: "INSERT",
				RecordID:  map[string]any{"id": i},
				NewValues: map[string]any{"id": i},
			})
			if err != nil {
				t.Errorf("log mutation %d: %v", i, err)
			}
		}()
	}
	wg.Wait()

	if got := execer.callCount(); got != total {
		t.Fatalf("expected %d audit insert calls, got %d", total, got)
	}
}
