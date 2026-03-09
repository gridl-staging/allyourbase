import { useCallback, useEffect, useMemo, useState } from "react";
import type { MatviewListResponse, MatviewRegistration, SchemaCache } from "../types";
import {
  listMatviews,
  registerMatview,
  updateMatview,
  deleteMatview,
  refreshMatview,
} from "../api";
import {
  AlertCircle,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { cn } from "../lib/utils";
import { ToastContainer, useToast } from "./Toast";

type ModalState =
  | { kind: "none" }
  | { kind: "register" }
  | { kind: "edit"; matview: MatviewRegistration }
  | { kind: "delete"; matview: MatviewRegistration };

function statusBadgeClass(status: string | null): string {
  switch (status) {
    case "success":
      return "bg-green-100 text-green-700";
    case "error":
      return "bg-red-100 text-red-700";
    default:
      return "bg-gray-100 text-gray-700";
  }
}

function formatDate(iso: string | null): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

function errorPreview(error: string | null): string {
  if (!error) return "-";
  return error.length > 80 ? `${error.slice(0, 80)}...` : error;
}

interface MatviewsAdminProps {
  schema: SchemaCache;
}

export function MatviewsAdmin({ schema }: MatviewsAdminProps) {
  const [data, setData] = useState<MatviewListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [modal, setModal] = useState<ModalState>({ kind: "none" });
  const [refreshingId, setRefreshingId] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [registerForm, setRegisterForm] = useState({ view: "", mode: "standard" });
  const [editMode, setEditMode] = useState("standard");
  const { toasts, addToast, removeToast } = useToast();

  const discoveredMatviews = useMemo(() => {
    return Object.values(schema.tables)
      .filter((t) => t.kind === "materialized_view")
      .map((t) => ({ schema: t.schema, name: t.name, key: `${t.schema}.${t.name}` }))
      .sort((a, b) => a.key.localeCompare(b.key));
  }, [schema]);

  const load = useCallback(async () => {
    try {
      setError(null);
      const res = await listMatviews();
      setData(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load materialized views");
      setData(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const handleRefresh = async (mv: MatviewRegistration) => {
    setRefreshingId(mv.id);
    try {
      await refreshMatview(mv.id);
      addToast("success", `Refreshed ${mv.schemaName}.${mv.viewName}`);
      await load();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Refresh failed");
    } finally {
      setRefreshingId(null);
    }
  };

  const openRegister = () => {
    setRegisterForm({
      view: discoveredMatviews.length > 0 ? discoveredMatviews[0].key : "",
      mode: "standard",
    });
    setModal({ kind: "register" });
  };

  const openEdit = (mv: MatviewRegistration) => {
    setEditMode(mv.refreshMode);
    setModal({ kind: "edit", matview: mv });
  };

  const handleRegister = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const [schemaName, viewName] = registerForm.view.split(".");
    if (!schemaName || !viewName) return;

    setSubmitting(true);
    try {
      await registerMatview({
        schema: schemaName,
        viewName,
        refreshMode: registerForm.mode,
      });
      addToast("success", `Registered ${registerForm.view}`);
      setModal({ kind: "none" });
      await load();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Registration failed");
    } finally {
      setSubmitting(false);
    }
  };

  const handleUpdate = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (modal.kind !== "edit") return;

    setSubmitting(true);
    try {
      await updateMatview(modal.matview.id, { refreshMode: editMode });
      addToast("success", "Refresh mode updated");
      setModal({ kind: "none" });
      await load();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Update failed");
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (mv: MatviewRegistration) => {
    setSubmitting(true);
    try {
      await deleteMatview(mv.id);
      addToast("success", `Unregistered ${mv.schemaName}.${mv.viewName}`);
      setModal({ kind: "none" });
      await load();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Delete failed");
    } finally {
      setSubmitting(false);
    }
  };

  if (loading && !data) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading materialized views...
      </div>
    );
  }

  if (error && !data) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{error}</p>
          <button
            onClick={() => {
              setLoading(true);
              load();
            }}
            className="mt-2 text-sm text-blue-600 hover:underline"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-lg font-semibold">Materialized Views</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Register and refresh materialized views
          </p>
        </div>
        <button
          onClick={openRegister}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          <Plus className="w-4 h-4" />
          Register Matview
        </button>
      </div>

      {data && data.items.length === 0 ? (
        <div className="text-center py-12 border rounded-lg bg-gray-50 text-gray-500 text-sm">
          No materialized views registered
        </div>
      ) : data ? (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Schema</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">View Name</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Mode</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Status</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Last Refresh</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Duration</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Error</th>
                <th className="text-right px-4 py-2 font-medium text-gray-600">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((mv) => (
                <tr key={mv.id} className="border-b last:border-0 hover:bg-gray-50">
                  <td className="px-4 py-2.5 text-xs text-gray-600">{mv.schemaName}</td>
                  <td className="px-4 py-2.5 font-medium">{mv.viewName}</td>
                  <td className="px-4 py-2.5">
                    <code className="text-xs bg-gray-100 px-1.5 py-0.5 rounded text-gray-700">
                      {mv.refreshMode}
                    </code>
                  </td>
                  <td className="px-4 py-2.5">
                    {mv.lastRefreshStatus ? (
                      <span
                        className={cn(
                          "inline-block px-2 py-0.5 rounded-full text-[10px] font-medium",
                          statusBadgeClass(mv.lastRefreshStatus),
                        )}
                      >
                        {mv.lastRefreshStatus}
                      </span>
                    ) : (
                      <span className="text-xs text-gray-400">-</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-500">
                    {formatDate(mv.lastRefreshAt)}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-500">
                    {mv.lastRefreshDurationMs != null ? `${mv.lastRefreshDurationMs}ms` : "-"}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-500 max-w-[240px]">
                    {errorPreview(mv.lastRefreshError)}
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex justify-end gap-1">
                      <button
                        onClick={() => handleRefresh(mv)}
                        disabled={refreshingId === mv.id}
                        aria-label={`Refresh matview ${mv.id}`}
                        className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded bg-blue-50 text-blue-700 hover:bg-blue-100 disabled:opacity-60"
                      >
                        <RefreshCw className={cn("w-3.5 h-3.5", refreshingId === mv.id && "animate-spin")} />
                        Refresh
                      </button>
                      <button
                        onClick={() => openEdit(mv)}
                        aria-label={`Edit matview ${mv.id}`}
                        className="p-1 text-gray-400 hover:text-blue-500 rounded hover:bg-gray-100"
                      >
                        <Pencil className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => setModal({ kind: "delete", matview: mv })}
                        aria-label={`Delete matview ${mv.id}`}
                        className="p-1 text-gray-400 hover:text-red-500 rounded hover:bg-gray-100"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {/* Register modal */}
      {modal.kind === "register" && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="w-full max-w-md rounded-lg bg-white shadow-xl border">
            <div className="px-5 py-4 border-b">
              <h2 className="text-base font-semibold">Register Materialized View</h2>
            </div>
            <form onSubmit={handleRegister} className="p-5 space-y-3">
              <div>
                <label htmlFor="matview-view" className="block text-sm text-gray-700 mb-1">
                  View
                </label>
                <select
                  id="matview-view"
                  aria-label="View"
                  value={registerForm.view}
                  onChange={(e) => setRegisterForm((prev) => ({ ...prev, view: e.target.value }))}
                  className="w-full border rounded px-3 py-2 text-sm"
                  required
                >
                  {discoveredMatviews.map((mv) => (
                    <option key={mv.key} value={mv.key}>
                      {mv.key}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label htmlFor="matview-mode" className="block text-sm text-gray-700 mb-1">
                  Refresh Mode
                </label>
                <select
                  id="matview-mode"
                  aria-label="Refresh Mode"
                  value={registerForm.mode}
                  onChange={(e) => setRegisterForm((prev) => ({ ...prev, mode: e.target.value }))}
                  className="w-full border rounded px-3 py-2 text-sm"
                >
                  <option value="standard">standard</option>
                  <option value="concurrent">concurrent</option>
                </select>
              </div>
              <div className="pt-2 flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setModal({ kind: "none" })}
                  className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting}
                  className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-70"
                >
                  Register
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit mode modal */}
      {modal.kind === "edit" && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="w-full max-w-md rounded-lg bg-white shadow-xl border">
            <div className="px-5 py-4 border-b">
              <h2 className="text-base font-semibold">Edit Refresh Mode</h2>
            </div>
            <form onSubmit={handleUpdate} className="p-5 space-y-3">
              <div>
                <label htmlFor="edit-matview-mode" className="block text-sm text-gray-700 mb-1">
                  Refresh Mode
                </label>
                <select
                  id="edit-matview-mode"
                  aria-label="Refresh Mode"
                  value={editMode}
                  onChange={(e) => setEditMode(e.target.value)}
                  className="w-full border rounded px-3 py-2 text-sm"
                >
                  <option value="standard">standard</option>
                  <option value="concurrent">concurrent</option>
                </select>
              </div>
              <div className="pt-2 flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setModal({ kind: "none" })}
                  className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting}
                  className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-70"
                >
                  Save
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete confirmation modal */}
      {modal.kind === "delete" && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="w-full max-w-md rounded-lg bg-white shadow-xl border">
            <div className="px-5 py-4 border-b">
              <h2 className="text-base font-semibold">Unregister materialized view?</h2>
            </div>
            <div className="p-5">
              <p className="text-sm text-gray-600">
                This will remove <span className="font-medium">{modal.matview.viewName}</span> from
                the refresh registry. The materialized view itself will not be dropped.
              </p>
              <div className="pt-4 flex justify-end gap-2">
                <button
                  onClick={() => setModal({ kind: "none" })}
                  className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50"
                >
                  Cancel
                </button>
                <button
                  onClick={() => handleDelete(modal.matview)}
                  disabled={submitting}
                  className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-70"
                >
                  Unregister
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
