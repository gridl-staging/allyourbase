package observability

import (
	"testing"
)

func TestTenantIDAttr(t *testing.T) {
	attr := TenantIDAttr("tenant-123")
	if attr.Key != TenantIDKey {
		t.Errorf("key = %q, want %q", attr.Key, TenantIDKey)
	}
	if attr.Value.AsString() != "tenant-123" {
		t.Errorf("value = %q, want %q", attr.Value.AsString(), "tenant-123")
	}
}

func TestTenantNameAttr(t *testing.T) {
	attr := TenantNameAttr("Test Tenant")
	if attr.Key != TenantNameKey {
		t.Errorf("key = %q, want %q", attr.Key, TenantNameKey)
	}
	if attr.Value.AsString() != "Test Tenant" {
		t.Errorf("value = %q, want %q", attr.Value.AsString(), "Test Tenant")
	}
}

func TestResourceAttr(t *testing.T) {
	attr := ResourceAttr("db_size_bytes")
	if attr.Key != ResourceKey {
		t.Errorf("key = %q, want %q", attr.Key, ResourceKey)
	}
	if attr.Value.AsString() != "db_size_bytes" {
		t.Errorf("value = %q, want %q", attr.Value.AsString(), "db_size_bytes")
	}
}

func TestCurrentAttr(t *testing.T) {
	attr := CurrentAttr(1500000)
	if attr.Key != CurrentKey {
		t.Errorf("key = %q, want %q", attr.Key, CurrentKey)
	}
	if attr.Value.AsInt64() != 1500000 {
		t.Errorf("value = %d, want %d", attr.Value.AsInt64(), 1500000)
	}
}

func TestLimitAttr(t *testing.T) {
	attr := LimitAttr(1000000)
	if attr.Key != LimitKey {
		t.Errorf("key = %q, want %q", attr.Key, LimitKey)
	}
	if attr.Value.AsInt64() != 1000000 {
		t.Errorf("value = %d, want %d", attr.Value.AsInt64(), 1000000)
	}
}

func TestOperationAttr(t *testing.T) {
	attr := OperationAttr("read")
	if attr.Key != OperationKey {
		t.Errorf("key = %q, want %q", attr.Key, OperationKey)
	}
	if attr.Value.AsString() != "read" {
		t.Errorf("value = %q, want %q", attr.Value.AsString(), "read")
	}
}

func TestTargetTenantAttr(t *testing.T) {
	attr := TargetTenantAttr("tenant-456")
	if attr.Key != TargetTenantKey {
		t.Errorf("key = %q, want %q", attr.Key, TargetTenantKey)
	}
	if attr.Value.AsString() != "tenant-456" {
		t.Errorf("value = %q, want %q", attr.Value.AsString(), "tenant-456")
	}
}

func TestTenantAttrs(t *testing.T) {
	attrs := TenantAttrs("tenant-123", ResourceAttr("db_size"), CurrentAttr(500))
	if len(attrs) != 3 {
		t.Errorf("len = %d, want 3", len(attrs))
	}

	foundTenant := false
	foundResource := false
	foundCurrent := false
	for _, attr := range attrs {
		switch attr.Key {
		case TenantIDKey:
			foundTenant = true
			if attr.Value.AsString() != "tenant-123" {
				t.Errorf("tenant_id value = %q, want %q", attr.Value.AsString(), "tenant-123")
			}
		case ResourceKey:
			foundResource = true
		case CurrentKey:
			foundCurrent = true
		}
	}

	if !foundTenant {
		t.Error("tenant_id attr not found")
	}
	if !foundResource {
		t.Error("resource attr not found")
	}
	if !foundCurrent {
		t.Error("current attr not found")
	}
}

func TestTenantAttrs_Empty(t *testing.T) {
	attrs := TenantAttrs("tenant-123")
	if len(attrs) != 1 {
		t.Errorf("len = %d, want 1", len(attrs))
	}
	if attrs[0].Key != TenantIDKey {
		t.Errorf("key = %q, want %q", attrs[0].Key, TenantIDKey)
	}
}

func TestTenantAttrs_Consistency(t *testing.T) {
	attrs1 := TenantAttrs("tenant-1", ResourceAttr("db"))
	attrs2 := TenantAttrs("tenant-1", ResourceAttr("db"))

	if len(attrs1) != len(attrs2) {
		t.Errorf("different lengths: %d vs %d", len(attrs1), len(attrs2))
	}

	for i := range attrs1 {
		if attrs1[i].Key != attrs2[i].Key {
			t.Errorf("key mismatch at %d: %q vs %q", i, attrs1[i].Key, attrs2[i].Key)
		}
		if attrs1[i].Value != attrs2[i].Value {
			t.Errorf("value mismatch at %d: %v vs %v", i, attrs1[i].Value, attrs2[i].Value)
		}
	}
}

func TestNewTenantMetrics_NilMeter(t *testing.T) {
	m, err := NewTenantMetrics(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if m != nil {
		t.Error("expected nil TenantMetrics with nil meter")
	}
}
