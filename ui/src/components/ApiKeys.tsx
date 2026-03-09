import { useState, useEffect, useCallback } from "react";
import type { APIKeyResponse, APIKeyListResponse, AppResponse } from "../types";
import { listApiKeys, createApiKey, revokeApiKey, listUsers, listApps } from "../api";
import {
  Plus,
  Trash2,
  Loader2,
  AlertCircle,
  KeyRound,
  ChevronLeft,
  ChevronRight,
  Copy,
  Check,
} from "lucide-react";
import { cn } from "../lib/utils";
import { ToastContainer, useToast } from "./Toast";

const PER_PAGE = 20;
const APP_PER_PAGE = 100;

function formatAppRateLimit(app: AppResponse | undefined): string {
  if (!app) {
    return "Rate: unknown";
  }
  if (app.rateLimitRps <= 0) {
    return "Rate: unlimited";
  }
  return `Rate: ${app.rateLimitRps} req/${app.rateLimitWindowSeconds}s`;
}

type Modal =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "created"; key: string; apiKey: APIKeyResponse }
  | { kind: "revoke"; apiKey: APIKeyResponse };

export function ApiKeys() {
  const [data, setData] = useState<APIKeyListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [modal, setModal] = useState<Modal>({ kind: "none" });
  const [revoking, setRevoking] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createUserId, setCreateUserId] = useState("");
  const [createScope, setCreateScope] = useState("*");
  const [createAllowedTables, setCreateAllowedTables] = useState("");
  const [createAppId, setCreateAppId] = useState("");
  const [userEmails, setUserEmails] = useState<Record<string, string>>({});
  const [appsById, setAppsById] = useState<Record<string, AppResponse>>({});
  const [appsError, setAppsError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const { toasts, addToast, removeToast } = useToast();

  const fetchKeys = useCallback(async () => {
    try {
      setError(null);
      const result = await listApiKeys({ page, perPage: PER_PAGE });
      setData(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load API keys");
      setData(null);
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchKeys();
  }, [fetchKeys]);

  // Load user emails for display.
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

  // Load apps for app-scoped API key association and display.
  useEffect(() => {
    let cancelled = false;
    const loadApps = async () => {
      try {
        setAppsError(null);
        const items: AppResponse[] = [];
        let page = 1;
        while (!cancelled) {
          const res = await listApps({ page, perPage: APP_PER_PAGE });
          items.push(...res.items);
          if (res.totalPages <= page || res.totalPages === 0) {
            break;
          }
          page += 1;
        }
        if (cancelled) return;
        const map: Record<string, AppResponse> = {};
        for (const app of items) {
          map[app.id] = app;
        }
        setAppsById(map);
      } catch {
        if (cancelled) return;
        setAppsError("Failed to load apps");
        setAppsById({});
      }
    };
    loadApps();
    return () => {
      cancelled = true;
    };
  }, []);

  const handleCreate = async () => {
    if (!createUserId || !createName) return;
    setCreating(true);
    try {
      const tables = createAllowedTables
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
      const result = await createApiKey({
        userId: createUserId,
        name: createName,
        scope: createScope,
        ...(createAppId ? { appId: createAppId } : {}),
        ...(tables.length > 0 ? { allowedTables: tables } : {}),
      });
      setModal({ kind: "created", key: result.key, apiKey: result.apiKey });
      setCreateName("");
      setCreateUserId("");
      setCreateScope("*");
      setCreateAllowedTables("");
      setCreateAppId("");
      addToast("success", `API key "${result.apiKey.name}" created`);
      fetchKeys();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to create key");
    } finally {
      setCreating(false);
    }
  };

  const handleRevoke = async (apiKey: APIKeyResponse) => {
    setRevoking(true);
    try {
      await revokeApiKey(apiKey.id);
      setModal({ kind: "none" });
      addToast("success", `API key "${apiKey.name}" revoked`);
      fetchKeys();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Revoke failed");
    } finally {
      setRevoking(false);
    }
  };

  const handleCopy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      addToast("error", "Failed to copy to clipboard");
    }
  };

  if (loading && !data) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading API keys...
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
              fetchKeys();
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
          <h1 className="text-lg font-semibold">API Keys</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Non-expiring keys for service-to-service authentication
          </p>
        </div>
        <button
          onClick={() => setModal({ kind: "create" })}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          <Plus className="w-4 h-4" />
          Create Key
        </button>
      </div>

      {data && data.items.length === 0 ? (
        <div className="text-center py-16 border rounded-lg bg-gray-50">
          <KeyRound className="w-10 h-10 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500 text-sm">No API keys created yet</p>
          <button
            onClick={() => setModal({ kind: "create" })}
            className="mt-3 text-sm text-blue-600 hover:underline"
          >
            Create your first API key
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
                    Key
                  </th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    Scope
                  </th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    User
                  </th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    App
                  </th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    Last Used
                  </th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    Created
                  </th>
                  <th className="text-center px-4 py-2 font-medium text-gray-600">
                    Status
                  </th>
                  <th className="text-right px-4 py-2 font-medium text-gray-600">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((apiKey) => (
                  <tr
                    key={apiKey.id}
                    className="border-b last:border-0 hover:bg-gray-50"
                  >
                    <td className="px-4 py-2.5">
                      <span className="font-medium">{apiKey.name}</span>
                      <div className="text-[10px] text-gray-400 mt-0.5">
                        {apiKey.id}
                      </div>
                    </td>
                    <td className="px-4 py-2.5">
                      <code className="text-xs bg-gray-100 px-1.5 py-0.5 rounded text-gray-600">
                        {apiKey.keyPrefix}...
                      </code>
                    </td>
                    <td className="px-4 py-2.5">
                      <span
                        className={cn(
                          "inline-block px-1.5 py-0.5 rounded text-[10px] font-medium",
                          apiKey.scope === "*"
                            ? "bg-purple-100 text-purple-700"
                            : apiKey.scope === "readonly"
                              ? "bg-blue-100 text-blue-700"
                              : "bg-yellow-100 text-yellow-700",
                        )}
                      >
                        {apiKey.scope === "*" ? "full access" : apiKey.scope}
                      </span>
                      {apiKey.allowedTables && apiKey.allowedTables.length > 0 && (
                        <div className="text-[10px] text-gray-400 mt-0.5">
                          {apiKey.allowedTables.join(", ")}
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500">
                      {userEmails[apiKey.userId] || apiKey.userId}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500">
                      {apiKey.appId ? (
                        <>
                          <div className="text-gray-700">
                            {appsById[apiKey.appId]?.name || apiKey.appId}
                          </div>
                          <div className="text-[10px] text-gray-400 mt-0.5">
                            {formatAppRateLimit(appsById[apiKey.appId])}
                          </div>
                        </>
                      ) : (
                        <span className="text-gray-400">User-scoped</span>
                      )}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500">
                      {apiKey.lastUsedAt
                        ? new Date(apiKey.lastUsedAt).toLocaleDateString()
                        : "Never"}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500">
                      {new Date(apiKey.createdAt).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-2.5 text-center">
                      <span
                        className={cn(
                          "inline-block px-2 py-0.5 rounded-full text-[10px] font-medium",
                          apiKey.revokedAt
                            ? "bg-red-100 text-red-700"
                            : "bg-green-100 text-green-700",
                        )}
                      >
                        {apiKey.revokedAt ? "Revoked" : "Active"}
                      </span>
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex justify-end">
                        {!apiKey.revokedAt && (
                          <button
                            onClick={() =>
                              setModal({ kind: "revoke", apiKey })
                            }
                            className="p-1 text-gray-400 hover:text-red-500 rounded hover:bg-gray-100"
                            title="Revoke key"
                            aria-label="Revoke key"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        )}
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
              {data.totalItems} key{data.totalItems !== 1 ? "s" : ""}
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
            <h3 className="font-semibold mb-4">Create API Key</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Name
                </label>
                <input
                  type="text"
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value)}
                  placeholder="e.g. CI/CD Pipeline"
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Key name"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  User ID
                </label>
                {Object.keys(userEmails).length > 0 ? (
                  <select
                    value={createUserId}
                    onChange={(e) => setCreateUserId(e.target.value)}
                    className="w-full border rounded px-3 py-1.5 text-sm"
                    aria-label="User"
                  >
                    <option value="">Select a user...</option>
                    {Object.entries(userEmails).map(([id, email]) => (
                      <option key={id} value={id}>
                        {email}
                      </option>
                    ))}
                  </select>
                ) : (
                  <input
                    type="text"
                    value={createUserId}
                    onChange={(e) => setCreateUserId(e.target.value)}
                    placeholder="User UUID"
                    className="w-full border rounded px-3 py-1.5 text-sm"
                    aria-label="User ID"
                  />
                )}
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Scope
                </label>
                <select
                  value={createScope}
                  onChange={(e) => setCreateScope(e.target.value)}
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Scope"
                >
                  <option value="*">Full access (*)</option>
                  <option value="readonly">Read only</option>
                  <option value="readwrite">Read & write</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  App Scope
                </label>
                <select
                  value={createAppId}
                  onChange={(e) => setCreateAppId(e.target.value)}
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="App Scope"
                >
                  <option value="">User-scoped (no app)</option>
                  {Object.values(appsById).map((app) => (
                    <option key={app.id} value={app.id}>
                      {app.name}
                    </option>
                  ))}
                </select>
                <p className="text-[10px] text-gray-400 mt-0.5">
                  Select an app to apply app-level scopes and rate limits.
                </p>
                {appsError && (
                  <p className="text-[10px] text-amber-600 mt-1">{appsError}</p>
                )}
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Allowed Tables
                </label>
                <input
                  type="text"
                  value={createAllowedTables}
                  onChange={(e) => setCreateAllowedTables(e.target.value)}
                  placeholder="Leave empty for all tables, or comma-separated: posts, users"
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Allowed tables"
                />
                <p className="text-[10px] text-gray-400 mt-0.5">
                  Comma-separated table names. Leave empty to allow all tables.
                </p>
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-6">
              <button
                onClick={() => {
                  setModal({ kind: "none" });
                  setCreateName("");
                  setCreateUserId("");
                  setCreateScope("*");
                  setCreateAllowedTables("");
                  setCreateAppId("");
                }}
                className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={handleCreate}
                disabled={creating || !createName || !createUserId}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {creating ? "Creating..." : "Create"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Created modal — shows the key once */}
      {modal.kind === "created" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white rounded-lg shadow-xl p-6 max-w-lg w-full mx-4">
            <h3 className="font-semibold mb-2">API Key Created</h3>
            <p className="text-sm text-gray-600 mb-4">
              Copy this key now. It will not be shown again.
            </p>
            <div className="flex items-center gap-2 bg-gray-50 border rounded p-3">
              <code className="flex-1 text-xs break-all font-mono">
                {modal.key}
              </code>
              <button
                onClick={() => handleCopy(modal.key)}
                className="p-1.5 text-gray-400 hover:text-gray-600 rounded hover:bg-gray-200 shrink-0"
                title="Copy to clipboard"
                aria-label="Copy to clipboard"
              >
                {copied ? (
                  <Check className="w-4 h-4 text-green-500" />
                ) : (
                  <Copy className="w-4 h-4" />
                )}
              </button>
            </div>
            <div className="mt-4 text-xs text-gray-500 space-y-0.5">
              <p>
                <strong>Name:</strong> {modal.apiKey.name}
              </p>
              <p>
                <strong>User:</strong>{" "}
                {userEmails[modal.apiKey.userId] || modal.apiKey.userId}
              </p>
              <p>
                <strong>Scope:</strong>{" "}
                {modal.apiKey.scope === "*" ? "full access" : modal.apiKey.scope}
              </p>
              {modal.apiKey.appId && (
                <>
                  <p>
                    <strong>App:</strong>{" "}
                    {appsById[modal.apiKey.appId]?.name || modal.apiKey.appId}
                  </p>
                  <p>
                    <strong>Rate:</strong>{" "}
                    {formatAppRateLimit(appsById[modal.apiKey.appId]).replace("Rate: ", "")}
                  </p>
                </>
              )}
              {modal.apiKey.allowedTables && modal.apiKey.allowedTables.length > 0 && (
                <p>
                  <strong>Tables:</strong>{" "}
                  {modal.apiKey.allowedTables.join(", ")}
                </p>
              )}
            </div>
            <div className="flex justify-end mt-6">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
              >
                Done
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Revoke confirmation */}
      {modal.kind === "revoke" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
            <h3 className="font-semibold mb-2">Revoke API Key</h3>
            <p className="text-sm text-gray-600 mb-1">
              This will permanently revoke the API key. Any applications using
              this key will lose access.
            </p>
            <p className="text-xs font-mono text-gray-500 break-all mb-4">
              {modal.apiKey.name} ({modal.apiKey.keyPrefix}...)
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={() => handleRevoke(modal.apiKey)}
                disabled={revoking}
                className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
              >
                {revoking ? "Revoking..." : "Revoke"}
              </button>
            </div>
          </div>
        </div>
      )}

      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
