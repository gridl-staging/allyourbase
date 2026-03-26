package tenant

import "testing"

func TestTenantConnCounter_AdmitUnderLimit(t *testing.T) {
	t.Parallel()

	counter := NewTenantConnCounter()
	hard := 3
	soft := 2

	allowed, softWarn, count := counter.Admit("tenant-1", &hard, &soft)
	if !allowed {
		t.Fatal("first admit should be allowed")
	}
	if softWarn {
		t.Fatal("first admit should not be soft warning")
	}
	if count != 1 {
		t.Fatalf("count=%d, want 1", count)
	}

	allowed, softWarn, count = counter.Admit("tenant-1", &hard, &soft)
	if !allowed {
		t.Fatal("second admit should be allowed")
	}
	if !softWarn {
		t.Fatal("second admit should be soft warning")
	}
	if count != 2 {
		t.Fatalf("count=%d, want 2", count)
	}
}

func TestTenantConnCounter_DenyAtHardLimit(t *testing.T) {
	t.Parallel()

	counter := NewTenantConnCounter()
	hard := 1
	soft := 2

	allowed, _, _ := counter.Admit("tenant-2", &hard, &soft)
	if !allowed {
		t.Fatal("first admit should be allowed")
	}

	allowed, _, count := counter.Admit("tenant-2", &hard, &soft)
	if allowed {
		t.Fatal("second admit should be denied")
	}
	if count != 1 {
		t.Fatalf("count=%d, want 1", count)
	}
}

func TestTenantConnCounter_SoftWarningBetweenLimits(t *testing.T) {
	t.Parallel()

	counter := NewTenantConnCounter()
	hard := 4
	soft := 2

	for i := 0; i < 2; i++ {
		allowed, softWarn, count := counter.Admit("tenant-3", &hard, &soft)
		if !allowed {
			t.Fatalf("admit %d should be allowed", i+1)
		}
		if i == 0 && softWarn {
			t.Fatalf("admit %d should not be soft warning", i+1)
		}
		if i == 1 && !softWarn {
			t.Fatalf("admit %d should be soft warning", i+1)
		}
		if count != int64(i+1) {
			t.Fatalf("count=%d, want %d", count, i+1)
		}
	}
}

func TestTenantConnCounter_NilLimitsAlwaysAllow(t *testing.T) {
	t.Parallel()

	counter := NewTenantConnCounter()
	allowed, softWarn, count := counter.Admit("tenant-4", nil, nil)
	if !allowed {
		t.Fatal("nil limits should allow")
	}
	if softWarn {
		t.Fatal("nil limits should not soft warn")
	}
	if count != 1 {
		t.Fatalf("count=%d, want 1", count)
	}
}

func TestTenantConnCounter_ReleaseClampsToZero(t *testing.T) {
	t.Parallel()

	counter := NewTenantConnCounter()
	counter.Admit("tenant-5", nil, nil)
	counter.Admit("tenant-5", nil, nil)

	counter.Release("tenant-5")
	counter.Release("tenant-5")
	counter.Release("tenant-5")

	counter.mu.Lock()
	defer counter.mu.Unlock()
	if _, ok := counter.tenants["tenant-5"]; ok {
		t.Fatal("expected tenant key to be removed when count reaches zero")
	}
}

func TestTenantConnCounter_ConcurrentAdmitReleaseSafety(t *testing.T) {
	t.Parallel()

	counter := NewTenantConnCounter()
	hard := 100
	soft := 90
	done := make(chan struct{}, 16)

	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				allowed, _, count := counter.Admit("tenant-6", &hard, &soft)
				if !allowed {
					t.Error("admit should always be allowed with headroom")
					continue
				}
				counter.Release("tenant-6")
				if count <= 0 {
					t.Errorf("count should remain positive while admitted, got %d", count)
				}
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 8; i++ {
		<-done
	}

	counter.mu.Lock()
	defer counter.mu.Unlock()
	if got := counter.tenants["tenant-6"]; got != 0 {
		t.Fatalf("count=%d, want 0", got)
	}
}
