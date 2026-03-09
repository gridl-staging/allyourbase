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
  ChevronRight,
} from "lucide-react";
import { cn } from "../lib/utils";
import { useToast, ToastContainer } from "./Toast";

const COMMANDS = ["ALL", "SELECT", "INSERT", "UPDATE", "DELETE"] as const;

interface PolicyTemplate {
  name: string;
  description: string;
  command: string;
  using: string;
  withCheck: string;
}

const TEMPLATES: PolicyTemplate[] = [
  {
    name: "Owner only",
    description: "Users can only access their own rows",
    command: "ALL",
    using: "(user_id = current_setting('app.user_id')::uuid)",
    withCheck: "(user_id = current_setting('app.user_id')::uuid)",
  },
  {
    name: "Public read, owner write",
    description: "Anyone can read, only owner can modify",
    command: "SELECT",
    using: "true",
    withCheck: "",
  },
  {
    name: "Role-based access",
    description: "Only authenticated role can access",
    command: "ALL",
    using: "(current_setting('app.role', true) = 'authenticated')",
    withCheck: "(current_setting('app.role', true) = 'authenticated')",
  },
  {
    name: "Tenant isolation",
    description: "Rows filtered by tenant_id session variable",
    command: "ALL",
    using: "(tenant_id = current_setting('app.tenant_id')::uuid)",
    withCheck: "(tenant_id = current_setting('app.tenant_id')::uuid)",
  },
];

interface RlsPoliciesProps {
  schema: SchemaCache;
}

type Modal =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "delete"; policy: RlsPolicy }
  | { kind: "sql-preview"; sql: string };

export function RlsPolicies({ schema }: RlsPoliciesProps) {
  const tables = Object.values(schema.tables)
    .filter((t) => t.kind === "table")
    .sort((a, b) => `${a.schema}.${a.name}`.localeCompare(`${b.schema}.${b.name}`));

  const [selectedTable, setSelectedTable] = useState<Table | null>(
    tables.length > 0 ? tables[0] : null,
  );
  const [policies, setPolicies] = useState<RlsPolicy[]>([]);
  const [rlsStatus, setRlsStatus] = useState<RlsTableStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [modal, setModal] = useState<Modal>({ kind: "none" });
  const [toggling, setToggling] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const { toasts, addToast, removeToast } = useToast();

  // Create form state
  const [createName, setCreateName] = useState("");
  const [createCommand, setCreateCommand] = useState("ALL");
  const [createUsing, setCreateUsing] = useState("");
  const [createWithCheck, setCreateWithCheck] = useState("");
  const [createPermissive, setCreatePermissive] = useState(true);
  const [creating, setCreating] = useState(false);

  const fetchData = useCallback(async () => {
    if (!selectedTable) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const [pols, status] = await Promise.all([
        listRlsPolicies(selectedTable.name),
        getRlsStatus(selectedTable.name),
      ]);
      setPolicies(pols);
      setRlsStatus(status);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [selectedTable]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const handleToggleRls = useCallback(async () => {
    if (!selectedTable || !rlsStatus) return;
    setToggling(true);
    try {
      if (rlsStatus.rlsEnabled) {
        await disableRls(selectedTable.name);
        addToast("success", `RLS disabled on ${selectedTable.name}`);
      } else {
        await enableRls(selectedTable.name);
        addToast("success", `RLS enabled on ${selectedTable.name}`);
      }
      await fetchData();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to toggle RLS");
    } finally {
      setToggling(false);
    }
  }, [selectedTable, rlsStatus, fetchData, addToast]);

  const handleCreate = useCallback(async () => {
    if (!selectedTable || !createName.trim()) return;
    setCreating(true);
    try {
      await createRlsPolicy({
        table: selectedTable.name,
        schema: selectedTable.schema,
        name: createName.trim(),
        command: createCommand,
        permissive: createPermissive,
        using: createUsing.trim() || undefined,
        withCheck: createWithCheck.trim() || undefined,
      });
      addToast("success", `Policy "${createName}" created`);
      setModal({ kind: "none" });
      setCreateName("");
      setCreateCommand("ALL");
      setCreateUsing("");
      setCreateWithCheck("");
      setCreatePermissive(true);
      await fetchData();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to create policy");
    } finally {
      setCreating(false);
    }
  }, [selectedTable, createName, createCommand, createUsing, createWithCheck, createPermissive, fetchData, addToast]);

  const handleDelete = useCallback(async () => {
    if (modal.kind !== "delete") return;
    setDeleting(true);
    try {
      await deleteRlsPolicy(modal.policy.tableName, modal.policy.policyName);
      addToast("success", `Policy "${modal.policy.policyName}" deleted`);
      setModal({ kind: "none" });
      await fetchData();
    } catch (err) {
      addToast("error", err instanceof Error ? err.message : "Failed to delete policy");
    } finally {
      setDeleting(false);
    }
  }, [modal, fetchData, addToast]);

  const applyTemplate = useCallback((tmpl: PolicyTemplate) => {
    setCreateCommand(tmpl.command);
    setCreateUsing(tmpl.using);
    setCreateWithCheck(tmpl.withCheck);
  }, []);

  const generatePolicySql = useCallback((pol: RlsPolicy): string => {
    let sql = `CREATE POLICY "${pol.policyName}" ON "${pol.tableSchema}"."${pol.tableName}"`;
    if (pol.permissive === "RESTRICTIVE") sql += "\n  AS RESTRICTIVE";
    sql += `\n  FOR ${pol.command}`;
    if (pol.roles.length > 0) {
      sql += `\n  TO ${pol.roles.join(", ")}`;
    }
    if (pol.usingExpr) sql += `\n  USING (${pol.usingExpr})`;
    if (pol.withCheckExpr) sql += `\n  WITH CHECK (${pol.withCheckExpr})`;
    return sql + ";";
  }, []);

  return (
    <div className="flex h-full">
      {/* Table selector sidebar */}
      <div className="w-56 border-r bg-gray-50 overflow-y-auto">
        <div className="px-3 py-2 border-b">
          <h2 className="text-xs font-medium text-gray-500 uppercase tracking-wider">
            Tables
          </h2>
        </div>
        {tables.map((t) => {
          const key = `${t.schema}.${t.name}`;
          const isSelected = selectedTable?.schema === t.schema && selectedTable?.name === t.name;
          return (
            <button
              key={key}
              onClick={() => setSelectedTable(t)}
              className={cn(
                "w-full text-left px-3 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100",
                isSelected && "bg-white font-medium border-l-2 border-blue-500",
              )}
            >
              <span className="truncate">
                {t.schema !== "public" && (
                  <span className="text-gray-400">{t.schema}.</span>
                )}
                {t.name}
              </span>
            </button>
          );
        })}
        {tables.length === 0 && (
          <p className="px-3 py-4 text-xs text-gray-400 text-center">
            No tables found
          </p>
        )}
      </div>

      {/* Main content */}
      <div className="flex-1 overflow-auto">
        {!selectedTable ? (
          <div className="flex items-center justify-center h-full text-gray-400 text-sm">
            Select a table to manage RLS policies
          </div>
        ) : loading ? (
          <div className="flex items-center justify-center h-full text-gray-400 text-sm gap-2">
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
            {/* Header with RLS toggle */}
            <div className="flex items-center justify-between mb-6">
              <div className="flex items-center gap-3">
                <h1 className="text-lg font-semibold">
                  {selectedTable.schema !== "public" && (
                    <span className="text-gray-400">{selectedTable.schema}.</span>
                  )}
                  {selectedTable.name}
                </h1>
                {rlsStatus && (
                  <span
                    className={cn(
                      "flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium",
                      rlsStatus.rlsEnabled
                        ? "bg-green-100 text-green-700"
                        : "bg-gray-100 text-gray-500",
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
                  onClick={() => setModal({ kind: "create" })}
                  className="px-3 py-1.5 text-xs font-medium rounded-lg bg-blue-600 text-white hover:bg-blue-700 flex items-center gap-1"
                >
                  <Plus className="w-3 h-3" />
                  Add Policy
                </button>
              </div>
            </div>

            {/* Policies list */}
            {policies.length === 0 ? (
              <div className="text-center py-12 text-gray-400">
                <Shield className="w-8 h-8 mx-auto mb-2 opacity-50" />
                <p className="text-sm">No policies on this table</p>
                <button
                  onClick={() => setModal({ kind: "create" })}
                  className="mt-2 text-xs text-blue-600 hover:text-blue-800"
                >
                  Create your first policy
                </button>
              </div>
            ) : (
              <div className="space-y-3">
                {policies.map((pol) => (
                  <div
                    key={pol.policyName}
                    className="border rounded-lg p-4 hover:border-gray-300"
                  >
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-sm">{pol.policyName}</span>
                        <span
                          className={cn(
                            "px-1.5 py-0.5 rounded text-[10px] font-bold",
                            pol.command === "SELECT"
                              ? "bg-green-100 text-green-700"
                              : pol.command === "INSERT"
                                ? "bg-blue-100 text-blue-700"
                                : pol.command === "UPDATE"
                                  ? "bg-yellow-100 text-yellow-700"
                                  : pol.command === "DELETE"
                                    ? "bg-red-100 text-red-700"
                                    : "bg-purple-100 text-purple-700",
                          )}
                        >
                          {pol.command}
                        </span>
                        <span
                          className={cn(
                            "px-1.5 py-0.5 rounded text-[10px]",
                            pol.permissive === "PERMISSIVE"
                              ? "bg-gray-100 text-gray-600"
                              : "bg-orange-100 text-orange-700",
                          )}
                        >
                          {pol.permissive}
                        </span>
                      </div>
                      <div className="flex items-center gap-1">
                        <button
                          onClick={() =>
                            setModal({ kind: "sql-preview", sql: generatePolicySql(pol) })
                          }
                          className="p-1 text-gray-400 hover:text-gray-600 rounded"
                          title="View SQL"
                          aria-label="View SQL"
                        >
                          <Code className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={() => setModal({ kind: "delete", policy: pol })}
                          className="p-1 text-gray-400 hover:text-red-500 rounded"
                          title="Delete policy"
                          aria-label="Delete policy"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </div>
                    {pol.roles.length > 0 && (
                      <div className="text-xs text-gray-500 mb-1">
                        Roles: {pol.roles.join(", ")}
                      </div>
                    )}
                    {pol.usingExpr && (
                      <div className="text-xs mb-1">
                        <span className="text-gray-400">USING:</span>{" "}
                        <code className="font-mono text-gray-600 bg-gray-50 px-1 rounded">
                          {pol.usingExpr}
                        </code>
                      </div>
                    )}
                    {pol.withCheckExpr && (
                      <div className="text-xs">
                        <span className="text-gray-400">WITH CHECK:</span>{" "}
                        <code className="font-mono text-gray-600 bg-gray-50 px-1 rounded">
                          {pol.withCheckExpr}
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

      {/* Create Policy Modal */}
      {modal.kind === "create" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg shadow-lg max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
            <div className="px-6 py-4 border-b">
              <h2 className="font-semibold">Create RLS Policy</h2>
              <p className="text-xs text-gray-400 mt-1">
                on {selectedTable?.schema}.{selectedTable?.name}
              </p>
            </div>
            <div className="px-6 py-4 space-y-3">
              {/* Templates */}
              <div>
                <label className="text-xs text-gray-500 block mb-1">Templates</label>
                <div className="flex flex-wrap gap-1">
                  {TEMPLATES.map((tmpl) => (
                    <button
                      key={tmpl.name}
                      onClick={() => applyTemplate(tmpl)}
                      className="px-2 py-1 text-xs bg-gray-100 hover:bg-gray-200 rounded flex items-center gap-1"
                    >
                      <ChevronRight className="w-3 h-3" />
                      {tmpl.name}
                    </button>
                  ))}
                </div>
              </div>

              <div>
                <label className="text-xs text-gray-500 block mb-0.5">Policy Name</label>
                <input
                  aria-label="Policy name"
                  type="text"
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value)}
                  className="w-full px-3 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
                  placeholder="owner_access"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs text-gray-500 block mb-0.5">Command</label>
                  <select
                    aria-label="Command"
                    value={createCommand}
                    onChange={(e) => setCreateCommand(e.target.value)}
                    className="w-full px-3 py-1.5 text-sm border rounded"
                  >
                    {COMMANDS.map((c) => (
                      <option key={c} value={c}>
                        {c}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="text-xs text-gray-500 block mb-0.5">Type</label>
                  <select
                    aria-label="Permissive"
                    value={createPermissive ? "permissive" : "restrictive"}
                    onChange={(e) => setCreatePermissive(e.target.value === "permissive")}
                    className="w-full px-3 py-1.5 text-sm border rounded"
                  >
                    <option value="permissive">PERMISSIVE</option>
                    <option value="restrictive">RESTRICTIVE</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="text-xs text-gray-500 block mb-0.5">
                  USING Expression (for SELECT, UPDATE, DELETE)
                </label>
                <textarea
                  aria-label="USING expression"
                  value={createUsing}
                  onChange={(e) => setCreateUsing(e.target.value)}
                  className="w-full px-3 py-1.5 text-xs font-mono border rounded resize-y h-16"
                  placeholder="(user_id = current_setting('app.user_id')::uuid)"
                  spellCheck={false}
                />
              </div>

              <div>
                <label className="text-xs text-gray-500 block mb-0.5">
                  WITH CHECK Expression (for INSERT, UPDATE)
                </label>
                <textarea
                  aria-label="WITH CHECK expression"
                  value={createWithCheck}
                  onChange={(e) => setCreateWithCheck(e.target.value)}
                  className="w-full px-3 py-1.5 text-xs font-mono border rounded resize-y h-16"
                  placeholder="(user_id = current_setting('app.user_id')::uuid)"
                  spellCheck={false}
                />
              </div>
            </div>
            <div className="px-6 py-3 border-t flex justify-end gap-2">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                onClick={handleCreate}
                disabled={creating || !createName.trim()}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {creating ? "Creating..." : "Create Policy"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {modal.kind === "delete" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg shadow-lg max-w-md w-full mx-4">
            <div className="px-6 py-4 border-b">
              <h2 className="font-semibold">Delete Policy</h2>
            </div>
            <div className="px-6 py-4">
              <p className="text-sm text-gray-600">
                This will permanently drop the policy{" "}
                <strong>{modal.policy.policyName}</strong> from{" "}
                <strong>{modal.policy.tableName}</strong>. This action cannot be undone.
              </p>
            </div>
            <div className="px-6 py-3 border-t flex justify-end gap-2">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                onClick={handleDelete}
                disabled={deleting}
                className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
              >
                {deleting ? "Deleting..." : "Delete"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* SQL Preview Modal */}
      {modal.kind === "sql-preview" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg shadow-lg max-w-lg w-full mx-4">
            <div className="px-6 py-4 border-b">
              <h2 className="font-semibold">SQL Preview</h2>
            </div>
            <div className="px-6 py-4">
              <pre className="p-3 text-xs font-mono bg-gray-900 text-gray-100 rounded overflow-x-auto whitespace-pre-wrap">
                {modal.sql}
              </pre>
            </div>
            <div className="px-6 py-3 border-t flex justify-end">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
