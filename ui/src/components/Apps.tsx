import { useState, useEffect, useCallback } from "react";
import type { AppResponse, AppListResponse } from "../types";
import { listApps, createApp, deleteApp, listUsers } from "../api";
import {
  Plus,
  Trash2,
  Loader2,
  AlertCircle,
  Box,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { ToastContainer, useToast } from "./Toast";

const PER_PAGE = 20;

type Modal =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "delete"; app: AppResponse };

export function Apps() {
  const [data, setData] = useState<AppListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [modal, setModal] = useState<Modal>({ kind: "none" });
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createDescription, setCreateDescription] = useState("");
  const [createOwnerId, setCreateOwnerId] = useState("");
  const [userEmails, setUserEmails] = useState<Record<string, string>>({});
  const { toasts, addToast, removeToast } = useToast();

  const fetchApps = useCallback(async () => {
    try {
      setError(null);
      const result = await listApps({ page, perPage: PER_PAGE });
      setData(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load apps");
      setData(null);
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchApps();
  }, [fetchApps]);

  useEffect(() => {
    listUsers({ perPage: 100 })
      .then((res) => {
        const map: Record<string, string> = {};
        for (const u of res.items) {
          map[u.id] = u.email;
        }
        setUserEmails(map);
      })
      .catch(() => {});
  }, []);

  const handleCreate = async () => {
    if (!createName || !createOwnerId) return;
    setCreating(true);
    try {
      const result = await createApp({
        name: createName,
        description: createDescription,
        ownerUserId: createOwnerId,
      });
      setModal({ kind: "none" });
      setCreateName("");
      setCreateDescription("");
      setCreateOwnerId("");
      addToast("success", `App "${result.name}" created`);
      fetchApps();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to create app");
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (app: AppResponse) => {
    setDeleting(true);
    try {
      await deleteApp(app.id);
      setModal({ kind: "none" });
      addToast("success", `App "${app.name}" deleted`);
      fetchApps();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Delete failed");
    } finally {
      setDeleting(false);
    }
  };

  if (loading && !data) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading apps...
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
              fetchApps();
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
          <h1 className="text-lg font-semibold">Applications</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Manage registered applications and their rate limits
          </p>
        </div>
        <button
          onClick={() => setModal({ kind: "create" })}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          <Plus className="w-4 h-4" />
          Create App
        </button>
      </div>

      {data && data.items.length === 0 ? (
        <div className="text-center py-16 border rounded-lg bg-gray-50">
          <Box className="w-10 h-10 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500 text-sm">No apps registered yet</p>
          <button
            onClick={() => setModal({ kind: "create" })}
            className="mt-3 text-sm text-blue-600 hover:underline"
          >
            Create your first app
          </button>
        </div>
      ) : data ? (
        <>
          <div className="border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    Name
                  </th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    Description
                  </th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    Owner
                  </th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    Rate Limit
                  </th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    Created
                  </th>
                  <th className="text-right px-4 py-2 font-medium text-gray-600">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((app) => (
                  <tr
                    key={app.id}
                    className="border-b last:border-0 hover:bg-gray-50"
                  >
                    <td className="px-4 py-2.5">
                      <span className="font-medium">{app.name}</span>
                      <div className="text-[10px] text-gray-400 mt-0.5">
                        {app.id}
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500">
                      {app.description || <span className="text-gray-300">—</span>}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500">
                      {userEmails[app.ownerUserId] || app.ownerUserId}
                    </td>
                    <td className="px-4 py-2.5 text-xs">
                      {app.rateLimitRps > 0 ? (
                        <span className="text-gray-700">
                          {app.rateLimitRps} req/{app.rateLimitWindowSeconds}s
                        </span>
                      ) : (
                        <span className="text-gray-400">none</span>
                      )}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500">
                      {new Date(app.createdAt).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex justify-end">
                        <button
                          onClick={() =>
                            setModal({ kind: "delete", app })
                          }
                          className="p-1 text-gray-400 hover:text-red-500 rounded hover:bg-gray-100"
                          title="Delete app"
                          aria-label="Delete app"
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

          {/* Pagination */}
          <div className="mt-3 flex items-center justify-between text-sm text-gray-500">
            <span>
              {data.totalItems} app{data.totalItems !== 1 ? "s" : ""}
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
                className="p-1 rounded hover:bg-gray-200 disabled:opacity-30"
              >
                <ChevronLeft className="w-4 h-4" />
              </button>
              <span>
                {page} / {data.totalPages || 1}
              </span>
              <button
                onClick={() =>
                  setPage((p) => Math.min(data.totalPages, p + 1))
                }
                disabled={page >= data.totalPages}
                className="p-1 rounded hover:bg-gray-200 disabled:opacity-30"
              >
                <ChevronRight className="w-4 h-4" />
              </button>
            </div>
          </div>
        </>
      ) : null}

      {/* Create modal */}
      {modal.kind === "create" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white rounded-lg shadow-xl p-6 max-w-md w-full mx-4">
            <h3 className="font-semibold mb-4">Create Application</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Name
                </label>
                <input
                  type="text"
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value)}
                  placeholder="e.g. Frontend App"
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="App name"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Description
                </label>
                <input
                  type="text"
                  value={createDescription}
                  onChange={(e) => setCreateDescription(e.target.value)}
                  placeholder="Optional description"
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Description"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Owner
                </label>
                {Object.keys(userEmails).length > 0 ? (
                  <select
                    value={createOwnerId}
                    onChange={(e) => setCreateOwnerId(e.target.value)}
                    className="w-full border rounded px-3 py-1.5 text-sm"
                    aria-label="Owner"
                  >
                    <option value="">Select an owner...</option>
                    {Object.entries(userEmails).map(([id, email]) => (
                      <option key={id} value={id}>
                        {email}
                      </option>
                    ))}
                  </select>
                ) : (
                  <input
                    type="text"
                    value={createOwnerId}
                    onChange={(e) => setCreateOwnerId(e.target.value)}
                    placeholder="Owner User UUID"
                    className="w-full border rounded px-3 py-1.5 text-sm"
                    aria-label="Owner"
                  />
                )}
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-6">
              <button
                onClick={() => {
                  setModal({ kind: "none" });
                  setCreateName("");
                  setCreateDescription("");
                  setCreateOwnerId("");
                }}
                className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={handleCreate}
                disabled={creating || !createName || !createOwnerId}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {creating ? "Creating..." : "Create"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete confirmation */}
      {modal.kind === "delete" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
            <h3 className="font-semibold mb-2">Delete Application</h3>
            <p className="text-sm text-gray-600 mb-1">
              This will permanently delete the application and revoke all API
              keys scoped to it.
            </p>
            <p className="text-xs font-mono text-gray-500 break-all mb-4">
              {modal.app.name} ({modal.app.id})
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={() => handleDelete(modal.app)}
                disabled={deleting}
                className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
              >
                {deleting ? "Deleting..." : "Delete"}
              </button>
            </div>
          </div>
        </div>
      )}

      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
