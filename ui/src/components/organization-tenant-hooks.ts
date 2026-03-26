/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/organization-tenant-hooks.ts.
 */
import { useCallback, useEffect, useState } from "react";
import { assignTenantToOrg, fetchOrgTenants, unassignTenantFromOrg } from "../api_orgs";
import { applyTenantAssignment, applyTenantUnassignment } from "./organization-detail-updaters";
import { toErrorMessage, type UseOrgManagementArgs } from "./organization-management-shared";

type TenantArgs = Pick<UseOrgManagementArgs, "selectedId" | "setDetail">;

/**
 * TODO: Document useTenantAssignmentControls.
 */
export function useTenantAssignmentControls({ selectedId, setDetail }: TenantArgs) {
  const [assignTenantId, setAssignTenantId] = useState("");
  const [tenantActionError, setTenantActionError] = useState<string | null>(null);
  const [isAssigningTenant, setAssigningTenant] = useState(false);

  useEffect(() => {
    setAssignTenantId("");
    setTenantActionError(null);
  }, [selectedId]);

  const handleAssignTenant = useCallback(async () => {
    if (!selectedId) return;
    if (!assignTenantId.trim()) {
      setTenantActionError("Tenant ID is required");
      return;
    }
    setAssigningTenant(true);
    setTenantActionError(null);
    try {
      await assignTenantToOrg(selectedId, assignTenantId.trim());
      const response = await fetchOrgTenants(selectedId);
      setDetail((state) => applyTenantAssignment(state, response.items));
      setAssignTenantId("");
    } catch (error) {
      setTenantActionError(toErrorMessage(error));
    } finally {
      setAssigningTenant(false);
    }
  }, [assignTenantId, selectedId, setDetail]);

  const handleUnassignTenant = useCallback(async (tenantId: string) => {
    if (!selectedId) return;
    setTenantActionError(null);
    try {
      await unassignTenantFromOrg(selectedId, tenantId);
      setDetail((state) => applyTenantUnassignment(state, tenantId));
    } catch (error) {
      setTenantActionError(toErrorMessage(error));
    }
  }, [selectedId, setDetail]);

  return {
    assignTenantId,
    setAssignTenantId,
    tenantActionError,
    isAssigningTenant,
    handleAssignTenant,
    handleUnassignTenant,
  };
}
