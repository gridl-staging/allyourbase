import { useState, useEffect, useCallback } from "react";
import type {
  OAuthClientListResponse,
  OAuthClientResponse,
} from "../types";
import {
  listOAuthClients,
  createOAuthClient,
  revokeOAuthClient,
  rotateOAuthClientSecret,
} from "../api";
import { Plus, Loader2, AlertCircle, KeyRound } from "lucide-react";
import { useAppToast } from "./ToastProvider";
import { OAuthClientsTable } from "./OAuthClientsTable";
import {
  OAuthClientsModals,
  type OAuthClientsModal,
} from "./OAuthClientsModals";
import { useAppsById } from "./useAppsById";

const PER_PAGE = 20;

export function OAuthClients() {
  const [data, setData] = useState<OAuthClientListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [modal, setModal] = useState<OAuthClientsModal>({ kind: "none" });
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState(false);
  const [rotating, setRotating] = useState(false);
  const [copied, setCopied] = useState(false);

  const [createName, setCreateName] = useState("");
  const [createAppId, setCreateAppId] = useState("");
  const [createClientType, setCreateClientType] = useState("confidential");
  const [createRedirectUris, setCreateRedirectUris] = useState("");
  const [createScopes, setCreateScopes] = useState("readonly");
  const { appsById } = useAppsById({ failureMessage: null });

  const { addToast } = useAppToast();

  const fetchClients = useCallback(async () => {
    try {
      setError(null);
      const result = await listOAuthClients({ page, perPage: PER_PAGE });
      setData(result);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Failed to load OAuth clients");
      setData(null);
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchClients();
  }, [fetchClients]);

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
        .map((value) => value.trim())
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
    } catch (requestError) {
      addToast("error", requestError instanceof Error ? requestError.message : "Failed to create client");
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
    } catch (requestError) {
      addToast("error", requestError instanceof Error ? requestError.message : "Revoke failed");
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
    } catch (requestError) {
      addToast("error", requestError instanceof Error ? requestError.message : "Rotate failed");
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
      <div className="flex items-center justify-center h-64 text-gray-400 dark:text-gray-500">
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
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
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
        <div className="text-center py-16 border rounded-lg bg-gray-50 dark:bg-gray-800">
          <KeyRound className="w-10 h-10 text-gray-300 dark:text-gray-500 mx-auto mb-3" />
          <p className="text-gray-500 dark:text-gray-400 text-sm">No OAuth clients registered yet</p>
          <button
            onClick={() => setModal({ kind: "create" })}
            className="mt-3 text-sm text-blue-600 hover:underline"
          >
            Register your first client
          </button>
        </div>
      ) : data ? (
        <OAuthClientsTable
          data={data}
          page={page}
          appsById={appsById}
          onPageChange={setPage}
          onRevoke={(client) => setModal({ kind: "revoke", client })}
          onRotate={(client) => setModal({ kind: "rotate-confirm", client })}
        />
      ) : null}

      <OAuthClientsModals
        modal={modal}
        creating={creating}
        revoking={revoking}
        rotating={rotating}
        copied={copied}
        createName={createName}
        createAppId={createAppId}
        createClientType={createClientType}
        createRedirectUris={createRedirectUris}
        createScopes={createScopes}
        appsById={appsById}
        onClose={() => setModal({ kind: "none" })}
        onCreateNameChange={setCreateName}
        onCreateAppIdChange={setCreateAppId}
        onCreateClientTypeChange={setCreateClientType}
        onCreateRedirectUrisChange={setCreateRedirectUris}
        onCreateScopesChange={setCreateScopes}
        onCreate={handleCreate}
        onRevoke={handleRevoke}
        onRotate={handleRotate}
        onCopy={handleCopy}
        onCancelCreate={() => {
          setModal({ kind: "none" });
          resetCreateForm();
        }}
      />
    </div>
  );
}
