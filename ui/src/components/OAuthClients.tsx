import { useState, useEffect, useCallback } from "react";
import type { OAuthClientResponse, OAuthClientListResponse, AppResponse } from "../types";
import {
  listOAuthClients,
  createOAuthClient,
  revokeOAuthClient,
  rotateOAuthClientSecret,
  listApps,
} from "../api";
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
  RefreshCw,
} from "lucide-react";
import { cn } from "../lib/utils";
import { ToastContainer, useToast } from "./Toast";

const PER_PAGE = 20;

type Modal =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "created"; secret: string; client: OAuthClientResponse }
  | { kind: "revoke"; client: OAuthClientResponse }
  | { kind: "rotate-confirm"; client: OAuthClientResponse }
  | { kind: "rotate-result"; secret: string; client: OAuthClientResponse };

export function OAuthClients() {
  const [data, setData] = useState<OAuthClientListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [modal, setModal] = useState<Modal>({ kind: "none" });
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState(false);
  const [rotating, setRotating] = useState(false);
  const [copied, setCopied] = useState(false);

  // Create form state.
  const [createName, setCreateName] = useState("");
  const [createAppId, setCreateAppId] = useState("");
  const [createClientType, setCreateClientType] = useState("confidential");
  const [createRedirectUris, setCreateRedirectUris] = useState("");
  const [createScopes, setCreateScopes] = useState("readonly");

  // Lookup maps.
  const [appsById, setAppsById] = useState<Record<string, AppResponse>>({});

  const { toasts, addToast, removeToast } = useToast();

  const fetchClients = useCallback(async () => {
    try {
      setError(null);
      const result = await listOAuthClients({ page, perPage: PER_PAGE });
      setData(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load OAuth clients");
      setData(null);
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchClients();
  }, [fetchClients]);

  // Load apps for display.
  useEffect(() => {
    let cancelled = false;
    const loadApps = async () => {
      try {
        const items: AppResponse[] = [];
        let pg = 1;
        while (!cancelled) {
          const res = await listApps({ page: pg, perPage: 100 });
          items.push(...res.items);
          if (res.totalPages <= pg || res.totalPages === 0) break;
          pg += 1;
        }
        if (cancelled) return;
        const map: Record<string, AppResponse> = {};
        for (const app of items) {
          map[app.id] = app;
        }
        setAppsById(map);
      } catch {
        if (cancelled) return;
        setAppsById({});
      }
    };
    loadApps();
    return () => { cancelled = true; };
  }, []);

  const resetCreateForm = () => {
    setCreateName("");
    setCreateAppId("");
    setCreateClientType("confidential");
    setCreateRedirectUris("");
    setCreateScopes("readonly");
  };

  const handleCreate = async () => {
    if (!createName || !createAppId) return;
    setCreating(true);
    try {
      const uris = createRedirectUris
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
      const result = await createOAuthClient({
        appId: createAppId,
        name: createName,
        clientType: createClientType,
        redirectUris: uris,
        scopes: [createScopes],
      });
      setModal({ kind: "created", secret: result.clientSecret, client: result.client });
      resetCreateForm();
      addToast("success", `OAuth client "${result.client.name}" registered`);
      fetchClients();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to create client");
    } finally {
      setCreating(false);
    }
  };

  const handleRevoke = async (client: OAuthClientResponse) => {
    setRevoking(true);
    try {
      await revokeOAuthClient(client.clientId);
      setModal({ kind: "none" });
      addToast("success", `OAuth client "${client.name}" revoked`);
      fetchClients();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Revoke failed");
    } finally {
      setRevoking(false);
    }
  };

  const handleRotate = async (client: OAuthClientResponse) => {
    setRotating(true);
    try {
      const result = await rotateOAuthClientSecret(client.clientId);
      setModal({ kind: "rotate-result", secret: result.clientSecret, client });
      addToast("success", `Secret rotated for "${client.name}"`);
      fetchClients();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Rotate failed");
    } finally {
      setRotating(false);
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
        Loading OAuth clients...
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
              fetchClients();
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
          <h1 className="text-lg font-semibold">OAuth Clients</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Manage OAuth 2.0 client applications for third-party access
          </p>
        </div>
        <button
          onClick={() => setModal({ kind: "create" })}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          <Plus className="w-4 h-4" />
          Register Client
        </button>
      </div>

      {data && data.items.length === 0 ? (
        <div className="text-center py-16 border rounded-lg bg-gray-50">
          <KeyRound className="w-10 h-10 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500 text-sm">No OAuth clients registered yet</p>
          <button
            onClick={() => setModal({ kind: "create" })}
            className="mt-3 text-sm text-blue-600 hover:underline"
          >
            Register your first client
          </button>
        </div>
      ) : data ? (
        <>
          <div className="border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">Name</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">Client ID</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">Type</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">App</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">Scopes</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">Redirect URIs</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">Created</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">Token Stats</th>
                  <th className="text-center px-4 py-2 font-medium text-gray-600">Status</th>
                  <th className="text-right px-4 py-2 font-medium text-gray-600">Actions</th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((client) => (
                  <tr key={client.id} className="border-b last:border-0 hover:bg-gray-50">
                    <td className="px-4 py-2.5">
                      <span className="font-medium">{client.name}</span>
                    </td>
                    <td className="px-4 py-2.5">
                      <code className="text-xs bg-gray-100 px-1.5 py-0.5 rounded text-gray-600">
                        {client.clientId}
                      </code>
                    </td>
                    <td className="px-4 py-2.5">
                      <span
                        className={cn(
                          "inline-block px-1.5 py-0.5 rounded text-[10px] font-medium",
                          client.clientType === "confidential"
                            ? "bg-purple-100 text-purple-700"
                            : "bg-blue-100 text-blue-700",
                        )}
                      >
                        {client.clientType}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500">
                      {appsById[client.appId]?.name || client.appId}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500">
                      {client.scopes.join(", ")}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500 max-w-[200px]">
                      {client.redirectUris.map((uri, i) => (
                        <div key={i} className="truncate">{uri}</div>
                      ))}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-gray-500">
                      {new Date(client.createdAt).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="text-xs text-gray-700">
                        {`Access ${client.activeAccessTokenCount} / Refresh ${client.activeRefreshTokenCount} / Grants ${client.totalGrants}`}
                      </div>
                      <div className="text-[10px] text-gray-500 mt-0.5">
                        {`Last issued ${client.lastTokenIssuedAt ? new Date(client.lastTokenIssuedAt).toLocaleString() : "never"}`}
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-center">
                      <span
                        className={cn(
                          "inline-block px-2 py-0.5 rounded-full text-[10px] font-medium",
                          client.revokedAt
                            ? "bg-red-100 text-red-700"
                            : "bg-green-100 text-green-700",
                        )}
                      >
                        {client.revokedAt ? "Revoked" : "Active"}
                      </span>
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex justify-end gap-1">
                        {!client.revokedAt && client.clientType === "confidential" && (
                          <button
                            onClick={() => setModal({ kind: "rotate-confirm", client })}
                            className="p-1 text-gray-400 hover:text-blue-500 rounded hover:bg-gray-100"
                            title="Rotate secret"
                            aria-label="Rotate secret"
                          >
                            <RefreshCw className="w-3.5 h-3.5" />
                          </button>
                        )}
                        {!client.revokedAt && (
                          <button
                            onClick={() => setModal({ kind: "revoke", client })}
                            className="p-1 text-gray-400 hover:text-red-500 rounded hover:bg-gray-100"
                            title="Revoke client"
                            aria-label="Revoke client"
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
              {data.totalItems} client{data.totalItems !== 1 ? "s" : ""}
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
                onClick={() => setPage((p) => Math.min(data.totalPages, p + 1))}
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
            <h3 className="font-semibold mb-4">Register OAuth Client</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Name
                </label>
                <input
                  type="text"
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value)}
                  placeholder="e.g. Web Dashboard"
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Client name"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  App
                </label>
                <select
                  value={createAppId}
                  onChange={(e) => setCreateAppId(e.target.value)}
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="App"
                >
                  <option value="">Select an app...</option>
                  {Object.values(appsById).map((app) => (
                    <option key={app.id} value={app.id}>
                      {app.name}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Client Type
                </label>
                <select
                  value={createClientType}
                  onChange={(e) => setCreateClientType(e.target.value)}
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Client type"
                >
                  <option value="confidential">Confidential (server-side)</option>
                  <option value="public">Public (SPA / native app)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Redirect URIs
                </label>
                <input
                  type="text"
                  value={createRedirectUris}
                  onChange={(e) => setCreateRedirectUris(e.target.value)}
                  placeholder="https://example.com/callback"
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Redirect URIs"
                />
                <p className="text-[10px] text-gray-400 mt-0.5">
                  Comma-separated. HTTPS required in production; localhost allowed for development.
                </p>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Scopes
                </label>
                <select
                  value={createScopes}
                  onChange={(e) => setCreateScopes(e.target.value)}
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Scopes"
                >
                  <option value="readonly">Read only</option>
                  <option value="readwrite">Read & write</option>
                  <option value="*">Full access (*)</option>
                </select>
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-6">
              <button
                onClick={() => {
                  setModal({ kind: "none" });
                  resetCreateForm();
                }}
                className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={handleCreate}
                disabled={creating || !createName || !createAppId}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {creating ? "Registering..." : "Register"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Created modal — shows client_id and secret */}
      {modal.kind === "created" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white rounded-lg shadow-xl p-6 max-w-lg w-full mx-4">
            <h3 className="font-semibold mb-2">OAuth Client Registered</h3>

            <div className="mt-4 text-xs text-gray-500 space-y-1">
              <p><strong>Client ID:</strong></p>
              <code className="block text-xs bg-gray-50 border rounded p-2 break-all font-mono">
                {modal.client.clientId}
              </code>
            </div>

            {modal.secret && (
              <div className="mt-4">
                <p className="text-sm text-gray-600 mb-2">
                  Copy this secret now. It will not be shown again.
                </p>
                <div className="flex items-center gap-2 bg-gray-50 border rounded p-3">
                  <code className="flex-1 text-xs break-all font-mono">
                    {modal.secret}
                  </code>
                  <button
                    onClick={() => handleCopy(modal.secret)}
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
              </div>
            )}

            <div className="mt-4 text-xs text-gray-500 space-y-0.5">
              <p><strong>Name:</strong> {modal.client.name}</p>
              <p><strong>Type:</strong> {modal.client.clientType}</p>
              <p><strong>Scopes:</strong> {modal.client.scopes.join(", ")}</p>
              <p><strong>Redirect URIs:</strong> {modal.client.redirectUris.join(", ")}</p>
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
            <h3 className="font-semibold mb-2">Revoke OAuth Client</h3>
            <p className="text-sm text-gray-600 mb-1">
              This will revoke the OAuth client and invalidate all tokens issued to it.
            </p>
            <p className="text-xs font-mono text-gray-500 break-all mb-4">
              {modal.client.name} ({modal.client.clientId})
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={() => handleRevoke(modal.client)}
                disabled={revoking}
                className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
              >
                {revoking ? "Revoking..." : "Revoke"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Rotate secret confirmation */}
      {modal.kind === "rotate-confirm" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
            <h3 className="font-semibold mb-2">Rotate Client Secret</h3>
            <p className="text-sm text-gray-600 mb-1">
              This will invalidate the current secret. Any application using the old secret will stop working.
            </p>
            <p className="text-xs font-mono text-gray-500 break-all mb-4">
              {modal.client.name}
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={() => handleRotate(modal.client)}
                disabled={rotating}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {rotating ? "Rotating..." : "Rotate"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Rotate result — shows new secret */}
      {modal.kind === "rotate-result" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white rounded-lg shadow-xl p-6 max-w-lg w-full mx-4">
            <h3 className="font-semibold mb-2">New Client Secret</h3>
            <p className="text-sm text-gray-600 mb-4">
              Copy this secret now. It will not be shown again.
            </p>
            <div className="flex items-center gap-2 bg-gray-50 border rounded p-3">
              <code className="flex-1 text-xs break-all font-mono">
                {modal.secret}
              </code>
              <button
                onClick={() => handleCopy(modal.secret)}
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

      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
