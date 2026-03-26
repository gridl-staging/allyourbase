import { Check, Copy } from "lucide-react";
import type {
  AppResponse,
  OAuthClientResponse,
} from "../types";

export type OAuthClientsModal =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "created"; secret: string; client: OAuthClientResponse }
  | { kind: "revoke"; client: OAuthClientResponse }
  | { kind: "rotate-confirm"; client: OAuthClientResponse }
  | { kind: "rotate-result"; secret: string; client: OAuthClientResponse };

interface OAuthClientsModalsProps {
  modal: OAuthClientsModal;
  creating: boolean;
  revoking: boolean;
  rotating: boolean;
  copied: boolean;
  createName: string;
  createAppId: string;
  createClientType: string;
  createRedirectUris: string;
  createScopes: string;
  appsById: Record<string, AppResponse>;
  onClose: () => void;
  onCreateNameChange: (value: string) => void;
  onCreateAppIdChange: (value: string) => void;
  onCreateClientTypeChange: (value: string) => void;
  onCreateRedirectUrisChange: (value: string) => void;
  onCreateScopesChange: (value: string) => void;
  onCreate: () => void;
  onRevoke: (client: OAuthClientResponse) => void;
  onRotate: (client: OAuthClientResponse) => void;
  onCopy: (value: string) => void;
  onCancelCreate: () => void;
}

export function OAuthClientsModals({
  modal,
  creating,
  revoking,
  rotating,
  copied,
  createName,
  createAppId,
  createClientType,
  createRedirectUris,
  createScopes,
  appsById,
  onClose,
  onCreateNameChange,
  onCreateAppIdChange,
  onCreateClientTypeChange,
  onCreateRedirectUrisChange,
  onCreateScopesChange,
  onCreate,
  onRevoke,
  onRotate,
  onCopy,
  onCancelCreate,
}: OAuthClientsModalsProps) {
  return (
    <>
      {modal.kind === "create" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-md w-full mx-4">
            <h3 className="font-semibold mb-4">Register OAuth Client</h3>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Name</label>
                <input
                  type="text"
                  value={createName}
                  onChange={(event) => onCreateNameChange(event.target.value)}
                  placeholder="e.g. Web Dashboard"
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Client name"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">App</label>
                <select
                  value={createAppId}
                  onChange={(event) => onCreateAppIdChange(event.target.value)}
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
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Client Type</label>
                <select
                  value={createClientType}
                  onChange={(event) => onCreateClientTypeChange(event.target.value)}
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Client type"
                >
                  <option value="confidential">Confidential (server-side)</option>
                  <option value="public">Public (SPA / native app)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Redirect URIs</label>
                <input
                  type="text"
                  value={createRedirectUris}
                  onChange={(event) => onCreateRedirectUrisChange(event.target.value)}
                  placeholder="https://example.com/callback"
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Redirect URIs"
                />
                <p className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">
                  Comma-separated. HTTPS required in production; localhost allowed for development.
                </p>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Scopes</label>
                <select
                  value={createScopes}
                  onChange={(event) => onCreateScopesChange(event.target.value)}
                  className="w-full border rounded px-3 py-1.5 text-sm"
                  aria-label="Scopes"
                >
                  <option value="readonly">Read only</option>
                  <option value="readwrite">Read &amp; write</option>
                  <option value="*">Full access (*)</option>
                </select>
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
                disabled={creating || !createName || !createAppId}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {creating ? "Registering..." : "Register"}
              </button>
            </div>
          </div>
        </div>
      )}

      {modal.kind === "created" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-lg w-full mx-4">
            <h3 className="font-semibold mb-2">OAuth Client Registered</h3>
            <div className="mt-4 text-xs text-gray-500 dark:text-gray-400 space-y-1">
              <p><strong>Client ID:</strong></p>
              <code className="block text-xs bg-gray-50 dark:bg-gray-800 border rounded p-2 break-all font-mono">
                {modal.client.clientId}
              </code>
            </div>

            {modal.secret && (
              <div className="mt-4">
                <p className="text-sm text-gray-600 dark:text-gray-300 mb-2">
                  Copy this secret now. It will not be shown again.
                </p>
                <div className="flex items-center gap-2 bg-gray-50 dark:bg-gray-800 border rounded p-3">
                  <code className="flex-1 text-xs break-all font-mono">{modal.secret}</code>
                  <button
                    onClick={() => onCopy(modal.secret)}
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
              </div>
            )}

            <div className="mt-4 text-xs text-gray-500 dark:text-gray-400 space-y-0.5">
              <p><strong>Name:</strong> {modal.client.name}</p>
              <p><strong>Type:</strong> {modal.client.clientType}</p>
              <p><strong>Scopes:</strong> {modal.client.scopes.join(", ")}</p>
              <p><strong>Redirect URIs:</strong> {modal.client.redirectUris.join(", ")}</p>
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
            <h3 className="font-semibold mb-2">Revoke OAuth Client</h3>
            <p className="text-sm text-gray-600 dark:text-gray-300 mb-1">
              This will revoke the OAuth client and invalidate all tokens issued to it.
            </p>
            <p className="text-xs font-mono text-gray-500 dark:text-gray-400 break-all mb-4">
              {modal.client.name} ({modal.client.clientId})
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={onClose}
                className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={() => onRevoke(modal.client)}
                disabled={revoking}
                className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
              >
                {revoking ? "Revoking..." : "Revoke"}
              </button>
            </div>
          </div>
        </div>
      )}

      {modal.kind === "rotate-confirm" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
            <h3 className="font-semibold mb-2">Rotate Client Secret</h3>
            <p className="text-sm text-gray-600 dark:text-gray-300 mb-1">
              This will invalidate the current secret. Any application using the old secret will stop working.
            </p>
            <p className="text-xs font-mono text-gray-500 dark:text-gray-400 break-all mb-4">
              {modal.client.name}
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={onClose}
                className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={() => onRotate(modal.client)}
                disabled={rotating}
                className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {rotating ? "Rotating..." : "Rotate"}
              </button>
            </div>
          </div>
        </div>
      )}

      {modal.kind === "rotate-result" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-lg w-full mx-4">
            <h3 className="font-semibold mb-2">New Client Secret</h3>
            <p className="text-sm text-gray-600 dark:text-gray-300 mb-4">
              Copy this secret now. It will not be shown again.
            </p>
            <div className="flex items-center gap-2 bg-gray-50 dark:bg-gray-800 border rounded p-3">
              <code className="flex-1 text-xs break-all font-mono">{modal.secret}</code>
              <button
                onClick={() => onCopy(modal.secret)}
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
    </>
  );
}
