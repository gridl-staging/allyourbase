/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/tenant-management-hooks.ts.
 */
import { useCallback, useEffect, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import {
  addTenantMember,
  deleteTenant,
  disableMaintenance,
  enableMaintenance,
  removeTenantMember,
  resetBreaker,
  resumeTenant,
  suspendTenant,
  updateTenant,
  updateTenantMemberRole,
} from "../api_tenants";
import type { MemberRole, Tenant, TenantMembership } from "../types/tenants";
import { MEMBER_ROLE_VALUES } from "../types/tenants";
import type { DetailState } from "./tenants-hooks";

interface UseTenantManagementStateArgs {
  selectedId: string | null;
  detail: DetailState;
  setDetail: Dispatch<SetStateAction<DetailState>>;
  refreshList: () => void;
}

function isMemberRole(value: string): value is MemberRole {
  return MEMBER_ROLE_VALUES.includes(value as MemberRole);
}

function normalizeMemberRole(value: string): MemberRole {
  return isMemberRole(value) ? value : "member";
}

function formatOrgMetadataDraft(orgMetadata: unknown): string {
  if (orgMetadata == null) {
    return "{}";
  }

  try {
    return JSON.stringify(orgMetadata, null, 2);
  } catch {
    return "{}";
  }
}

function parseOrgMetadataDraft(rawValue: string): { value?: unknown; error?: string } {
  const trimmedValue = rawValue.trim();
  if (!trimmedValue) {
    return { value: {} };
  }

  try {
    return { value: JSON.parse(trimmedValue) };
  } catch {
    return { error: "Org metadata must be valid JSON" };
  }
}

/**
 * TODO: Document useTenantManagementState.
 */
export function useTenantManagementState({
  selectedId,
  detail,
  setDetail,
  refreshList,
}: UseTenantManagementStateArgs) {
  const [roleDraftByUserId, setRoleDraftByUserId] = useState<Partial<Record<string, MemberRole>>>({});
  const [updatingUserId, setUpdatingUserId] = useState<string | null>(null);
  const [tenantNameDraft, setTenantNameDraft] = useState("");
  const [tenantOrgMetadataDraft, setTenantOrgMetadataDraft] = useState("{}");
  const [tenantInfoError, setTenantInfoError] = useState<string | null>(null);
  const [isSavingTenantInfo, setSavingTenantInfo] = useState(false);
  const [newMemberUserId, setNewMemberUserId] = useState("");
  const [newMemberRole, setNewMemberRole] = useState<MemberRole>("member");
  const [memberActionError, setMemberActionError] = useState<string | null>(null);
  const [isAddingMember, setAddingMember] = useState(false);
  const [removingUserId, setRemovingUserId] = useState<string | null>(null);

  useEffect(() => {
    setRoleDraftByUserId({});
    setUpdatingUserId(null);
    setNewMemberUserId("");
    setNewMemberRole("member");
    setMemberActionError(null);
    setTenantInfoError(null);
  }, [selectedId]);

  useEffect(() => {
    const tenant = detail.tenant;
    if (!tenant) {
      setTenantNameDraft("");
      setTenantOrgMetadataDraft("{}");
      return;
    }

    setTenantNameDraft(tenant.name);
    setTenantOrgMetadataDraft(formatOrgMetadataDraft(tenant.orgMetadata));
    setTenantInfoError(null);
  }, [detail.tenant]);

  const handleTenantNameChange = useCallback((value: string) => {
    setTenantNameDraft(value);
    setTenantInfoError(null);
  }, []);

  const handleTenantOrgMetadataChange = useCallback((value: string) => {
    setTenantOrgMetadataDraft(value);
    setTenantInfoError(null);
  }, []);

  const resetTenantInfoDrafts = useCallback((tenant: Tenant | null) => {
    if (!tenant) {
      setTenantNameDraft("");
      setTenantOrgMetadataDraft("{}");
      return;
    }

    setTenantNameDraft(tenant.name);
    setTenantOrgMetadataDraft(formatOrgMetadataDraft(tenant.orgMetadata));
  }, []);

  const handleSaveTenantInfo = useCallback(async () => {
    if (!selectedId || !detail.tenant) {
      return;
    }
    if (!tenantNameDraft.trim()) {
      setTenantInfoError("Tenant name is required");
      return;
    }

    const parsedOrgMetadata = parseOrgMetadataDraft(tenantOrgMetadataDraft);
    if (parsedOrgMetadata.error) {
      setTenantInfoError(parsedOrgMetadata.error);
      return;
    }

    setSavingTenantInfo(true);
    setTenantInfoError(null);
    try {
      const updatedTenant = await updateTenant(selectedId, {
        name: tenantNameDraft.trim(),
        orgMetadata: parsedOrgMetadata.value,
      });
      setDetail((state) => ({ ...state, tenant: updatedTenant, error: null }));
      resetTenantInfoDrafts(updatedTenant);
      refreshList();
    } catch (error) {
      setTenantInfoError(String((error as Error)?.message ?? error));
    } finally {
      setSavingTenantInfo(false);
    }
  }, [
    detail.tenant,
    refreshList,
    resetTenantInfoDrafts,
    selectedId,
    setDetail,
    tenantNameDraft,
    tenantOrgMetadataDraft,
  ]);

  const handleResetTenantInfo = useCallback(() => {
    resetTenantInfoDrafts(detail.tenant);
    setTenantInfoError(null);
  }, [detail.tenant, resetTenantInfoDrafts]);

  const handleNewMemberUserIdChange = useCallback((value: string) => {
    setNewMemberUserId(value);
    setMemberActionError(null);
  }, []);

  const handleNewMemberRoleChange = useCallback((value: MemberRole) => {
    setNewMemberRole(value);
    setMemberActionError(null);
  }, []);

  const handleAddMember = useCallback(async () => {
    if (!selectedId) {
      return;
    }
    if (!newMemberUserId.trim()) {
      setMemberActionError("Member user ID is required");
      return;
    }

    setAddingMember(true);
    setMemberActionError(null);
    try {
      const createdMember = await addTenantMember(selectedId, newMemberUserId.trim(), newMemberRole);
      setDetail((state) => ({
        ...state,
        members: [...state.members, createdMember],
        error: null,
      }));
      setRoleDraftByUserId((current) => ({ ...current, [createdMember.userId]: newMemberRole }));
      setNewMemberUserId("");
      setNewMemberRole("member");
    } catch (error) {
      setMemberActionError(String((error as Error)?.message ?? error));
    } finally {
      setAddingMember(false);
    }
  }, [newMemberRole, newMemberUserId, selectedId, setDetail]);

  const handleRemoveMember = useCallback(
    async (userId: string) => {
      if (!selectedId) {
        return;
      }

      setRemovingUserId(userId);
      setMemberActionError(null);
      try {
        await removeTenantMember(selectedId, userId);
        setDetail((state) => ({
          ...state,
          members: state.members.filter((entry) => entry.userId !== userId),
          error: null,
        }));
        setRoleDraftByUserId((current) => {
          const next = { ...current };
          delete next[userId];
          return next;
        });
      } catch (error) {
        setMemberActionError(String((error as Error)?.message ?? error));
      } finally {
        setRemovingUserId(null);
      }
    },
    [selectedId, setDetail],
  );

  const handleSuspend = useCallback(async () => {
    if (!selectedId) return;
    const updated = await suspendTenant(selectedId);
    setDetail((state) => ({ ...state, tenant: updated }));
    refreshList();
  }, [refreshList, selectedId, setDetail]);

  const handleResume = useCallback(async () => {
    if (!selectedId) return;
    const updated = await resumeTenant(selectedId);
    setDetail((state) => ({ ...state, tenant: updated }));
    refreshList();
  }, [refreshList, selectedId, setDetail]);

  const handleDelete = useCallback(async () => {
    if (!selectedId) return;
    const updated = await deleteTenant(selectedId);
    setDetail((state) => ({ ...state, tenant: updated, error: null }));
    refreshList();
  }, [refreshList, selectedId, setDetail]);

  const handleEnableMaintenance = useCallback(async () => {
    if (!selectedId) return;
    const updated = await enableMaintenance(selectedId, "Enabled via admin dashboard");
    setDetail((state) => ({ ...state, maintenance: updated }));
  }, [selectedId, setDetail]);

  const handleDisableMaintenance = useCallback(async () => {
    if (!selectedId) return;
    const updated = await disableMaintenance(selectedId);
    setDetail((state) => ({ ...state, maintenance: updated }));
  }, [selectedId, setDetail]);

  const handleResetBreaker = useCallback(async () => {
    if (!selectedId) return;
    const updated = await resetBreaker(selectedId);
    setDetail((state) => ({ ...state, breaker: updated }));
  }, [selectedId, setDetail]);

  const handleMemberRoleDraftChange = useCallback((userId: string, nextRole: MemberRole) => {
    setRoleDraftByUserId((current) => ({ ...current, [userId]: nextRole }));
  }, []);

  const handleUpdateMemberRole = useCallback(
    async (userId: string) => {
      if (!selectedId) return;
      const member = detail.members.find((entry) => entry.userId === userId);
      if (!member) return;
      const nextRole = roleDraftByUserId[userId] ?? normalizeMemberRole(member.role);

      let previousMembers: TenantMembership[] = [];
      setUpdatingUserId(userId);
      setDetail((state) => {
        previousMembers = state.members;
        return {
          ...state,
          members: state.members.map((entry) =>
            entry.userId === userId ? { ...entry, role: nextRole } : entry,
          ),
          error: null,
        };
      });

      try {
        const updatedMember = await updateTenantMemberRole(selectedId, userId, nextRole);
        const normalizedRole = normalizeMemberRole(updatedMember.role);
        setRoleDraftByUserId((current) => ({ ...current, [userId]: normalizedRole }));
        setDetail((state) => ({
          ...state,
          members: state.members.map((entry) =>
            entry.userId === userId ? { ...entry, role: normalizedRole } : entry,
          ),
          error: null,
        }));
      } catch (error) {
        setDetail((state) => ({
          ...state,
          members: previousMembers,
          error: String((error as Error)?.message ?? error),
        }));
      } finally {
        setUpdatingUserId(null);
      }
    },
    [detail.members, roleDraftByUserId, selectedId, setDetail],
  );

  return {
    roleDraftByUserId,
    updatingUserId,
    tenantNameDraft,
    tenantOrgMetadataDraft,
    tenantInfoError,
    isSavingTenantInfo,
    newMemberUserId,
    newMemberRole,
    memberActionError,
    isAddingMember,
    removingUserId,
    handleTenantNameChange,
    handleTenantOrgMetadataChange,
    handleSaveTenantInfo,
    handleResetTenantInfo,
    handleNewMemberUserIdChange,
    handleNewMemberRoleChange,
    handleAddMember,
    handleRemoveMember,
    handleSuspend,
    handleResume,
    handleDelete,
    handleEnableMaintenance,
    handleDisableMaintenance,
    handleResetBreaker,
    handleMemberRoleDraftChange,
    handleUpdateMemberRole,
  };
}
