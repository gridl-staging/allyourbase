import { useCallback, useMemo, useState } from "react";
import { AlertCircle, Loader2 } from "lucide-react";
import { createTenant } from "../api_tenants";
import { DEFAULT_CREATE_TENANT_ISOLATION_MODE, DEFAULT_CREATE_TENANT_PLAN_TIER } from "../types/tenants";
import type { DetailTab, TenantAuditQuery } from "../types/tenants";
import { useDraftFilters } from "../hooks/useDraftFilters";
import { useTenantDetailState, useTenantListState } from "./tenants-hooks";
import { useTenantManagementState } from "./tenant-management-hooks";
import {
  TenantAuditSection,
  TenantDetailHeader,
  TenantListPanel,
  TenantMaintenanceSection,
} from "./TenantsSections";
import { TenantInfoSection, TenantMembersSection } from "./TenantManagementSections";
import {
  type CreateTenantFormErrors,
  type CreateTenantFormValues,
  TenantCreateDialog,
} from "./TenantCreateDialog";

const TENANT_SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])$/;
const CREATE_TENANT_DUPLICATE_SLUG_ERROR = "A tenant with this slug already exists.";
const CREATE_TENANT_GENERIC_ERROR = "Unable to create tenant. Verify the values and try again.";
const DEFAULT_CREATE_TENANT_VALUES: CreateTenantFormValues = {
  name: "",
  slug: "",
  ownerUserId: "",
  isolationMode: DEFAULT_CREATE_TENANT_ISOLATION_MODE,
  planTier: DEFAULT_CREATE_TENANT_PLAN_TIER,
  region: "us-east-1",
};

const DEFAULT_AUDIT_FILTERS = {
  from: "",
  to: "",
  action: "",
  result: "",
  actorId: "",
};

function validateCreateTenant(values: CreateTenantFormValues): CreateTenantFormErrors {
  const errors: CreateTenantFormErrors = {};
  if (!values.name.trim()) {
    errors.name = "Tenant name is required";
  }
  if (!values.slug.trim()) {
    errors.slug = "Slug is required";
  } else if (!TENANT_SLUG_PATTERN.test(values.slug.trim())) {
    errors.slug = "Slug must match: lowercase letters, numbers, and hyphens";
  }
  return errors;
}

function getApiErrorStatus(error: unknown): number | null {
  if (!error || typeof error !== "object" || !("status" in error)) {
    return null;
  }
  return typeof error.status === "number" ? error.status : null;
}

function getCreateTenantSubmitError(error: unknown): string {
  if (getApiErrorStatus(error) === 409) {
    return CREATE_TENANT_DUPLICATE_SLUG_ERROR;
  }
  return CREATE_TENANT_GENERIC_ERROR;
}

export function Tenants() {
  const { listState, listQuery, nextPage, prevPage, refreshList } = useTenantListState();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<DetailTab>("info");
  const [isCreateDialogOpen, setCreateDialogOpen] = useState(false);
  const [createValues, setCreateValues] = useState<CreateTenantFormValues>(DEFAULT_CREATE_TENANT_VALUES);
  const [createErrors, setCreateErrors] = useState<CreateTenantFormErrors>({});
  const [createSubmitError, setCreateSubmitError] = useState<string | null>(null);
  const [isCreatingTenant, setCreatingTenant] = useState(false);

  const {
    draftValues: draftAuditFilters,
    appliedValues: appliedAuditFilters,
    setDraftValue: setDraftAuditFilter,
    applyValues: applyAuditFilters,
    resetValues: resetAuditFilters,
  } = useDraftFilters(DEFAULT_AUDIT_FILTERS);

  const auditQuery = useMemo<TenantAuditQuery>(
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

  const { detail, setDetail } = useTenantDetailState(selectedId, activeTab, auditQuery);
  const {
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
  } = useTenantManagementState({
    selectedId,
    detail,
    setDetail,
    refreshList,
  });

  const selectTenant = useCallback((id: string) => {
    setSelectedId(id);
    setActiveTab("info");
  }, []);

  const openCreateDialog = useCallback(() => {
    setCreateDialogOpen(true);
    setCreateSubmitError(null);
  }, []);

  const closeCreateDialog = useCallback(() => {
    if (isCreatingTenant) {
      return;
    }
    setCreateDialogOpen(false);
    setCreateErrors({});
    setCreateSubmitError(null);
  }, [isCreatingTenant]);

  const handleCreateFieldChange = useCallback((field: keyof CreateTenantFormValues, value: string) => {
    setCreateValues((current) => ({ ...current, [field]: value }));
    setCreateErrors((current) => {
      if (!current[field]) {
        return current;
      }
      const next = { ...current };
      delete next[field];
      return next;
    });
    setCreateSubmitError(null);
  }, []);

  const handleCreateTenant = useCallback(async () => {
    const nextErrors = validateCreateTenant(createValues);
    if (Object.keys(nextErrors).length > 0) {
      setCreateErrors(nextErrors);
      return;
    }

    setCreatingTenant(true);
    setCreateSubmitError(null);
    try {
      const createdTenant = await createTenant({
        name: createValues.name.trim(),
        slug: createValues.slug.trim(),
        ownerUserId: createValues.ownerUserId.trim(),
        isolationMode: createValues.isolationMode.trim(),
        planTier: createValues.planTier.trim(),
        region: createValues.region.trim(),
      });
      setCreateDialogOpen(false);
      setCreateValues(DEFAULT_CREATE_TENANT_VALUES);
      setCreateErrors({});
      setSelectedId(createdTenant.id);
      setActiveTab("info");
      setDetail((state) => ({ ...state, tenant: createdTenant, error: null }));
      refreshList();
    } catch (error) {
      setCreateSubmitError(getCreateTenantSubmitError(error));
    } finally {
      setCreatingTenant(false);
    }
  }, [createValues, setDetail, refreshList]);

  // Loading state
  if (listState.isLoading && listState.items.length === 0) {
    return (
      <div data-testid="tenants-view" className="flex-1 flex items-center justify-center p-8">
        <Loader2 className="w-6 h-6 animate-spin text-gray-400 mr-2" />
        <span className="text-gray-500 dark:text-gray-400">Loading tenants…</span>
      </div>
    );
  }

  // Error state
  if (listState.error && listState.items.length === 0) {
    return (
      <div data-testid="tenants-view" className="flex-1 flex items-center justify-center p-8">
        <AlertCircle className="w-5 h-5 text-red-400 mr-2" />
        <span className="text-red-600 dark:text-red-400">Failed to load tenants: {listState.error}</span>
      </div>
    );
  }

  return (
    <div data-testid="tenants-view" className="flex-1 flex overflow-hidden">
      <TenantListPanel
        items={listState.items}
        page={listQuery.page}
        totalPages={listState.totalPages}
        selectedId={selectedId}
        onSelect={selectTenant}
        onCreateTenant={openCreateDialog}
        onNextPage={nextPage}
        onPrevPage={prevPage}
      />
      <div className="flex-1 overflow-auto border-l border-gray-200 dark:border-gray-700">
        {detail.tenant ? (
          <div className="p-4">
            <TenantDetailHeader
              tenant={detail.tenant}
              activeTab={activeTab}
              onTabChange={setActiveTab}
              onSuspend={handleSuspend}
              onResume={handleResume}
              onDelete={handleDelete}
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
                {activeTab === "info" ? (
                  <TenantInfoSection
                    tenant={detail.tenant}
                    tenantNameDraft={tenantNameDraft}
                    tenantOrgMetadataDraft={tenantOrgMetadataDraft}
                    isSaving={isSavingTenantInfo}
                    error={tenantInfoError}
                    onTenantNameChange={handleTenantNameChange}
                    onTenantOrgMetadataChange={handleTenantOrgMetadataChange}
                    onSave={handleSaveTenantInfo}
                    onReset={handleResetTenantInfo}
                  />
                ) : activeTab === "members" ? (
                  <TenantMembersSection
                    members={detail.members}
                    roleDraftByUserId={roleDraftByUserId}
                    updatingUserId={updatingUserId}
                    addMemberUserId={newMemberUserId}
                    addMemberRole={newMemberRole}
                    isAddingMember={isAddingMember}
                    removingUserId={removingUserId}
                    actionError={memberActionError}
                    onRoleDraftChange={handleMemberRoleDraftChange}
                    onUpdateRole={handleUpdateMemberRole}
                    onAddMemberUserIdChange={handleNewMemberUserIdChange}
                    onAddMemberRoleChange={handleNewMemberRoleChange}
                    onAddMember={handleAddMember}
                    onRemoveMember={handleRemoveMember}
                  />
                ) : activeTab === "maintenance" ? (
                  <TenantMaintenanceSection
                    maintenance={detail.maintenance}
                    breaker={detail.breaker}
                    onEnableMaintenance={handleEnableMaintenance}
                    onDisableMaintenance={handleDisableMaintenance}
                    onResetBreaker={handleResetBreaker}
                  />
                ) : activeTab === "audit" ? (
                  <TenantAuditSection
                    items={detail.auditItems}
                    count={detail.auditCount}
                    filterValues={draftAuditFilters}
                    onFilterChange={setDraftAuditFilter}
                    onApplyFilters={applyAuditFilters}
                    onResetFilters={resetAuditFilters}
                  />
                ) : null}
              </>
            )}
          </div>
        ) : selectedId && detail.isLoading ? (
          <div className="flex items-center justify-center h-full gap-2 text-gray-500 dark:text-gray-400">
            <Loader2 className="w-5 h-5 animate-spin" />
            <span>Loading tenant details…</span>
          </div>
        ) : selectedId && detail.error ? (
          <div className="p-4">
            <div className="rounded border border-red-200 bg-red-50 text-red-700 px-3 py-2 text-sm">
              Failed to load tenant: {detail.error}
            </div>
          </div>
        ) : (
          <div className="flex items-center justify-center h-full text-gray-400 dark:text-gray-500">
            {listState.items.length === 0 ? "No tenants found" : "Select a tenant to view details"}
          </div>
        )}
      </div>
      <TenantCreateDialog
        isOpen={isCreateDialogOpen}
        values={createValues}
        errors={createErrors}
        submitError={createSubmitError}
        isSubmitting={isCreatingTenant}
        onChange={handleCreateFieldChange}
        onClose={closeCreateDialog}
        onSubmit={handleCreateTenant}
      />
    </div>
  );
}
