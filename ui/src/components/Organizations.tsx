import { useCallback, useMemo, useState } from "react";
import { AlertCircle, Loader2 } from "lucide-react";
import { useDraftFilters } from "../hooks/useDraftFilters";
import type { OrgAuditQuery, OrgDetailTab, OrgUsagePeriod, OrgUsageQuery } from "../types/organizations";
import { useOrgManagementState } from "./organization-management-hooks";
import { useOrgDetailState, useOrgListState, type OrgDetailState } from "./organizations-hooks";
import { OrgInfoSection, OrgMembersSection, OrgTenantsSection } from "./OrganizationManagementSections";
import { OrgTeamsSection } from "./OrganizationTeamSections";
import { OrgAuditSection, OrgDetailHeader, OrgListPanel, OrgUsageSection } from "./OrganizationsSections";

const DEFAULT_AUDIT_FILTERS = {
  from: "",
  to: "",
  action: "",
  result: "",
  actorId: "",
};

const DEFAULT_USAGE_FILTERS = {
  period: "month",
  from: "",
  to: "",
};

interface OrganizationFilterState {
  auditQuery: OrgAuditQuery;
  usageQuery: OrgUsageQuery;
  draftAuditFilters: Record<string, string>;
  setDraftAuditFilter: (name: string, value: string) => void;
  applyAuditFilters: (values: Record<string, string>) => void;
  resetAuditFilters: () => void;
  draftUsageFilters: Record<string, string>;
  setDraftUsageFilter: (name: string, value: string) => void;
  applyUsageFilters: (values: Record<string, string>) => void;
  resetUsageFilters: () => void;
}

interface OrganizationSelectionState {
  selectedId: string | null;
  activeTab: OrgDetailTab;
  selectedTeamId: string | null;
  selectOrg: (id: string) => void;
  handleTabChange: (tab: OrgDetailTab) => void;
  handleTeamSelect: (teamId: string) => void;
  handleOrgDeleted: () => void;
  handleOrgCreated: (id: string) => void;
  handleTeamDeleted: () => void;
}

type OrgManagementState = ReturnType<typeof useOrgManagementState>;

function normalizeUsagePeriod(value: string): OrgUsagePeriod {
  return value === "day" || value === "week" || value === "month" ? value : "month";
}

function useOrganizationFilters(): OrganizationFilterState {
  const {
    draftValues: draftAuditFilters,
    appliedValues: appliedAuditFilters,
    setDraftValue: setDraftAuditFilter,
    applyValues: applyAuditFilters,
    resetValues: resetAuditFilters,
  } = useDraftFilters(DEFAULT_AUDIT_FILTERS);
  const {
    draftValues: draftUsageFilters,
    appliedValues: appliedUsageFilters,
    setDraftValue: setDraftUsageFilter,
    applyValues: applyUsageFilters,
    resetValues: resetUsageFilters,
  } = useDraftFilters(DEFAULT_USAGE_FILTERS);

  const auditQuery = useMemo<OrgAuditQuery>(
    () => ({
      limit: 50,
      offset: 0,
      from: appliedAuditFilters.from || undefined,
      to: appliedAuditFilters.to || undefined,
      action: appliedAuditFilters.action || undefined,
      result: appliedAuditFilters.result || undefined,
      actorId: appliedAuditFilters.actorId || undefined,
    }),
    [appliedAuditFilters],
  );
  const usageQuery = useMemo<OrgUsageQuery>(
    () => ({
      period: normalizeUsagePeriod(appliedUsageFilters.period),
      from: appliedUsageFilters.from || null,
      to: appliedUsageFilters.to || null,
    }),
    [appliedUsageFilters],
  );

  return {
    auditQuery,
    usageQuery,
    draftAuditFilters,
    setDraftAuditFilter,
    applyAuditFilters,
    resetAuditFilters,
    draftUsageFilters,
    setDraftUsageFilter,
    applyUsageFilters,
    resetUsageFilters,
  };
}

function useOrganizationSelection(): OrganizationSelectionState {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<OrgDetailTab>("info");
  const [selectedTeamId, setSelectedTeamId] = useState<string | null>(null);

  const resetSelection = useCallback((id: string | null) => {
    setSelectedId(id);
    setActiveTab("info");
    setSelectedTeamId(null);
  }, []);
  const selectOrg = useCallback((id: string) => resetSelection(id), [resetSelection]);
  const handleOrgDeleted = useCallback(() => resetSelection(null), [resetSelection]);
  const handleOrgCreated = useCallback((id: string) => resetSelection(id), [resetSelection]);
  const handleTeamDeleted = useCallback(() => setSelectedTeamId(null), []);
  const handleTeamSelect = useCallback((teamId: string) => setSelectedTeamId(teamId), []);
  const handleTabChange = useCallback((tab: OrgDetailTab) => {
    setActiveTab(tab);
    if (tab !== "teams") {
      setSelectedTeamId(null);
    }
  }, []);

  return {
    selectedId,
    activeTab,
    selectedTeamId,
    selectOrg,
    handleTabChange,
    handleTeamSelect,
    handleOrgDeleted,
    handleOrgCreated,
    handleTeamDeleted,
  };
}

interface OrganizationTabContentProps {
  activeTab: OrgDetailTab;
  detail: OrgDetailState;
  selectedTeamId: string | null;
  onSelectTeam: (teamId: string) => void;
  management: OrgManagementState;
  filters: OrganizationFilterState;
}

function OrganizationInfoTab({ detail, management }: Pick<OrganizationTabContentProps, "detail" | "management">) {
  return (
    <OrgInfoSection
      orgId={detail.org!.id}
      slug={detail.org!.slug}
      planTier={detail.org!.planTier}
      parentOrgId={detail.org!.parentOrgId}
      createdAt={detail.org!.createdAt}
      updatedAt={detail.org!.updatedAt}
      orgNameDraft={management.orgNameDraft}
      orgSlugDraft={management.orgSlugDraft}
      orgParentIdDraft={management.orgParentIdDraft}
      isSaving={management.isSavingOrgInfo}
      error={management.orgInfoError}
      onNameChange={management.setOrgNameDraft}
      onSlugChange={management.setOrgSlugDraft}
      onParentIdChange={management.setOrgParentIdDraft}
      onSave={management.handleSaveOrgInfo}
      onReset={management.handleResetOrgInfo}
    />
  );
}

function OrganizationMembersTab({ detail, management }: Pick<OrganizationTabContentProps, "detail" | "management">) {
  return (
    <OrgMembersSection
      members={detail.members}
      roleDraftByUserId={management.orgRoleDraftByUserId}
      updatingUserId={management.orgUpdatingUserId}
      addMemberUserId={management.newOrgMemberUserId}
      addMemberRole={management.newOrgMemberRole}
      isAddingMember={management.isAddingOrgMember}
      removingUserId={management.removingOrgMemberUserId}
      actionError={management.orgMemberActionError}
      onRoleDraftChange={management.handleOrgRoleDraftChange}
      onUpdateRole={management.handleUpdateOrgMemberRole}
      onAddMemberUserIdChange={management.setNewOrgMemberUserId}
      onAddMemberRoleChange={management.setNewOrgMemberRole}
      onAddMember={management.handleAddOrgMember}
      onRemoveMember={management.handleRemoveOrgMember}
    />
  );
}

function OrganizationTeamsTab({
  detail,
  selectedTeamId,
  onSelectTeam,
  management,
}: Pick<OrganizationTabContentProps, "detail" | "selectedTeamId" | "onSelectTeam" | "management">) {
  return (
    <OrgTeamsSection
      teams={detail.teams}
      selectedTeamId={selectedTeamId}
      teamMembers={detail.teamMembersByTeamId[selectedTeamId ?? ""] ?? []}
      onSelectTeam={onSelectTeam}
      newTeamName={management.newTeamName}
      newTeamSlug={management.newTeamSlug}
      isCreatingTeam={management.isCreatingTeam}
      teamCreateError={management.teamCreateError}
      onTeamNameChange={management.setNewTeamName}
      onTeamSlugChange={management.setNewTeamSlug}
      onCreateTeam={management.handleCreateTeam}
      teamNameDraft={management.teamNameDraft}
      teamSlugDraft={management.teamSlugDraft}
      teamInfoError={management.teamInfoError}
      isSavingTeamInfo={management.isSavingTeamInfo}
      isDeletingTeam={management.isDeletingTeam}
      onSelectedTeamNameChange={management.setTeamNameDraft}
      onSelectedTeamSlugChange={management.setTeamSlugDraft}
      onSaveTeam={management.handleSaveTeamInfo}
      onDeleteTeam={management.handleDeleteTeam}
      teamRoleDraftByUserId={management.teamRoleDraftByUserId}
      teamUpdatingUserId={management.teamUpdatingUserId}
      newTeamMemberUserId={management.newTeamMemberUserId}
      newTeamMemberRole={management.newTeamMemberRole}
      isAddingTeamMember={management.isAddingTeamMember}
      removingTeamMemberUserId={management.removingTeamMemberUserId}
      teamMemberActionError={management.teamMemberActionError}
      onTeamRoleDraftChange={management.handleTeamRoleDraftChange}
      onUpdateTeamMemberRole={management.handleUpdateTeamMemberRole}
      onTeamMemberUserIdChange={management.setNewTeamMemberUserId}
      onTeamMemberRoleChange={management.setNewTeamMemberRole}
      onAddTeamMember={management.handleAddTeamMember}
      onRemoveTeamMember={management.handleRemoveTeamMember}
    />
  );
}

function OrganizationTenantsTab({ detail, management }: Pick<OrganizationTabContentProps, "detail" | "management">) {
  return (
    <OrgTenantsSection
      tenants={detail.tenants}
      assignTenantId={management.assignTenantId}
      isAssigning={management.isAssigningTenant}
      actionError={management.tenantActionError}
      onTenantIdChange={management.setAssignTenantId}
      onAssign={management.handleAssignTenant}
      onUnassign={management.handleUnassignTenant}
    />
  );
}

function OrganizationTabContent({
  activeTab,
  detail,
  selectedTeamId,
  onSelectTeam,
  management,
  filters,
}: OrganizationTabContentProps) {
  switch (activeTab) {
    case "info":
      return <OrganizationInfoTab detail={detail} management={management} />;
    case "members":
      return <OrganizationMembersTab detail={detail} management={management} />;
    case "teams":
      return (
        <OrganizationTeamsTab
          detail={detail}
          selectedTeamId={selectedTeamId}
          onSelectTeam={onSelectTeam}
          management={management}
        />
      );
    case "tenants":
      return <OrganizationTenantsTab detail={detail} management={management} />;
    case "usage":
      return (
        <OrgUsageSection
          summary={detail.usage}
          filterValues={filters.draftUsageFilters}
          onFilterChange={filters.setDraftUsageFilter}
          onApplyFilters={filters.applyUsageFilters}
          onResetFilters={filters.resetUsageFilters}
        />
      );
    case "audit":
      return (
        <OrgAuditSection
          items={detail.auditItems}
          count={detail.auditCount}
          filterValues={filters.draftAuditFilters}
          onFilterChange={filters.setDraftAuditFilter}
          onApplyFilters={filters.applyAuditFilters}
          onResetFilters={filters.resetAuditFilters}
        />
      );
    default:
      return null;
  }
}

interface OrganizationDetailPaneProps {
  listItemCount: number;
  selectedId: string | null;
  activeTab: OrgDetailTab;
  detail: OrgDetailState;
  selectedTeamId: string | null;
  onTabChange: (tab: OrgDetailTab) => void;
  onSelectTeam: (teamId: string) => void;
  management: OrgManagementState;
  filters: OrganizationFilterState;
}

function OrganizationDetailPane({
  listItemCount,
  selectedId,
  activeTab,
  detail,
  selectedTeamId,
  onTabChange,
  onSelectTeam,
  management,
  filters,
}: OrganizationDetailPaneProps) {
  if (!detail.org) {
    if (selectedId && detail.isLoading) {
      return (
        <div className="flex items-center justify-center h-full gap-2 text-gray-500 dark:text-gray-400">
          <Loader2 className="w-5 h-5 animate-spin" />
          <span>Loading organization details…</span>
        </div>
      );
    }
    if (selectedId && detail.error) {
      return (
        <div className="p-4">
          <div className="rounded border border-red-200 bg-red-50 text-red-700 px-3 py-2 text-sm">
            Failed to load organization: {detail.error}
          </div>
        </div>
      );
    }
    return (
      <div className="flex items-center justify-center h-full text-gray-400 dark:text-gray-500">
        {listItemCount === 0 ? "No organizations found" : "Select an organization to view details"}
      </div>
    );
  }

  return (
    <div className="p-4">
      <OrgDetailHeader
        org={detail.org}
        activeTab={activeTab}
        onTabChange={onTabChange}
        onDelete={management.handleDeleteOrg}
        isDeleting={management.isDeletingOrg}
      />
      {detail.isLoading ? (
        <div className="flex items-center justify-center py-8">
          <Loader2 className="w-5 h-5 animate-spin text-gray-400" />
        </div>
      ) : (
        <>
          {detail.error && (
            <div className="mb-3 rounded border border-red-200 bg-red-50 text-red-700 px-3 py-2 text-sm">
              {detail.error}
            </div>
          )}
          <OrganizationTabContent
            activeTab={activeTab}
            detail={detail}
            selectedTeamId={selectedTeamId}
            onSelectTeam={onSelectTeam}
            management={management}
            filters={filters}
          />
        </>
      )}
    </div>
  );
}

export function Organizations() {
  const { listState, refreshList } = useOrgListState();
  const filters = useOrganizationFilters();
  const selection = useOrganizationSelection();
  const { detail, setDetail } = useOrgDetailState(
    selection.selectedId,
    selection.activeTab,
    filters.auditQuery,
    filters.usageQuery,
    selection.selectedTeamId,
  );
  const management = useOrgManagementState({
    selectedId: selection.selectedId,
    selectedTeamId: selection.selectedTeamId,
    detail,
    setDetail,
    refreshList,
    onOrgDeleted: selection.handleOrgDeleted,
    onOrgCreated: selection.handleOrgCreated,
    onTeamDeleted: selection.handleTeamDeleted,
  });

  if (listState.isLoading && listState.items.length === 0) {
    return (
      <div data-testid="organizations-view" className="flex-1 flex items-center justify-center p-8">
        <Loader2 className="w-6 h-6 animate-spin text-gray-400 mr-2" />
        <span className="text-gray-500 dark:text-gray-400">Loading organizations…</span>
      </div>
    );
  }
  if (listState.error && listState.items.length === 0) {
    return (
      <div data-testid="organizations-view" className="flex-1 flex items-center justify-center p-8">
        <AlertCircle className="w-5 h-5 text-red-400 mr-2" />
        <span className="text-red-600 dark:text-red-400">Failed to load organizations: {listState.error}</span>
      </div>
    );
  }

  return (
    <div data-testid="organizations-view" className="flex-1 flex overflow-hidden">
      <OrgListPanel
        items={listState.items}
        selectedId={selection.selectedId}
        onSelect={selection.selectOrg}
        newOrgName={management.newOrgName}
        newOrgSlug={management.newOrgSlug}
        newOrgPlanTier={management.newOrgPlanTier}
        newOrgParentId={management.newOrgParentId}
        isCreatingOrg={management.isCreatingOrg}
        createOrgError={management.createOrgError}
        onNewOrgNameChange={management.setNewOrgName}
        onNewOrgSlugChange={management.setNewOrgSlug}
        onNewOrgPlanTierChange={management.setNewOrgPlanTier}
        onNewOrgParentIdChange={management.setNewOrgParentId}
        onCreateOrg={management.handleCreateOrg}
      />
      <div className="flex-1 overflow-auto border-l border-gray-200 dark:border-gray-700">
        <OrganizationDetailPane
          listItemCount={listState.items.length}
          selectedId={selection.selectedId}
          activeTab={selection.activeTab}
          detail={detail}
          selectedTeamId={selection.selectedTeamId}
          onTabChange={selection.handleTabChange}
          onSelectTeam={selection.handleTeamSelect}
          management={management}
          filters={filters}
        />
      </div>
    </div>
  );
}
