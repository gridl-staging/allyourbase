import { Check, Copy } from "lucide-react";
import type {
  APIKeyResponse,
  AppResponse,
} from "../types";
import { formatAppRateLimit } from "./api-keys-helpers";

export type ApiKeysModal =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "created"; key: string; apiKey: APIKeyResponse }
  | { kind: "revoke"; apiKey: APIKeyResponse };

interface ApiKeysModalsProps {
  modal: ApiKeysModal;
  creating: boolean;
  revoking: boolean;
  copied: boolean;
  createName: string;
  createUserId: string;
  createScope: string;
  createAllowedTables: string;
  createAppId: string;
  userEmails: Record<string, string>;
  appsById: Record<string, AppResponse>;
  appsError: string | null;
  onClose: () => void;
  onCreateNameChange: (value: string) => void;
  onCreateUserIdChange: (value: string) => void;
  onCreateScopeChange: (value: string) => void;
  onCreateAllowedTablesChange: (value: string) => void;
  onCreateAppIdChange: (value: string) => void;
  onCreate: () => void;
  onRevoke: (apiKey: APIKeyResponse) => void;
  onCopy: (value: string) => void;
  onCancelCreate: () => void;
}

export function ApiKeysModals({
  modal,
  creating,
  revoking,
  copied,
  createName,
  createUserId,
  createScope,
  createAllowedTables,
  createAppId,
  userEmails,
  appsById,
  appsError,
  onClose,
  onCreateNameChange,
  onCreateUserIdChange,
  onCreateScopeChange,
  onCreateAllowedTablesChange,
  onCreateAppIdChange,
  onCreate,
  onRevoke,
  onCopy,
  onCancelCreate,
}: ApiKeysModalsProps) {
  return (
    <>
      {modal.kind === "create" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-md w-full mx-4">
            <h3 className="font-semibold mb-4">Create API Key</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Name</label>
                <input
                  type="text"
                  value={createName}
                  onChange={(event) => onCreateNameChange(event.target.value)}
                  placeholder="e.g. CI/CD Pipeline"
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Key name"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">User ID</label>
                {Object.keys(userEmails).length > 0 ? (
                  <select
                    value={createUserId}
                    onChange={(event) => onCreateUserIdChange(event.target.value)}
                    className="w-full border rounded px-3 py-1.5 text-sm"
                    aria-label="User"
                  >
                    <option value="">Select a user...</option>
                    {Object.entries(userEmails).map(([id, email]) => (
                      <option key={id} value={id}>{email}</option>
                    ))}
                  </select>
                ) : (
                  <input
                    type="text"
                    value={createUserId}
                    onChange={(event) => onCreateUserIdChange(event.target.value)}
                    placeholder="User UUID"
                    className="w-full border rounded px-3 py-1.5 text-sm"
                    aria-label="User ID"
                  />
                )}
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Scope</label>
                <select
                  value={createScope}
                  onChange={(event) => onCreateScopeChange(event.target.value)}
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Scope"
                >
                  <option value="*">Full access (*)</option>
                  <option value="readonly">Read only</option>
                  <option value="readwrite">Read &amp; write</option>
                </select>
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">App Scope</label>
                <select
                  value={createAppId}
                  onChange={(event) => onCreateAppIdChange(event.target.value)}
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="App Scope"
                >
                  <option value="">User-scoped (no app)</option>
                  {Object.values(appsById).map((app) => (
                    <option key={app.id} value={app.id}>{app.name}</option>
                  ))}
                </select>
                <p className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">
                  Select an app to apply app-level scopes and rate limits.
                </p>
                {appsError && <p className="text-[10px] text-amber-600 mt-1">{appsError}</p>}
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Allowed Tables</label>
                <input
                  type="text"
                  value={createAllowedTables}
                  onChange={(event) => onCreateAllowedTablesChange(event.target.value)}
                  placeholder="Leave empty for all tables, or comma-separated: posts, users"
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Allowed tables"
                />
                <p className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">
                  Comma-separated table names. Leave empty to allow all tables.
                </p>
              </div>
            </div>

            <div className="flex justify-end gap-2 mt-6">
              <button
                onClick={onCancelCreate}
                className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={onCreate}
                disabled={creating || !createName || !createUserId}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {creating ? "Creating..." : "Create"}
              </button>
            </div>
          </div>
        </div>
      )}

      {modal.kind === "created" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-lg w-full mx-4">
            <h3 className="font-semibold mb-2">API Key Created</h3>
            <p className="text-sm text-gray-600 dark:text-gray-300 mb-4">
              Copy this key now. It will not be shown again.
            </p>
            <div className="flex items-center gap-2 bg-gray-50 dark:bg-gray-800 border rounded p-3">
              <code className="flex-1 text-xs break-all font-mono">{modal.key}</code>
              <button
                onClick={() => onCopy(modal.key)}
                className="p-1.5 text-gray-400 dark:text-gray-300 hover:text-gray-600 dark:hover:text-gray-300 rounded hover:bg-gray-200 dark:hover:bg-gray-700 dark:bg-gray-700 shrink-0"
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
            <div className="mt-4 text-xs text-gray-500 dark:text-gray-400 space-y-0.5">
              <p><strong>Name:</strong> {modal.apiKey.name}</p>
              <p><strong>User:</strong> {userEmails[modal.apiKey.userId] || modal.apiKey.userId}</p>
              <p><strong>Scope:</strong> {modal.apiKey.scope === "*" ? "full access" : modal.apiKey.scope}</p>
              {modal.apiKey.appId && (
                <>
                  <p><strong>App:</strong> {appsById[modal.apiKey.appId]?.name || modal.apiKey.appId}</p>
                  <p><strong>Rate:</strong> {formatAppRateLimit(appsById[modal.apiKey.appId]).replace("Rate: ", "")}</p>
                </>
              )}
              {modal.apiKey.allowedTables && modal.apiKey.allowedTables.length > 0 && (
                <p><strong>Tables:</strong> {modal.apiKey.allowedTables.join(", ")}</p>
              )}
            </div>
            <div className="flex justify-end mt-6">
              <button
                onClick={onClose}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
              >
                Done
              </button>
            </div>
          </div>
        </div>
      )}

      {modal.kind === "revoke" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
            <h3 className="font-semibold mb-2">Revoke API Key</h3>
            <p className="text-sm text-gray-600 dark:text-gray-300 mb-1">
              This will permanently revoke the API key. Any applications using this key will lose access.
            </p>
            <p className="text-xs font-mono text-gray-500 dark:text-gray-400 break-all mb-4">
              {modal.apiKey.name} ({modal.apiKey.keyPrefix}...)
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={onClose}
                className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={() => onRevoke(modal.apiKey)}
                disabled={revoking}
                className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
              >
                {revoking ? "Revoking..." : "Revoke"}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
