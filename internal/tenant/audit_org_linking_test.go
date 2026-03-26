package tenant

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAuditActionTenantOrgLinkingConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{name: "tenant assigned to org", constant: AuditActionTenantAssignedToOrg, expected: "tenant.assigned_to_org"},
		{name: "tenant unassigned from org", constant: AuditActionTenantUnassignedFromOrg, expected: "tenant.unassigned_from_org"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.Equal(t, tt.expected, tt.constant)
		})
	}
}

func TestAuditEmitterEmitTenantAssignedToOrg(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	err := emitter.EmitTenantAssignedToOrg(context.Background(), "tenant-1", "org-1", nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, AuditActionTenantAssignedToOrg, mock.lastAction)
	testutil.Equal(t, AuditResultSuccess, mock.lastResult)

	var metadata map[string]string
	testutil.NoError(t, json.Unmarshal(mock.lastMetadata, &metadata))
	testutil.Equal(t, "org-1", metadata["orgId"])
}

func TestAuditEmitterEmitTenantUnassignedFromOrg(t *testing.T) {
	mock := &mockAuditInserter{}
	emitter := NewAuditEmitterWithInserter(mock, testutil.DiscardLogger())

	err := emitter.EmitTenantUnassignedFromOrg(context.Background(), "tenant-1", "org-1", nil, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, AuditActionTenantUnassignedFromOrg, mock.lastAction)
	testutil.Equal(t, AuditResultSuccess, mock.lastResult)

	var metadata map[string]string
	testutil.NoError(t, json.Unmarshal(mock.lastMetadata, &metadata))
	testutil.Equal(t, "org-1", metadata["orgId"])
}
