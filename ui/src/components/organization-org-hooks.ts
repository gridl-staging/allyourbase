/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/organization-org-hooks.ts.
 */
import { useCallback, useEffect, useState } from "react";
import {
  addOrgMember,
  createOrg,
  deleteOrg,
  removeOrgMember,
  updateOrg,
  updateOrgMemberRole,
} from "../api_orgs";
import type { OrgMemberRole } from "../types/organizations";
import {
  applyCreatedOrgMember,
  applyDeletedOrgMember,
  applyUpdatedOrgMemberRole,
} from "./organization-detail-updaters";
import { normalizeOrgRole, toErrorMessage, type UseOrgManagementArgs } from "./organization-management-shared";

type OrgCoreArgs = Pick<
  UseOrgManagementArgs,
  "selectedId" | "detail" | "setDetail" | "refreshList" | "onOrgDeleted" | "onOrgCreated"
>;

type OrgMembershipArgs = Pick<UseOrgManagementArgs, "selectedId" | "detail" | "setDetail">;

type OrgCreateArgs = Pick<UseOrgManagementArgs, "selectedId" | "refreshList" | "onOrgCreated">;

type OrgInfoArgs = Pick<UseOrgManagementArgs, "selectedId" | "detail" | "setDetail" | "refreshList" | "onOrgDeleted">;

/**
 * TODO: Document useOrganizationCreateControls.
 */
function useOrganizationCreateControls({ selectedId, refreshList, onOrgCreated }: OrgCreateArgs) {
  const [newOrgName, setNewOrgName] = useState("");
  const [newOrgSlug, setNewOrgSlug] = useState("");
  const [newOrgPlanTier, setNewOrgPlanTier] = useState("free");
  const [newOrgParentId, setNewOrgParentId] = useState("");
  const [createOrgError, setCreateOrgError] = useState<string | null>(null);
  const [isCreatingOrg, setCreatingOrg] = useState(false);

  const resetNewOrgInputs = useCallback(() => {
    setNewOrgName("");
    setNewOrgSlug("");
    setNewOrgPlanTier("free");
    setNewOrgParentId("");
  }, []);

  useEffect(() => {
    setCreateOrgError(null);
    resetNewOrgInputs();
  }, [resetNewOrgInputs, selectedId]);

  const handleCreateOrg = useCallback(async () => {
    if (!newOrgName.trim() || !newOrgSlug.trim()) {
      setCreateOrgError("Organization name and slug are required");
      return;
    }
    setCreatingOrg(true);
    setCreateOrgError(null);
    try {
      const created = await createOrg({
        name: newOrgName.trim(),
        slug: newOrgSlug.trim(),
        planTier: newOrgPlanTier.trim() || "free",
        parentOrgId: newOrgParentId.trim() || undefined,
      });
      resetNewOrgInputs();
      refreshList();
      onOrgCreated(created.id);
    } catch (error) {
      setCreateOrgError(toErrorMessage(error));
    } finally {
      setCreatingOrg(false);
    }
  }, [newOrgName, newOrgParentId, newOrgPlanTier, newOrgSlug, onOrgCreated, refreshList, resetNewOrgInputs]);

  return {
    newOrgName,
    setNewOrgName,
    newOrgSlug,
    setNewOrgSlug,
    newOrgPlanTier,
    setNewOrgPlanTier,
    newOrgParentId,
    setNewOrgParentId,
    createOrgError,
    isCreatingOrg,
    handleCreateOrg,
  };
}

/**
 * TODO: Document useOrganizationInfoControls.
 */
function useOrganizationInfoControls({ selectedId, detail, setDetail, refreshList, onOrgDeleted }: OrgInfoArgs) {
  const [orgNameDraft, setOrgNameDraft] = useState("");
  const [orgSlugDraft, setOrgSlugDraft] = useState("");
  const [orgParentIdDraft, setOrgParentIdDraft] = useState("");
  const [orgInfoError, setOrgInfoError] = useState<string | null>(null);
  const [isSavingOrgInfo, setSavingOrgInfo] = useState(false);
  const [isDeletingOrg, setDeletingOrg] = useState(false);

  useEffect(() => {
    setOrgInfoError(null);
    setSavingOrgInfo(false);
    setDeletingOrg(false);
  }, [selectedId]);

  useEffect(() => {
    const org = detail.org;
    if (!org) {
      setOrgNameDraft("");
      setOrgSlugDraft("");
      setOrgParentIdDraft("");
      return;
    }
    setOrgNameDraft(org.name);
    setOrgSlugDraft(org.slug);
    setOrgParentIdDraft(org.parentOrgId ?? "");
    setOrgInfoError(null);
  }, [detail.org]);

  const handleSaveOrgInfo = useCallback(async () => {
    if (!selectedId || !detail.org) return;
    if (!orgNameDraft.trim()) {
      setOrgInfoError("Organization name is required");
      return;
    }
    setSavingOrgInfo(true);
    setOrgInfoError(null);
    try {
      const updated = await updateOrg(selectedId, {
        name: orgNameDraft.trim(),
        slug: orgSlugDraft.trim() || undefined,
        parentOrgId: orgParentIdDraft.trim() === "" ? "" : orgParentIdDraft.trim(),
      });
      setDetail((state) => ({
        ...state,
        org: state.org ? { ...state.org, ...updated } : state.org,
        error: null,
      }));
      refreshList();
    } catch (error) {
      setOrgInfoError(toErrorMessage(error));
    } finally {
      setSavingOrgInfo(false);
    }
  }, [detail.org, orgNameDraft, orgParentIdDraft, orgSlugDraft, refreshList, selectedId, setDetail]);

  const handleResetOrgInfo = useCallback(() => {
    const org = detail.org;
    if (!org) return;
    setOrgNameDraft(org.name);
    setOrgSlugDraft(org.slug);
    setOrgParentIdDraft(org.parentOrgId ?? "");
    setOrgInfoError(null);
  }, [detail.org]);

  const handleDeleteOrg = useCallback(async () => {
    if (!selectedId) return;
    setDeletingOrg(true);
    setDetail((state) => ({ ...state, error: null }));
    try {
      await deleteOrg(selectedId);
      onOrgDeleted();
      refreshList();
    } catch (error) {
      setDetail((state) => ({ ...state, error: toErrorMessage(error) }));
    } finally {
      setDeletingOrg(false);
    }
  }, [onOrgDeleted, refreshList, selectedId, setDetail]);

  return {
    orgNameDraft,
    setOrgNameDraft,
    orgSlugDraft,
    setOrgSlugDraft,
    orgParentIdDraft,
    setOrgParentIdDraft,
    orgInfoError,
    isSavingOrgInfo,
    isDeletingOrg,
    handleSaveOrgInfo,
    handleResetOrgInfo,
    handleDeleteOrg,
  };
}

/**
 * TODO: Document useOrganizationControls.
 */
export function useOrganizationControls({
  selectedId,
  detail,
  setDetail,
  refreshList,
  onOrgDeleted,
  onOrgCreated,
}: OrgCoreArgs) {
  const createControls = useOrganizationCreateControls({ selectedId, refreshList, onOrgCreated });
  const infoControls = useOrganizationInfoControls({ selectedId, detail, setDetail, refreshList, onOrgDeleted });

  return {
    ...createControls,
    ...infoControls,
  };
}

/**
 * TODO: Document useOrgMembershipControls.
 */
export function useOrgMembershipControls({ selectedId, detail, setDetail }: OrgMembershipArgs) {
  const [orgRoleDraftByUserId, setOrgRoleDraftByUserId] = useState<Partial<Record<string, OrgMemberRole>>>({});
  const [orgUpdatingUserId, setOrgUpdatingUserId] = useState<string | null>(null);
  const [newOrgMemberUserId, setNewOrgMemberUserId] = useState("");
  const [newOrgMemberRole, setNewOrgMemberRole] = useState<OrgMemberRole>("member");
  const [orgMemberActionError, setOrgMemberActionError] = useState<string | null>(null);
  const [isAddingOrgMember, setAddingOrgMember] = useState(false);
  const [removingOrgMemberUserId, setRemovingOrgMemberUserId] = useState<string | null>(null);

  useEffect(() => {
    setOrgRoleDraftByUserId({});
    setOrgUpdatingUserId(null);
    setNewOrgMemberUserId("");
    setNewOrgMemberRole("member");
    setOrgMemberActionError(null);
  }, [selectedId]);

  const handleAddOrgMember = useCallback(async () => {
    if (!selectedId) return;
    if (!newOrgMemberUserId.trim()) {
      setOrgMemberActionError("Member user ID is required");
      return;
    }
    setAddingOrgMember(true);
    setOrgMemberActionError(null);
    try {
      const created = await addOrgMember(selectedId, newOrgMemberUserId.trim(), newOrgMemberRole);
      setDetail((state) => applyCreatedOrgMember(state, created));
      setNewOrgMemberUserId("");
      setNewOrgMemberRole("member");
    } catch (error) {
      setOrgMemberActionError(toErrorMessage(error));
    } finally {
      setAddingOrgMember(false);
    }
  }, [newOrgMemberRole, newOrgMemberUserId, selectedId, setDetail]);

  const handleRemoveOrgMember = useCallback(
    async (userId: string) => {
      if (!selectedId) return;
      setRemovingOrgMemberUserId(userId);
      setOrgMemberActionError(null);
      try {
        await removeOrgMember(selectedId, userId);
        setDetail((state) => applyDeletedOrgMember(state, userId));
      } catch (error) {
        setOrgMemberActionError(toErrorMessage(error));
      } finally {
        setRemovingOrgMemberUserId(null);
      }
    },
    [selectedId, setDetail],
  );

  const handleOrgRoleDraftChange = useCallback((userId: string, nextRole: OrgMemberRole) => {
    setOrgRoleDraftByUserId((current) => ({ ...current, [userId]: nextRole }));
  }, []);

  const handleUpdateOrgMemberRole = useCallback(async (userId: string) => {
    if (!selectedId) return;
    const member = detail.members.find((current) => current.userId === userId);
    if (!member) return;
    const nextRole = orgRoleDraftByUserId[userId] ?? normalizeOrgRole(member.role);

    setOrgUpdatingUserId(userId);
    setOrgMemberActionError(null);
    try {
      const updated = await updateOrgMemberRole(selectedId, userId, nextRole);
      const normalized = normalizeOrgRole(updated.role);
      setOrgRoleDraftByUserId((current) => ({ ...current, [userId]: normalized }));
      setDetail((state) => applyUpdatedOrgMemberRole(state, userId, normalized));
    } catch (error) {
      setOrgMemberActionError(toErrorMessage(error));
    } finally {
      setOrgUpdatingUserId(null);
    }
  }, [detail.members, orgRoleDraftByUserId, selectedId, setDetail]);

  return {
    orgRoleDraftByUserId,
    orgUpdatingUserId,
    newOrgMemberUserId,
    setNewOrgMemberUserId,
    newOrgMemberRole,
    setNewOrgMemberRole,
    orgMemberActionError,
    isAddingOrgMember,
    removingOrgMemberUserId,
    handleAddOrgMember,
    handleRemoveOrgMember,
    handleOrgRoleDraftChange,
    handleUpdateOrgMemberRole,
  };
}
