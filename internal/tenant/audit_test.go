package tenant

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAuditActionConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"tenant created", AuditActionTenantCreated, "tenant.created"},
		{"tenant updated", AuditActionTenantUpdated, "tenant.updated"},
		{"tenant suspended", AuditActionTenantSuspended, "tenant.suspended"},
		{"tenant resumed", AuditActionTenantResumed, "tenant.resumed"},
		{"tenant deleted", AuditActionTenantDeleted, "tenant.deleted"},
		{"membership added", AuditActionMembershipAdded, "membership.added"},
		{"membership removed", AuditActionMembershipRemoved, "membership.removed"},
		{"membership role change", AuditActionMembershipRoleChange, "membership.role_change"},
		{"quota change", AuditActionQuotaChange, "quota.change"},
		{"quota violation", AuditActionQuotaViolation, "quota.violation"},
		{"cross tenant blocked", AuditActionCrossTenantBlocked, "cross_tenant.blocked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("constant = %q, want %q", tt.constant, tt.expected)
			}
		})
	}
}

func TestAuditResultConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"success", AuditResultSuccess, "success"},
		{"failure", AuditResultFailure, "failure"},
		{"denied", AuditResultDenied, "denied"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("constant = %q, want %q", tt.constant, tt.expected)
			}
		})
	}
}

func TestAuditEmitter_Emit(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	actorID := "user-123"
	ipAddress := "192.168.1.1"
	metadata := map[string]string{"key": "value"}

	err := emitter.Emit(ctx, "tenant-1", AuditActionTenantCreated, AuditResultSuccess, &actorID, metadata, &ipAddress)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mock.lastAction != AuditActionTenantCreated {
		t.Errorf("action = %q, want %q", mock.lastAction, AuditActionTenantCreated)
	}
}

func TestAuditEmitter_EmitWithNilLogger(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, nil)

	ctx := context.Background()
	actorID := "user-123"
	metadata := map[string]string{"key": "value"}

	err := emitter.Emit(ctx, "tenant-1", AuditActionTenantCreated, AuditResultSuccess, &actorID, metadata, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mock.lastAction != AuditActionTenantCreated {
		t.Errorf("action = %q, want %q", mock.lastAction, AuditActionTenantCreated)
	}
}

func TestAuditEmitter_BuildMetadata(t *testing.T) {
	emitter := NewAuditEmitter(nil, testutil.DiscardLogger())

	meta := emitter.BuildMetadata("key1", "value1", "key2", "value2")

	var result map[string]string
	if err := json.Unmarshal(meta, &result); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if result["key1"] != "value1" {
		t.Errorf("key1 = %q, want %q", result["key1"], "value1")
	}
	if result["key2"] != "value2" {
		t.Errorf("key2 = %q, want %q", result["key2"], "value2")
	}
}

func TestAuditEmitter_BuildMetadata_Empty(t *testing.T) {
	emitter := NewAuditEmitter(nil, testutil.DiscardLogger())

	meta := emitter.BuildMetadata()

	if string(meta) != "{}" {
		t.Errorf("metadata = %s, want {}", meta)
	}
}

func TestAuditEmitter_BuildMetadata_OddArgs(t *testing.T) {
	emitter := NewAuditEmitter(nil, testutil.DiscardLogger())

	meta := emitter.BuildMetadata("key1", "value1", "key2")

	var result map[string]any
	if err := json.Unmarshal(meta, &result); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if result["key1"] != "value1" {
		t.Errorf("key1 = %q, want %q", result["key1"], "value1")
	}
	if _, ok := result["key2"]; ok {
		t.Error("expected key2 to be missing due to odd number of args")
	}
}

type mockAuditInserter struct {
	lastTenantID string
	lastAction   string
	lastResult   string
	lastMetadata json.RawMessage
	err          error
}

func (m *mockAuditInserter) InsertAuditEvent(_ context.Context, tenantID string, actorID *string, action, result string, metadata json.RawMessage, ipAddress *string) error {
	m.lastTenantID = tenantID
	m.lastAction = action
	m.lastResult = result
	m.lastMetadata = metadata
	return m.err
}

func TestAuditEmitter_EmitLifecycle_TenantCreated(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	actorID := "user-123"
	ipAddress := "192.168.1.1"

	err := emitter.EmitTenantCreated(ctx, "tenant-1", &actorID, "Test Tenant", &ipAddress)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mock.lastAction != AuditActionTenantCreated {
		t.Errorf("action = %q, want %q", mock.lastAction, AuditActionTenantCreated)
	}
	if mock.lastTenantID != "tenant-1" {
		t.Errorf("tenantID = %q, want %q", mock.lastTenantID, "tenant-1")
	}

	var meta map[string]string
	_ = json.Unmarshal(mock.lastMetadata, &meta)
	if meta["name"] != "Test Tenant" {
		t.Errorf("metadata name = %q, want %q", meta["name"], "Test Tenant")
	}
}

func TestAuditEmitter_EmitLifecycle_TenantSuspended(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	actorID := "user-123"

	err := emitter.EmitTenantSuspended(ctx, "tenant-1", &actorID, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mock.lastAction != AuditActionTenantSuspended {
		t.Errorf("action = %q, want %q", mock.lastAction, AuditActionTenantSuspended)
	}
	if mock.lastResult != AuditResultSuccess {
		t.Errorf("result = %q, want %q", mock.lastResult, AuditResultSuccess)
	}
}

func TestAuditEmitter_EmitMembership_MemberAdded(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	actorID := "admin-123"

	err := emitter.EmitMembershipAdded(ctx, "tenant-1", "user-456", "admin", &actorID, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mock.lastAction != AuditActionMembershipAdded {
		t.Errorf("action = %q, want %q", mock.lastAction, AuditActionMembershipAdded)
	}

	var meta map[string]string
	_ = json.Unmarshal(mock.lastMetadata, &meta)
	if meta["userId"] != "user-456" {
		t.Errorf("metadata userId = %q, want %q", meta["userId"], "user-456")
	}
	if meta["role"] != "admin" {
		t.Errorf("metadata role = %q, want %q", meta["role"], "admin")
	}
}

func TestAuditEmitter_EmitMembership_MemberRemoved(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	actorID := "admin-123"

	err := emitter.EmitMembershipRemoved(ctx, "tenant-1", "user-456", &actorID, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mock.lastAction != AuditActionMembershipRemoved {
		t.Errorf("action = %q, want %q", mock.lastAction, AuditActionMembershipRemoved)
	}
}

func TestAuditEmitter_EmitMembership_RoleChanged(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	actorID := "admin-123"

	err := emitter.EmitMembershipRoleChange(ctx, "tenant-1", "user-456", "viewer", "admin", &actorID, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mock.lastAction != AuditActionMembershipRoleChange {
		t.Errorf("action = %q, want %q", mock.lastAction, AuditActionMembershipRoleChange)
	}

	var meta map[string]string
	_ = json.Unmarshal(mock.lastMetadata, &meta)
	if meta["oldRole"] != "viewer" {
		t.Errorf("metadata oldRole = %q, want %q", meta["oldRole"], "viewer")
	}
	if meta["newRole"] != "admin" {
		t.Errorf("metadata newRole = %q, want %q", meta["newRole"], "admin")
	}
}

func TestAuditEmitter_EmitQuota_Change(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	actorID := "admin-123"
	changes := map[string]any{
		"dbSizeBytesHard": int64(1000000),
	}

	err := emitter.EmitQuotaChange(ctx, "tenant-1", changes, &actorID, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mock.lastAction != AuditActionQuotaChange {
		t.Errorf("action = %q, want %q", mock.lastAction, AuditActionQuotaChange)
	}

	var meta map[string]any
	_ = json.Unmarshal(mock.lastMetadata, &meta)
	if meta["changes"] == nil {
		t.Error("expected changes in metadata")
	}
}

func TestAuditEmitter_EmitQuota_Violation(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	actorID := "user-123"

	err := emitter.EmitQuotaViolation(ctx, "tenant-1", "db_size_bytes", 1500000, 1000000, &actorID, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mock.lastAction != AuditActionQuotaViolation {
		t.Errorf("action = %q, want %q", mock.lastAction, AuditActionQuotaViolation)
	}
	if mock.lastResult != AuditResultDenied {
		t.Errorf("result = %q, want %q", mock.lastResult, AuditResultDenied)
	}

	var meta map[string]any
	_ = json.Unmarshal(mock.lastMetadata, &meta)
	if meta["resource"] != "db_size_bytes" {
		t.Errorf("metadata resource = %v, want %q", meta["resource"], "db_size_bytes")
	}
	if meta["current"] != float64(1500000) {
		t.Errorf("metadata current = %v, want %v", meta["current"], 1500000)
	}
	if meta["limit"] != float64(1000000) {
		t.Errorf("metadata limit = %v, want %v", meta["limit"], 1000000)
	}
}

func TestAuditEmitter_EmitCrossTenant_Blocked(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	actorID := "user-123"

	err := emitter.EmitCrossTenantBlocked(ctx, "tenant-1", "tenant-2", "resource-1", "read", &actorID, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if mock.lastAction != AuditActionCrossTenantBlocked {
		t.Errorf("action = %q, want %q", mock.lastAction, AuditActionCrossTenantBlocked)
	}
	if mock.lastResult != AuditResultDenied {
		t.Errorf("result = %q, want %q", mock.lastResult, AuditResultDenied)
	}

	var meta map[string]string
	_ = json.Unmarshal(mock.lastMetadata, &meta)
	if meta["targetTenantId"] != "tenant-2" {
		t.Errorf("metadata targetTenantId = %q, want %q", meta["targetTenantId"], "tenant-2")
	}
	if meta["resourceId"] != "resource-1" {
		t.Errorf("metadata resourceId = %q, want %q", meta["resourceId"], "resource-1")
	}
	if meta["operation"] != "read" {
		t.Errorf("metadata operation = %q, want %q", meta["operation"], "read")
	}
}

// --- EmitWithOrg tests ---

type mockOrgAwareAuditInserter struct {
	mockAuditInserter
	lastOrgID    *string
	lookupOrgID  *string
	lookupOrgErr error
}

func (m *mockOrgAwareAuditInserter) InsertAuditEventWithOrg(_ context.Context, tenantID string, actorID *string, action, result string, metadata json.RawMessage, ipAddress *string, orgID *string) error {
	m.lastTenantID = tenantID
	m.lastAction = action
	m.lastResult = result
	m.lastMetadata = metadata
	m.lastOrgID = orgID
	return m.err
}

func (m *mockOrgAwareAuditInserter) TenantOrgID(_ context.Context, _ string) (*string, error) {
	return m.lookupOrgID, m.lookupOrgErr
}

func TestAuditEmitter_EmitWithOrg_PopulatesOrgID(t *testing.T) {
	mock := &mockOrgAwareAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	orgID := "org-123"
	actorID := "user-1"

	err := emitter.EmitWithOrg(ctx, "tenant-1", AuditActionTenantCreated, AuditResultSuccess, &actorID, nil, nil, &orgID)
	testutil.NoError(t, err)
	testutil.Equal(t, "tenant-1", mock.lastTenantID)
	testutil.Equal(t, AuditActionTenantCreated, mock.lastAction)

	if mock.lastOrgID == nil || *mock.lastOrgID != "org-123" {
		t.Errorf("expected org_id = org-123, got %v", mock.lastOrgID)
	}
}

func TestAuditEmitter_EmitWithOrg_NilOrgFallsBackToEmit(t *testing.T) {
	mock := &mockOrgAwareAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	err := emitter.EmitWithOrg(ctx, "tenant-1", AuditActionTenantUpdated, AuditResultSuccess, nil, nil, nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, AuditActionTenantUpdated, mock.lastAction)
	// When orgID is nil, the regular InsertAuditEvent path is used
	if mock.lastOrgID != nil {
		t.Errorf("expected nil org_id for nil org, got %v", mock.lastOrgID)
	}
}

func TestAuditEmitter_EmitWithOrg_NonOrgAwareInserterFallsBack(t *testing.T) {
	// A basic inserter that doesn't implement orgAwareAuditInserter
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	ctx := context.Background()
	orgID := "org-456"
	err := emitter.EmitWithOrg(ctx, "tenant-1", AuditActionTenantCreated, AuditResultSuccess, nil, nil, nil, &orgID)
	testutil.NoError(t, err)
	// Falls back to regular Emit, org_id is lost but no error
	testutil.Equal(t, AuditActionTenantCreated, mock.lastAction)
}

func TestAuditEmitter_EmitResolvesTenantOrgID(t *testing.T) {
	mock := &mockOrgAwareAuditInserter{}
	orgID := "org-lookup"
	mock.lookupOrgID = &orgID
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	err := emitter.Emit(context.Background(), "tenant-1", AuditActionTenantUpdated, AuditResultSuccess, nil, map[string]any{"field": "value"}, nil)
	testutil.NoError(t, err)
	if mock.lastOrgID == nil || *mock.lastOrgID != orgID {
		t.Fatalf("expected resolved org_id %q, got %v", orgID, mock.lastOrgID)
	}
}

func TestAuditEmitter_EmitTenantAssignedToOrg_UsesExplicitOrgID(t *testing.T) {
	mock := &mockOrgAwareAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	err := emitter.EmitTenantAssignedToOrg(context.Background(), "tenant-1", "org-123", nil, nil)
	testutil.NoError(t, err)
	if mock.lastOrgID == nil || *mock.lastOrgID != "org-123" {
		t.Fatalf("expected assigned org_id to be written, got %v", mock.lastOrgID)
	}
}
