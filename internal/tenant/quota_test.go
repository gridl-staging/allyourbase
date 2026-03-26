package tenant

import (
	"testing"
)

func TestCheckQuota_NilQuotasAllows(t *testing.T) {
	result := CheckQuota(nil, ResourceTypeRequestRate, 100, 10)

	if !result.Allowed {
		t.Error("expected Allowed=true when quotas is nil")
	}
	if result.HardLimited {
		t.Error("expected HardLimited=false when quotas is nil")
	}
	if result.SoftWarning {
		t.Error("expected SoftWarning=false when quotas is nil")
	}
}

func TestCheckQuota_NilResourceFieldAllows(t *testing.T) {
	quotas := &TenantQuotas{
		TenantID:                "test-tenant",
		RequestRateRPSHard:      nil,
		RequestRateRPSSoft:      nil,
		DBSizeBytesHard:         nil,
		DBSizeBytesSoft:         nil,
		RealtimeConnectionsHard: nil,
		RealtimeConnectionsSoft: nil,
		JobConcurrencyHard:      nil,
		JobConcurrencySoft:      nil,
	}

	result := CheckQuota(quotas, ResourceTypeRequestRate, 100, 10)

	if !result.Allowed {
		t.Error("expected Allowed=true when resource field is nil")
	}
}

func TestCheckQuota_SoftOnlyWarnsNoDeny(t *testing.T) {
	soft := 100
	quotas := &TenantQuotas{
		TenantID:           "test-tenant",
		RequestRateRPSSoft: &soft,
	}

	result := CheckQuota(quotas, ResourceTypeRequestRate, 50, 60)

	if !result.Allowed {
		t.Error("expected Allowed=true when only soft limit and under hard")
	}
	if !result.SoftWarning {
		t.Error("expected SoftWarning=true when >= soft threshold")
	}
	if result.HardLimited {
		t.Error("expected HardLimited=false when no hard limit set")
	}
}

func TestCheckQuota_SoftOnlyAtThresholdWarns(t *testing.T) {
	soft := 100
	quotas := &TenantQuotas{
		TenantID:           "test-tenant",
		RequestRateRPSSoft: &soft,
	}

	result := CheckQuota(quotas, ResourceTypeRequestRate, 90, 10)

	if !result.Allowed {
		t.Error("expected Allowed=true at soft threshold")
	}
	if !result.SoftWarning {
		t.Error("expected SoftWarning=true at soft threshold")
	}
}

func TestCheckQuota_HardOnlyDeniesAtThreshold(t *testing.T) {
	hard := 100
	quotas := &TenantQuotas{
		TenantID:           "test-tenant",
		RequestRateRPSHard: &hard,
	}

	result := CheckQuota(quotas, ResourceTypeRequestRate, 90, 10)

	if result.Allowed {
		t.Error("expected Allowed=false when current+proposed >= hard")
	}
	if !result.HardLimited {
		t.Error("expected HardLimited=true when hard exceeded")
	}
	if result.SoftWarning {
		t.Error("expected SoftWarning=false when only hard limit")
	}
}

func TestCheckQuota_HardOnlyBelowThresholdAllows(t *testing.T) {
	hard := 100
	quotas := &TenantQuotas{
		TenantID:           "test-tenant",
		RequestRateRPSHard: &hard,
	}

	result := CheckQuota(quotas, ResourceTypeRequestRate, 50, 30)

	if !result.Allowed {
		t.Error("expected Allowed=true when under hard limit")
	}
	if result.HardLimited {
		t.Error("expected HardLimited=false when under hard limit")
	}
	if result.SoftWarning {
		t.Error("expected SoftWarning=false when no soft limit")
	}
}

func TestCheckQuota_BetweenSoftAndHardWarns(t *testing.T) {
	soft := 100
	hard := 200
	quotas := &TenantQuotas{
		TenantID:           "test-tenant",
		RequestRateRPSSoft: &soft,
		RequestRateRPSHard: &hard,
	}

	result := CheckQuota(quotas, ResourceTypeRequestRate, 50, 60)

	if !result.Allowed {
		t.Error("expected Allowed=true between soft and hard")
	}
	if !result.SoftWarning {
		t.Error("expected SoftWarning=true when >= soft")
	}
	if result.HardLimited {
		t.Error("expected HardLimited=false when under hard")
	}
}

func TestCheckQuota_BelowSoftNoWarning(t *testing.T) {
	soft := 100
	hard := 200
	quotas := &TenantQuotas{
		TenantID:           "test-tenant",
		RequestRateRPSSoft: &soft,
		RequestRateRPSHard: &hard,
	}

	result := CheckQuota(quotas, ResourceTypeRequestRate, 20, 30)

	if !result.Allowed {
		t.Error("expected Allowed=true when below soft")
	}
	if result.SoftWarning {
		t.Error("expected SoftWarning=false when below soft")
	}
	if result.HardLimited {
		t.Error("expected HardLimited=false when below soft")
	}
}

func TestCheckQuota_ProposedHandling_CurrentBelowHardButProposedCrosses(t *testing.T) {
	hard := 100
	quotas := &TenantQuotas{
		TenantID:           "test-tenant",
		RequestRateRPSHard: &hard,
	}

	result := CheckQuota(quotas, ResourceTypeRequestRate, 80, 30)

	if result.Allowed {
		t.Error("expected denied when current+proposed crosses hard")
	}
	if !result.HardLimited {
		t.Error("expected HardLimited=true when hard exceeded")
	}
}

func TestCheckQuota_ProposedZeroAtThreshold(t *testing.T) {
	hard := 100
	quotas := &TenantQuotas{
		TenantID:           "test-tenant",
		RequestRateRPSHard: &hard,
	}

	result := CheckQuota(quotas, ResourceTypeRequestRate, 100, 0)

	if result.Allowed {
		t.Error("expected denied when current == hard with zero proposed")
	}
	if !result.HardLimited {
		t.Error("expected HardLimited=true at hard threshold")
	}
}

func TestCheckQuota_DifferentResourceTypes(t *testing.T) {
	dbHard := int64(1000000)
	connHard := 50
	jobHard := 10

	quotas := &TenantQuotas{
		TenantID:                "test-tenant",
		DBSizeBytesHard:         &dbHard,
		RealtimeConnectionsHard: &connHard,
		JobConcurrencyHard:      &jobHard,
	}

	dbResult := CheckQuota(quotas, ResourceTypeDBSizeBytes, 500000, 600000)
	if dbResult.Allowed {
		t.Error("expected DB size denied when exceeding hard")
	}

	connResult := CheckQuota(quotas, ResourceTypeRealtimeConns, 30, 30)
	if connResult.Allowed {
		t.Error("expected connections denied when exceeding hard")
	}

	jobResult := CheckQuota(quotas, ResourceTypeJobConcurrency, 5, 10)
	if jobResult.Allowed {
		t.Error("expected job concurrency denied when exceeding hard")
	}
}

func TestCheckQuota_DBSizeBytesResource(t *testing.T) {
	soft := int64(1000)
	hard := int64(2000)
	quotas := &TenantQuotas{
		TenantID:        "test-tenant",
		DBSizeBytesSoft: &soft,
		DBSizeBytesHard: &hard,
	}

	result := CheckQuota(quotas, ResourceTypeDBSizeBytes, 500, 600)
	if !result.SoftWarning {
		t.Error("expected SoftWarning=true when >= soft")
	}

	result = CheckQuota(quotas, ResourceTypeDBSizeBytes, 1500, 600)
	if result.Allowed {
		t.Error("expected denied when >= hard")
	}
}

func TestCheckQuota_FieldsSetCorrectly(t *testing.T) {
	soft := 100
	hard := 200
	quotas := &TenantQuotas{
		TenantID:           "test-tenant",
		RequestRateRPSSoft: &soft,
		RequestRateRPSHard: &hard,
	}

	result := CheckQuota(quotas, ResourceTypeRequestRate, 50, 30)

	if result.Current != 50 {
		t.Errorf("expected Current=50, got %d", result.Current)
	}
	if result.Proposed != 30 {
		t.Errorf("expected Proposed=30, got %d", result.Proposed)
	}

	result = CheckQuota(quotas, ResourceTypeRequestRate, 50, 60)
	if result.Limit != 100 {
		t.Errorf("expected Limit=100 (soft), got %d", result.Limit)
	}
}

func TestDefaultQuotaChecker_ImplementsInterface(t *testing.T) {
	var _ QuotaChecker = DefaultQuotaChecker{}

	checker := DefaultQuotaChecker{}
	soft := 100
	quotas := &TenantQuotas{
		TenantID:           "test-tenant",
		RequestRateRPSSoft: &soft,
	}

	result := checker.CheckQuota(quotas, ResourceTypeRequestRate, 50, 60)

	if !result.SoftWarning {
		t.Error("expected SoftWarning from checker")
	}
}
