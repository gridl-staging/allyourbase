package backup

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestRecoveryInstanceStartStopLifecycle(t *testing.T) {
	t.Parallel()

	inst := NewRecoveryInstance("/tmp/restore-data", 55432, slog.Default())
	var calls [][]string
	inst.runCommand = func(ctx context.Context, name string, args ...string) error {
		_ = ctx
		line := append([]string{name}, args...)
		calls = append(calls, line)
		return nil
	}

	if err := inst.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := inst.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	wantStart := []string{"pg_ctl", "start", "-D", "/tmp/restore-data", "-o", "-p 55432", "-w", "-t", "300"}
	wantStop := []string{"pg_ctl", "stop", "-D", "/tmp/restore-data", "-m", "fast"}
	if len(calls) != 2 {
		t.Fatalf("calls = %d; want 2", len(calls))
	}
	if !reflect.DeepEqual(calls[0], wantStart) {
		t.Fatalf("start args = %#v; want %#v", calls[0], wantStart)
	}
	if !reflect.DeepEqual(calls[1], wantStop) {
		t.Fatalf("stop args = %#v; want %#v", calls[1], wantStop)
	}
	if got := inst.ConnURL(); got != "postgresql://localhost:55432/postgres" {
		t.Fatalf("ConnURL = %q", got)
	}
}

func TestRecoveryInstanceWaitForRecoveryTimeout(t *testing.T) {
	t.Parallel()

	inst := NewRecoveryInstance("/tmp/restore-data", 55432, slog.Default())
	inst.waitTimeout = 30 * time.Millisecond
	inst.pollInterval = 5 * time.Millisecond
	inst.queryRecoveryFn = func(ctx context.Context, connURL string) (bool, error) {
		_ = ctx
		_ = connURL
		return true, nil
	}

	err := inst.WaitForRecovery(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestRecoveryInstanceWaitForRecoverySuccess(t *testing.T) {
	t.Parallel()

	inst := NewRecoveryInstance("/tmp/restore-data", 55432, slog.Default())
	inst.waitTimeout = time.Second
	inst.pollInterval = 5 * time.Millisecond
	attempts := 0
	inst.queryRecoveryFn = func(ctx context.Context, connURL string) (bool, error) {
		_ = ctx
		_ = connURL
		attempts++
		return attempts < 2, nil
	}

	if err := inst.WaitForRecovery(context.Background()); err != nil {
		t.Fatalf("WaitForRecovery: %v", err)
	}
}

func TestRecoveryInstanceFindFreePort(t *testing.T) {
	t.Parallel()

	port, err := FindFreePort()
	if err != nil {
		t.Fatalf("FindFreePort: %v", err)
	}
	if port <= 0 {
		t.Fatalf("invalid port: %d", port)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("expected to bind to returned port %d: %v", port, err)
	}
	_ = ln.Close()
}
