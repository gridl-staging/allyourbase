/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/organization-management-hooks.ts.
 */
import { useOrgMembershipControls, useOrganizationControls } from "./organization-org-hooks";
import { useTeamControls, useTeamMembershipControls } from "./organization-team-hooks";
import { useTenantAssignmentControls } from "./organization-tenant-hooks";
import type { UseOrgManagementArgs } from "./organization-management-shared";

/**
 * TODO: Document useOrgManagementState.
 */
export function useOrgManagementState({
  selectedId,
  selectedTeamId,
  detail,
  setDetail,
  refreshList,
  onOrgDeleted,
  onOrgCreated,
  onTeamDeleted,
}: UseOrgManagementArgs) {
  const organizationControls = useOrganizationControls({
    selectedId,
    detail,
    setDetail,
    refreshList,
    onOrgDeleted,
    onOrgCreated,
  });
  const orgMembershipControls = useOrgMembershipControls({
    selectedId,
    detail,
    setDetail,
  });
  const teamControls = useTeamControls({
    selectedId,
    selectedTeamId,
    detail,
    setDetail,
    onTeamDeleted,
  });
  const teamMembershipControls = useTeamMembershipControls({
    selectedId,
    selectedTeamId,
    detail,
    setDetail,
  });
  const tenantAssignmentControls = useTenantAssignmentControls({
    selectedId,
    setDetail,
  });

  return {
    ...organizationControls,
    ...orgMembershipControls,
    ...teamControls,
    ...teamMembershipControls,
    ...tenantAssignmentControls,
  };
}
