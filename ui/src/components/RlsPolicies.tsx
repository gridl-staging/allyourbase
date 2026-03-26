import { useState, useCallback, useEffect } from "react";
import {
  listRlsPolicies,
  getRlsStatus,
  createRlsPolicy,
  deleteRlsPolicy,
  enableRls,
  disableRls,
} from "../api";
import type { SchemaCache, Table, RlsPolicy, RlsTableStatus } from "../types";
import {
  Shield,
  ShieldCheck,
  ShieldOff,
  Plus,
  Trash2,
  AlertCircle,
  Loader2,
  Code,
} from "lucide-react";
import { cn } from "../lib/utils";
import { useAppToast } from "./ToastProvider";
import {
  RlsPolicyCreateModal,
  type CreatePolicyFormState,
} from "./RlsPolicyCreateModal";
import {
  RlsPolicyActionModals,
  type RlsPolicyModalState,
} from "./RlsPolicyActionModals";
import {
  generatePolicySql,
  type PolicyTemplate,
} from "./rls-helpers";

interface RlsPoliciesProps {
  schema: SchemaCache;
}

const initialCreatePolicyFormState: CreatePolicyFormState = {
  name: "",
  command: "ALL",
  usingExpression: "",
  withCheckExpression: "",
  isPermissive: true,
};

function buildDefaultSelectedTable(tables: Table[]): Table | null {
  if (tables.length === 0) {
    return null;
  }
  return tables[0];
}

function buildQualifiedTableName(table: Pick<Table, "schema" | "name">): string {
  return `${table.schema}.${table.name}`;
}

export function RlsPolicies({ schema }: RlsPoliciesProps) {
  const tables = Object.values(schema.tables)
    .filter((table) => table.kind === "table")
    .sort((left, right) => buildQualifiedTableName(left).localeCompare(buildQualifiedTableName(right)));

  const [selectedTable, setSelectedTable] = useState<Table | null>(buildDefaultSelectedTable(tables));
  const [policies, setPolicies] = useState<RlsPolicy[]>([]);
  const [rlsStatus, setRlsStatus] = useState<RlsTableStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [actionModal, setActionModal] = useState<RlsPolicyModalState>({ kind: "none" });
  const [toggling, setToggling] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [createPolicyFormState, setCreatePolicyFormState] = useState<CreatePolicyFormState>(
    initialCreatePolicyFormState,
  );
  const { addToast } = useAppToast();

  const resetCreatePolicyForm = useCallback(() => {
    setCreatePolicyFormState(initialCreatePolicyFormState);
  }, []);

  const updateCreatePolicyForm = useCallback((updates: Partial<CreatePolicyFormState>) => {
    setCreatePolicyFormState((previousFormState) => ({
      ...previousFormState,
      ...updates,
    }));
  }, []);

  const fetchData = useCallback(async () => {
    if (!selectedTable) {
      setLoading(false);
      return;
    }
    const selectedTableIdentifier = buildQualifiedTableName(selectedTable);

    setLoading(true);
    setError(null);

    try {
      const [listedPolicies, tableRlsStatus] = await Promise.all([
        listRlsPolicies(selectedTableIdentifier),
        getRlsStatus(selectedTableIdentifier),
      ]);
      setPolicies(listedPolicies);
      setRlsStatus(tableRlsStatus);
    } catch (fetchError) {
      setError(fetchError instanceof Error ? fetchError.message : String(fetchError));
    } finally {
      setLoading(false);
    }
  }, [selectedTable]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const handleToggleRls = useCallback(async () => {
    if (!selectedTable || !rlsStatus) {
      return;
    }
    const selectedTableIdentifier = buildQualifiedTableName(selectedTable);

    setToggling(true);

    try {
      if (rlsStatus.rlsEnabled) {
        await disableRls(selectedTableIdentifier);
        addToast("success", `RLS disabled on ${selectedTable.name}`);
      } else {
        await enableRls(selectedTableIdentifier);
        addToast("success", `RLS enabled on ${selectedTable.name}`);
      }
      await fetchData();
    } catch (toggleError) {
      addToast("error", toggleError instanceof Error ? toggleError.message : "Failed to toggle RLS");
    } finally {
      setToggling(false);
    }
  }, [selectedTable, rlsStatus, fetchData, addToast]);

  const handleOpenCreateModal = useCallback(() => {
    setIsCreateModalOpen(true);
  }, []);

  const handleCloseCreateModal = useCallback(() => {
    setIsCreateModalOpen(false);
    resetCreatePolicyForm();
  }, [resetCreatePolicyForm]);

  const handleCreatePolicy = useCallback(async () => {
    if (!selectedTable || !createPolicyFormState.name.trim()) {
      return;
    }

    setIsCreating(true);

    try {
      await createRlsPolicy({
        table: selectedTable.name,
        schema: selectedTable.schema,
        name: createPolicyFormState.name.trim(),
        command: createPolicyFormState.command,
        permissive: createPolicyFormState.isPermissive,
        using: createPolicyFormState.usingExpression.trim() || undefined,
        withCheck: createPolicyFormState.withCheckExpression.trim() || undefined,
      });

      addToast("success", `Policy "${createPolicyFormState.name}" created`);
      handleCloseCreateModal();
      await fetchData();
    } catch (createError) {
      addToast("error", createError instanceof Error ? createError.message : "Failed to create policy");
    } finally {
      setIsCreating(false);
    }
  }, [selectedTable, createPolicyFormState, fetchData, addToast, handleCloseCreateModal]);

  const handleConfirmDelete = useCallback(async () => {
    if (actionModal.kind !== "delete") {
      return;
    }

    setDeleting(true);

    try {
      const tableIdentifier = buildQualifiedTableName({
        schema: actionModal.policy.tableSchema,
        name: actionModal.policy.tableName,
      });
      await deleteRlsPolicy(tableIdentifier, actionModal.policy.policyName);
      addToast("success", `Policy "${actionModal.policy.policyName}" deleted`);
      setActionModal({ kind: "none" });
      await fetchData();
    } catch (deleteError) {
      addToast("error", deleteError instanceof Error ? deleteError.message : "Failed to delete policy");
    } finally {
      setDeleting(false);
    }
  }, [actionModal, fetchData, addToast]);

  const handleApplyTemplate = useCallback((template: PolicyTemplate) => {
    updateCreatePolicyForm({
      command: template.command,
      usingExpression: template.using,
      withCheckExpression: template.withCheck,
    });
  }, [updateCreatePolicyForm]);

  return (
    <div className="flex h-full">
      <div className="w-56 border-r bg-gray-50 dark:bg-gray-800 overflow-y-auto">
        <div className="px-3 py-2 border-b">
          <h2 className="text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
            Tables
          </h2>
        </div>

        {tables.map((table) => {
          const tableKey = buildQualifiedTableName(table);
          const isSelectedTable =
            selectedTable?.schema === table.schema && selectedTable?.name === table.name;

          return (
            <button
              key={tableKey}
              onClick={() => setSelectedTable(table)}
              className={cn(
                "w-full text-left px-3 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700",
                isSelectedTable && "bg-white dark:bg-gray-800 font-medium border-l-2 border-blue-500",
              )}
            >
              <span className="truncate">
                {table.schema !== "public" && (
                  <span className="text-gray-400 dark:text-gray-500">{table.schema}.</span>
                )}
                {table.name}
              </span>
            </button>
          );
        })}

        {tables.length === 0 && (
          <p className="px-3 py-4 text-xs text-gray-400 dark:text-gray-500 text-center">
            No tables found
          </p>
        )}
      </div>

      <div className="flex-1 overflow-auto">
        {!selectedTable ? (
          <div className="flex items-center justify-center h-full text-gray-400 dark:text-gray-500 text-sm">
            Select a table to manage RLS policies
          </div>
        ) : loading ? (
          <div className="flex items-center justify-center h-full text-gray-400 dark:text-gray-500 text-sm gap-2">
            <Loader2 className="w-4 h-4 animate-spin" />
            Loading policies...
          </div>
        ) : error ? (
          <div className="m-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
            <AlertCircle className="w-4 h-4 text-red-500 mt-0.5 shrink-0" />
            <div>
              <p className="text-sm text-red-700">{error}</p>
              <button
                onClick={fetchData}
                className="mt-2 text-xs text-red-600 hover:text-red-800 underline"
              >
                Retry
              </button>
            </div>
          </div>
        ) : (
          <div className="p-6">
            <div className="flex items-center justify-between mb-6">
              <div className="flex items-center gap-3">
                <h1 className="text-lg font-semibold">
                  {selectedTable.schema !== "public" && (
                    <span className="text-gray-400 dark:text-gray-500">{selectedTable.schema}.</span>
                  )}
                  {selectedTable.name}
                </h1>

                {rlsStatus && (
                  <span
                    className={cn(
                      "flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium",
                      rlsStatus.rlsEnabled
                        ? "bg-green-100 text-green-700"
                        : "bg-gray-100 dark:bg-gray-700 text-gray-500 dark:text-gray-400",
                    )}
                  >
                    {rlsStatus.rlsEnabled ? (
                      <ShieldCheck className="w-3 h-3" />
                    ) : (
                      <ShieldOff className="w-3 h-3" />
                    )}
                    {rlsStatus.rlsEnabled ? "RLS Enabled" : "RLS Disabled"}
                  </span>
                )}
              </div>

              <div className="flex items-center gap-2">
                <button
                  onClick={handleToggleRls}
                  disabled={toggling}
                  className={cn(
                    "px-3 py-1.5 text-xs font-medium rounded-lg border",
                    rlsStatus?.rlsEnabled
                      ? "text-red-600 border-red-200 hover:bg-red-50"
                      : "text-green-600 border-green-200 hover:bg-green-50",
                    toggling && "opacity-50",
                  )}
                >
                  {rlsStatus?.rlsEnabled ? "Disable RLS" : "Enable RLS"}
                </button>

                <button
                  onClick={handleOpenCreateModal}
                  className="px-3 py-1.5 text-xs font-medium rounded-lg bg-blue-600 text-white hover:bg-blue-700 flex items-center gap-1"
                >
                  <Plus className="w-3 h-3" />
                  Add Policy
                </button>
              </div>
            </div>

            {policies.length === 0 ? (
              <div className="text-center py-12 text-gray-400 dark:text-gray-500">
                <Shield className="w-8 h-8 mx-auto mb-2 opacity-50" />
                <p className="text-sm">No policies on this table</p>
                <button
                  onClick={handleOpenCreateModal}
                  className="mt-2 text-xs text-blue-600 hover:text-blue-800"
                >
                  Create your first policy
                </button>
              </div>
            ) : (
              <div className="space-y-3">
                {policies.map((policy) => (
                  <div
                    key={policy.policyName}
                    className="border rounded-lg p-4 hover:border-gray-300 dark:border-gray-600"
                  >
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-sm">{policy.policyName}</span>
                        <span
                          className={cn(
                            "px-1.5 py-0.5 rounded text-[10px] font-bold",
                            policy.command === "SELECT"
                              ? "bg-green-100 text-green-700"
                              : policy.command === "INSERT"
                                ? "bg-blue-100 text-blue-700"
                                : policy.command === "UPDATE"
                                  ? "bg-yellow-100 text-yellow-700"
                                  : policy.command === "DELETE"
                                    ? "bg-red-100 text-red-700"
                                    : "bg-purple-100 text-purple-700",
                          )}
                        >
                          {policy.command}
                        </span>
                        <span
                          className={cn(
                            "px-1.5 py-0.5 rounded text-[10px]",
                            policy.permissive === "PERMISSIVE"
                              ? "bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300"
                              : "bg-orange-100 text-orange-700",
                          )}
                        >
                          {policy.permissive}
                        </span>
                      </div>

                      <div className="flex items-center gap-1">
                        <button
                          onClick={() =>
                            setActionModal({ kind: "sql-preview", sql: generatePolicySql(policy) })
                          }
                          className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300 rounded"
                          title="View SQL"
                          aria-label="View SQL"
                        >
                          <Code className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={() => setActionModal({ kind: "delete", policy })}
                          className="p-1 text-gray-400 dark:text-gray-500 hover:text-red-500 rounded"
                          title="Delete policy"
                          aria-label="Delete policy"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </div>

                    {policy.roles.length > 0 && (
                      <div className="text-xs text-gray-500 dark:text-gray-400 mb-1">
                        Roles: {policy.roles.join(", ")}
                      </div>
                    )}

                    {policy.usingExpr && (
                      <div className="text-xs mb-1">
                        <span className="text-gray-400 dark:text-gray-500">USING:</span>{" "}
                        <code className="font-mono text-gray-600 dark:text-gray-300 bg-gray-50 dark:bg-gray-800 px-1 rounded">
                          {policy.usingExpr}
                        </code>
                      </div>
                    )}

                    {policy.withCheckExpr && (
                      <div className="text-xs">
                        <span className="text-gray-400 dark:text-gray-500">WITH CHECK:</span>{" "}
                        <code className="font-mono text-gray-600 dark:text-gray-300 bg-gray-50 dark:bg-gray-800 px-1 rounded">
                          {policy.withCheckExpr}
                        </code>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      <RlsPolicyCreateModal
        isOpen={isCreateModalOpen}
        selectedTable={selectedTable}
        formState={createPolicyFormState}
        isSubmitting={isCreating}
        onClose={handleCloseCreateModal}
        onSubmit={handleCreatePolicy}
        onApplyTemplate={handleApplyTemplate}
        onNameChange={(name) => updateCreatePolicyForm({ name })}
        onCommandChange={(command) => updateCreatePolicyForm({ command })}
        onPermissiveChange={(isPermissive) => updateCreatePolicyForm({ isPermissive })}
        onUsingExpressionChange={(usingExpression) => updateCreatePolicyForm({ usingExpression })}
        onWithCheckExpressionChange={(withCheckExpression) =>
          updateCreatePolicyForm({ withCheckExpression })
        }
      />

      <RlsPolicyActionModals
        modal={actionModal}
        isDeleting={deleting}
        onClose={() => setActionModal({ kind: "none" })}
        onConfirmDelete={handleConfirmDelete}
      />
    </div>
  );
}
