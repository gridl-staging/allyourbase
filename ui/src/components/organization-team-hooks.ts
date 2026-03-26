/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/organization-team-hooks.ts.
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  addTeamMember,
  createTeam,
  deleteTeam,
  removeTeamMember,
  updateTeam,
  updateTeamMemberRole,
} from "../api_orgs";
import type { TeamMemberRole } from "../types/organizations";
import {
  applyCreatedTeam,
  applyCreatedTeamMember,
  applyDeletedTeam,
  applyDeletedTeamMember,
  applyUpdatedTeam,
  applyUpdatedTeamMemberRole,
} from "./organization-detail-updaters";
import {
  normalizeTeamRole,
  teamMemberDraftKey,
  toErrorMessage,
  type UseOrgManagementArgs,
} from "./organization-management-shared";

type TeamCoreArgs = Pick<UseOrgManagementArgs, "selectedId" | "selectedTeamId" | "detail" | "setDetail" | "onTeamDeleted">;
type TeamMembershipArgs = Pick<UseOrgManagementArgs, "selectedId" | "selectedTeamId" | "detail" | "setDetail">;

type TeamCreateArgs = Pick<UseOrgManagementArgs, "selectedId" | "setDetail">;
type TeamInfoArgs = Pick<UseOrgManagementArgs, "selectedId" | "selectedTeamId" | "detail" | "setDetail" | "onTeamDeleted">;

/**
 * TODO: Document useTeamCreateControls.
 */
function useTeamCreateControls({ selectedId, setDetail }: TeamCreateArgs) {
  const [newTeamName, setNewTeamName] = useState("");
  const [newTeamSlug, setNewTeamSlug] = useState("");
  const [teamCreateError, setTeamCreateError] = useState<string | null>(null);
  const [isCreatingTeam, setCreatingTeam] = useState(false);

  useEffect(() => {
    setNewTeamName("");
    setNewTeamSlug("");
    setTeamCreateError(null);
  }, [selectedId]);

  const handleCreateTeam = useCallback(async () => {
    if (!selectedId) return;
    if (!newTeamName.trim() || !newTeamSlug.trim()) {
      setTeamCreateError("Team name and slug are required");
      return;
    }
    setCreatingTeam(true);
    setTeamCreateError(null);
    try {
      const created = await createTeam(selectedId, {
        name: newTeamName.trim(),
        slug: newTeamSlug.trim(),
      });
      setDetail((state) => applyCreatedTeam(state, created));
      setNewTeamName("");
      setNewTeamSlug("");
    } catch (error) {
      setTeamCreateError(toErrorMessage(error));
    } finally {
      setCreatingTeam(false);
    }
  }, [newTeamName, newTeamSlug, selectedId, setDetail]);

  return {
    newTeamName,
    setNewTeamName,
    newTeamSlug,
    setNewTeamSlug,
    teamCreateError,
    isCreatingTeam,
    handleCreateTeam,
  };
}

/**
 * TODO: Document useTeamInfoControls.
 */
function useTeamInfoControls({ selectedId, selectedTeamId, detail, setDetail, onTeamDeleted }: TeamInfoArgs) {
  const [teamNameDraft, setTeamNameDraft] = useState("");
  const [teamSlugDraft, setTeamSlugDraft] = useState("");
  const [teamInfoError, setTeamInfoError] = useState<string | null>(null);
  const [isSavingTeamInfo, setSavingTeamInfo] = useState(false);
  const [isDeletingTeam, setDeletingTeam] = useState(false);

  useEffect(() => {
    const selectedTeam = detail.teams.find((team) => team.id === selectedTeamId);
    setTeamNameDraft(selectedTeam?.name ?? "");
    setTeamSlugDraft(selectedTeam?.slug ?? "");
    setTeamInfoError(null);
    setSavingTeamInfo(false);
    setDeletingTeam(false);
  }, [detail.teams, selectedTeamId]);

  const handleSaveTeamInfo = useCallback(async () => {
    if (!selectedId || !selectedTeamId) return;
    if (!teamNameDraft.trim() || !teamSlugDraft.trim()) {
      setTeamInfoError("Team name and slug are required");
      return;
    }
    setSavingTeamInfo(true);
    setTeamInfoError(null);
    try {
      const updated = await updateTeam(selectedId, selectedTeamId, {
        name: teamNameDraft.trim(),
        slug: teamSlugDraft.trim(),
      });
      setDetail((state) => applyUpdatedTeam(state, selectedTeamId, updated));
    } catch (error) {
      setTeamInfoError(toErrorMessage(error));
    } finally {
      setSavingTeamInfo(false);
    }
  }, [selectedId, selectedTeamId, setDetail, teamNameDraft, teamSlugDraft]);

  const handleDeleteTeam = useCallback(async () => {
    if (!selectedId || !selectedTeamId) return;
    setDeletingTeam(true);
    setTeamInfoError(null);
    try {
      await deleteTeam(selectedId, selectedTeamId);
      setDetail((state) => applyDeletedTeam(state, selectedTeamId));
      onTeamDeleted();
    } catch (error) {
      setTeamInfoError(toErrorMessage(error));
    } finally {
      setDeletingTeam(false);
    }
  }, [onTeamDeleted, selectedId, selectedTeamId, setDetail]);

  return {
    teamNameDraft,
    setTeamNameDraft,
    teamSlugDraft,
    setTeamSlugDraft,
    teamInfoError,
    isSavingTeamInfo,
    isDeletingTeam,
    handleSaveTeamInfo,
    handleDeleteTeam,
  };
}

export function useTeamControls({ selectedId, selectedTeamId, detail, setDetail, onTeamDeleted }: TeamCoreArgs) {
  const createControls = useTeamCreateControls({ selectedId, setDetail });
  const infoControls = useTeamInfoControls({ selectedId, selectedTeamId, detail, setDetail, onTeamDeleted });

  return {
    ...createControls,
    ...infoControls,
  };
}

/**
 * TODO: Document useTeamMembershipDraftControls.
 */
function useTeamMembershipDraftControls(selectedId: string | null, selectedTeamId: string | null) {
  const [teamRoleDraftsByMemberKey, setTeamRoleDraftsByMemberKey] =
    useState<Partial<Record<string, TeamMemberRole>>>({});
  const [teamUpdatingUserId, setTeamUpdatingUserId] = useState<string | null>(null);
  const [newTeamMemberUserId, setNewTeamMemberUserId] = useState("");
  const [newTeamMemberRole, setNewTeamMemberRole] = useState<TeamMemberRole>("member");
  const [teamMemberActionError, setTeamMemberActionError] = useState<string | null>(null);
  const [isAddingTeamMember, setAddingTeamMember] = useState(false);
  const [removingTeamMemberUserId, setRemovingTeamMemberUserId] = useState<string | null>(null);

  useEffect(() => {
    setTeamRoleDraftsByMemberKey({});
    setTeamUpdatingUserId(null);
    setNewTeamMemberUserId("");
    setNewTeamMemberRole("member");
    setTeamMemberActionError(null);
    setRemovingTeamMemberUserId(null);
  }, [selectedId]);

  useEffect(() => {
    setTeamUpdatingUserId(null);
    setNewTeamMemberUserId("");
    setNewTeamMemberRole("member");
    setTeamMemberActionError(null);
    setRemovingTeamMemberUserId(null);
  }, [selectedTeamId]);

  return {
    teamRoleDraftsByMemberKey,
    setTeamRoleDraftsByMemberKey,
    teamUpdatingUserId,
    setTeamUpdatingUserId,
    newTeamMemberUserId,
    setNewTeamMemberUserId,
    newTeamMemberRole,
    setNewTeamMemberRole,
    teamMemberActionError,
    setTeamMemberActionError,
    isAddingTeamMember,
    setAddingTeamMember,
    removingTeamMemberUserId,
    setRemovingTeamMemberUserId,
  };
}

type TeamMembershipDraftControls = ReturnType<typeof useTeamMembershipDraftControls>;
type TeamMembershipActionArgs = TeamMembershipArgs & { draftControls: TeamMembershipDraftControls };

/**
 * TODO: Document useTeamMembershipActionHandlers.
 */
function useTeamMembershipActionHandlers({
  selectedId,
  selectedTeamId,
  detail,
  setDetail,
  draftControls,
}: TeamMembershipActionArgs) {
  const handleAddTeamMember = useCallback(async () => {
    if (!selectedId || !selectedTeamId) return;
    if (!draftControls.newTeamMemberUserId.trim()) {
      draftControls.setTeamMemberActionError("Member user ID is required");
      return;
    }

    draftControls.setAddingTeamMember(true);
    draftControls.setTeamMemberActionError(null);
    try {
      const created = await addTeamMember(
        selectedId,
        selectedTeamId,
        draftControls.newTeamMemberUserId.trim(),
        draftControls.newTeamMemberRole,
      );
      setDetail((state) => applyCreatedTeamMember(state, selectedTeamId, created));
      draftControls.setNewTeamMemberUserId("");
      draftControls.setNewTeamMemberRole("member");
    } catch (error) {
      draftControls.setTeamMemberActionError(toErrorMessage(error));
    } finally {
      draftControls.setAddingTeamMember(false);
    }
  }, [draftControls, selectedId, selectedTeamId, setDetail]);

  const handleRemoveTeamMember = useCallback(async (userId: string) => {
    if (!selectedId || !selectedTeamId) return;
    draftControls.setRemovingTeamMemberUserId(userId);
    draftControls.setTeamMemberActionError(null);
    try {
      await removeTeamMember(selectedId, selectedTeamId, userId);
      setDetail((state) => applyDeletedTeamMember(state, selectedTeamId, userId));
    } catch (error) {
      draftControls.setTeamMemberActionError(toErrorMessage(error));
    } finally {
      draftControls.setRemovingTeamMemberUserId(null);
    }
  }, [draftControls, selectedId, selectedTeamId, setDetail]);

  const handleTeamRoleDraftChange = useCallback((userId: string, nextRole: TeamMemberRole) => {
    if (!selectedTeamId) return;
    const draftKey = teamMemberDraftKey(selectedTeamId, userId);
    draftControls.setTeamRoleDraftsByMemberKey((current) => ({ ...current, [draftKey]: nextRole }));
  }, [draftControls, selectedTeamId]);

  const handleUpdateTeamMemberRole = useCallback(async (userId: string) => {
    if (!selectedId || !selectedTeamId) return;
    const members = detail.teamMembersByTeamId[selectedTeamId] ?? [];
    const member = members.find((current) => current.userId === userId);
    if (!member) return;

    const draftKey = teamMemberDraftKey(selectedTeamId, userId);
    const nextRole = draftControls.teamRoleDraftsByMemberKey[draftKey] ?? normalizeTeamRole(member.role);

    draftControls.setTeamUpdatingUserId(userId);
    draftControls.setTeamMemberActionError(null);
    try {
      const updated = await updateTeamMemberRole(selectedId, selectedTeamId, userId, nextRole);
      const normalized = normalizeTeamRole(updated.role);
      draftControls.setTeamRoleDraftsByMemberKey((current) => ({ ...current, [draftKey]: normalized }));
      setDetail((state) => applyUpdatedTeamMemberRole(state, selectedTeamId, userId, normalized));
    } catch (error) {
      draftControls.setTeamMemberActionError(toErrorMessage(error));
    } finally {
      draftControls.setTeamUpdatingUserId(null);
    }
  }, [detail.teamMembersByTeamId, draftControls, selectedId, selectedTeamId, setDetail]);

  return {
    handleAddTeamMember,
    handleRemoveTeamMember,
    handleTeamRoleDraftChange,
    handleUpdateTeamMemberRole,
  };
}

/**
 * TODO: Document useTeamMembershipControls.
 */
export function useTeamMembershipControls({ selectedId, selectedTeamId, detail, setDetail }: TeamMembershipArgs) {
  const draftControls = useTeamMembershipDraftControls(selectedId, selectedTeamId);
  const handlers = useTeamMembershipActionHandlers({
    selectedId,
    selectedTeamId,
    detail,
    setDetail,
    draftControls,
  });

  const teamRoleDraftByUserId = useMemo(
    () =>
      selectedTeamId == null
        ? {}
        : Object.fromEntries(
            Object.entries(draftControls.teamRoleDraftsByMemberKey)
              .filter(([key]) => key.startsWith(`${selectedTeamId}:`))
              .map(([key, role]) => [key.slice(selectedTeamId.length + 1), role]),
          ),
    [draftControls.teamRoleDraftsByMemberKey, selectedTeamId],
  );

  return {
    teamRoleDraftByUserId,
    teamUpdatingUserId: draftControls.teamUpdatingUserId,
    newTeamMemberUserId: draftControls.newTeamMemberUserId,
    setNewTeamMemberUserId: draftControls.setNewTeamMemberUserId,
    newTeamMemberRole: draftControls.newTeamMemberRole,
    setNewTeamMemberRole: draftControls.setNewTeamMemberRole,
    teamMemberActionError: draftControls.teamMemberActionError,
    isAddingTeamMember: draftControls.isAddingTeamMember,
    removingTeamMemberUserId: draftControls.removingTeamMemberUserId,
    handleAddTeamMember: handlers.handleAddTeamMember,
    handleRemoveTeamMember: handlers.handleRemoveTeamMember,
    handleTeamRoleDraftChange: handlers.handleTeamRoleDraftChange,
    handleUpdateTeamMemberRole: handlers.handleUpdateTeamMemberRole,
  };
}
