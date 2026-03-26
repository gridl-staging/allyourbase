package tenant

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUsageAccumulator_RecordRoundTrip(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)

	tenantID := "test-tenant"
	resource := ResourceTypeRequestRate

	ua.Record(tenantID, resource, 10)
	ua.Record(tenantID, resource, 20)
	ua.Record(tenantID, resource, 5)

	got := ua.GetCurrentWindow(tenantID, resource)
	want := int64(35)

	if got != want {
		t.Errorf("GetCurrentWindow() = %d, want %d", got, want)
	}
}

func TestUsageAccumulator_RecordMultipleTenants(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)

	ua.Record("tenant-a", ResourceTypeRequestRate, 100)
	ua.Record("tenant-b", ResourceTypeRequestRate, 200)
	ua.Record("tenant-a", ResourceTypeDBSizeBytes, 500)

	if ua.GetCurrentWindow("tenant-a", ResourceTypeRequestRate) != 100 {
		t.Error("tenant-a request rate should be 100")
	}
	if ua.GetCurrentWindow("tenant-b", ResourceTypeRequestRate) != 200 {
		t.Error("tenant-b request rate should be 200")
	}
	if ua.GetCurrentWindow("tenant-a", ResourceTypeDBSizeBytes) != 500 {
		t.Error("tenant-a db size should be 500")
	}
}

func TestUsageAccumulator_RecordMultipleResources(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)

	tenantID := "test-tenant"

	ua.Record(tenantID, ResourceTypeRequestRate, 10)
	ua.Record(tenantID, ResourceTypeDBSizeBytes, 100)
	ua.Record(tenantID, ResourceTypeJobConcurrency, 5)

	if ua.GetCurrentWindow(tenantID, ResourceTypeRequestRate) != 10 {
		t.Error("request rate should be 10")
	}
	if ua.GetCurrentWindow(tenantID, ResourceTypeDBSizeBytes) != 100 {
		t.Error("db size should be 100")
	}
	if ua.GetCurrentWindow(tenantID, ResourceTypeJobConcurrency) != 5 {
		t.Error("job concurrency should be 5")
	}
}

func TestUsageAccumulator_RecordPeakIncreasing(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)

	tenantID := "test-tenant"
	resource := ResourceTypeRealtimeConns

	ua.RecordPeak(tenantID, resource, 10)
	ua.RecordPeak(tenantID, resource, 25)
	ua.RecordPeak(tenantID, resource, 15)
	ua.RecordPeak(tenantID, resource, 30)

	got := ua.GetCurrentPeakWindow(tenantID, resource)
	want := int64(30)

	if got != want {
		t.Errorf("GetCurrentPeakWindow() = %d, want %d", got, want)
	}
}

func TestUsageAccumulator_RecordPeakDecreasing(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)

	tenantID := "test-tenant"
	resource := ResourceTypeRealtimeConns

	ua.RecordPeak(tenantID, resource, 50)
	ua.RecordPeak(tenantID, resource, 40)
	ua.RecordPeak(tenantID, resource, 30)

	got := ua.GetCurrentPeakWindow(tenantID, resource)
	want := int64(50)

	if got != want {
		t.Errorf("GetCurrentPeakWindow() = %d, want %d (should retain max)", got, want)
	}
}

func TestUsageAccumulator_RecordPeakZeroInitially(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)

	tenantID := "test-tenant"
	resource := ResourceTypeRealtimeConns

	ua.RecordPeak(tenantID, resource, 5)

	got := ua.GetCurrentPeakWindow(tenantID, resource)
	if got != 5 {
		t.Errorf("GetCurrentPeakWindow() = %d, want 5", got)
	}
}

func TestUsageAccumulator_ConcurrentRecord(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)

	tenantID := "test-tenant"
	resource := ResourceTypeRequestRate

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				ua.Record(tenantID, resource, 1)
			}
		}()
	}
	wg.Wait()

	got := ua.GetCurrentWindow(tenantID, resource)
	want := int64(1000)

	if got != want {
		t.Errorf("GetCurrentWindow() = %d, want %d after concurrent writes", got, want)
	}
}

func TestUsageAccumulator_ConcurrentRecordPeak(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)

	tenantID := "test-tenant"
	resource := ResourceTypeRealtimeConns

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(val int64) {
			defer wg.Done()
			ua.RecordPeak(tenantID, resource, val)
		}(int64(i * 10))
	}
	wg.Wait()

	got := ua.GetCurrentPeakWindow(tenantID, resource)
	want := int64(90)

	if got != want {
		t.Errorf("GetCurrentPeakWindow() = %d, want %d after concurrent writes", got, want)
	}
}

func TestUsageAccumulator_GetCurrentWindow_EmptyTenant(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)

	got := ua.GetCurrentWindow("nonexistent", ResourceTypeRequestRate)
	if got != 0 {
		t.Errorf("GetCurrentWindow() for nonexistent tenant = %d, want 0", got)
	}
}

func TestUsageAccumulator_GetCurrentWindow_EmptyResource(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)

	ua.Record("tenant-a", ResourceTypeRequestRate, 100)

	got := ua.GetCurrentWindow("tenant-a", ResourceTypeDBSizeBytes)
	if got != 0 {
		t.Errorf("GetCurrentWindow() for unset resource = %d, want 0", got)
	}
}

func TestUsageAccumulator_FlushClearsCounters(t *testing.T) {
	ctx := context.Background()
	ua := NewUsageAccumulator(nil, nil)

	ua.Record("tenant-a", ResourceTypeRequestRate, 100)
	ua.Record("tenant-b", ResourceTypeRequestRate, 200)

	ua.additiveCounters["tenant-a"][ResourceTypeDBSizeBytes] = 500

	if err := ua.Flush(ctx); err != nil {
		t.Skipf("Flush requires DB pool, skipping: %v", err)
	}

	if ua.GetCurrentWindow("tenant-a", ResourceTypeRequestRate) != 0 {
		t.Error("counter should be cleared after flush")
	}
}

func TestUsageAccumulator_GetCurrentUsage_NilPoolReturnsWindowUsage(t *testing.T) {
	ua := NewUsageAccumulator(nil, nil)
	ua.Record("tenant-a", ResourceTypeRequestRate, 42)

	got, err := ua.GetCurrentUsage(context.Background(), "tenant-a", ResourceTypeRequestRate)
	if err != nil {
		t.Fatalf("GetCurrentUsage() error = %v, want nil", err)
	}
	if got != 42 {
		t.Fatalf("GetCurrentUsage() = %d, want 42", got)
	}
}

func TestUsageAccumulator_FlushRestoresCountersOnDBError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig("postgres://127.0.0.1:1/postgres?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewWithConfig() error = %v", err)
	}
	defer pool.Close()

	ua := NewUsageAccumulator(pool, nil)
	ua.Record("tenant-a", ResourceTypeRequestRate, 100)
	ua.RecordPeak("tenant-a", ResourceTypeRealtimeConns, 7)

	err = ua.Flush(ctx)
	if err == nil {
		t.Fatal("Flush() error = nil, want connection failure")
	}

	if got := ua.GetCurrentWindow("tenant-a", ResourceTypeRequestRate); got != 100 {
		t.Fatalf("request counter after failed flush = %d, want 100", got)
	}
	if got := ua.GetCurrentPeakWindow("tenant-a", ResourceTypeRealtimeConns); got != 7 {
		t.Fatalf("peak counter after failed flush = %d, want 7", got)
	}
}
