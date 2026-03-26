import { useState, useEffect, useCallback } from "react";
import type {
  APIKeyListResponse,
  APIKeyResponse,
} from "../types";
import { listApiKeys, createApiKey, revokeApiKey, listUsers } from "../api";
import { Plus, Loader2, AlertCircle, KeyRound } from "lucide-react";
import { useAppToast } from "./ToastProvider";
import { ApiKeysTable } from "./ApiKeysTable";
import { ApiKeysModals, type ApiKeysModal } from "./ApiKeysModals";
import { useAppsById } from "./useAppsById";

const PER_PAGE = 20;

export function ApiKeys() {
  const [data, setData] = useState<APIKeyListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [modal, setModal] = useState<ApiKeysModal>({ kind: "none" });
  const [revoking, setRevoking] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createUserId, setCreateUserId] = useState("");
  const [createScope, setCreateScope] = useState("*");
  const [createAllowedTables, setCreateAllowedTables] = useState("");
  const [createAppId, setCreateAppId] = useState("");
  const [userEmails, setUserEmails] = useState<Record<string, string>>({});
  const { appsById, error: appsError } = useAppsById();
  const [copied, setCopied] = useState(false);
  const { addToast } = useAppToast();

  const fetchKeys = useCallback(async () => {
    try {
      setError(null);
      const result = await listApiKeys({ page, perPage: PER_PAGE });
      setData(result);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Failed to load API keys");
      setData(null);
    } finally {
      setLoading(false);
    }
  }, [page]);

  useEffect(() => {
    fetchKeys();
  }, [fetchKeys]);

  useEffect(() => {
    listUsers({ perPage: 100 })
      .then((response) => {
        const map: Record<string, string> = {};
        for (const user of response.items) {
          map[user.id] = user.email;
        }
        setUserEmails(map);
      })
      .catch(() => {});
  }, []);

  const resetCreateForm = () => {
    setCreateName("");
    setCreateUserId("");
    setCreateScope("*");
    setCreateAllowedTables("");
    setCreateAppId("");
  };

  const handleCreate = async () => {
    if (!createUserId || !createName) return;
    setCreating(true);
    try {
      const tables = createAllowedTables
        .split(",")
        .map((value) => value.trim())
        .filter(Boolean);

      const result = await createApiKey({
        userId: createUserId,
        name: createName,
        scope: createScope,
        ...(createAppId ? { appId: createAppId } : {}),
        ...(tables.length > 0 ? { allowedTables: tables } : {}),
      });

      setModal({ kind: "created", key: result.key, apiKey: result.apiKey });
      resetCreateForm();
      addToast("success", `API key "${result.apiKey.name}" created`);
      fetchKeys();
    } catch (requestError) {
      addToast("error", requestError instanceof Error ? requestError.message : "Failed to create key");
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
    } catch (requestError) {
      addToast("error", requestError instanceof Error ? requestError.message : "Revoke failed");
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
      <div className="flex items-center justify-center h-64 text-gray-400 dark:text-gray-500">
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
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
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
        <div className="text-center py-16 border rounded-lg bg-gray-50 dark:bg-gray-800">
          <KeyRound className="w-10 h-10 text-gray-300 dark:text-gray-500 mx-auto mb-3" />
          <p className="text-gray-500 dark:text-gray-400 text-sm">No API keys created yet</p>
          <p className="text-gray-500 dark:text-gray-400 text-sm mt-1">
            Use API keys for service-to-service authentication between backend systems.
          </p>
          <button
            onClick={() => setModal({ kind: "create" })}
            className="mt-3 text-sm text-blue-600 hover:underline"
          >
            Create your first API key
          </button>
        </div>
      ) : data ? (
        <ApiKeysTable
          data={data}
          page={page}
          userEmails={userEmails}
          appsById={appsById}
          onPageChange={setPage}
          onRevoke={(apiKey) => setModal({ kind: "revoke", apiKey })}
        />
      ) : null}

      <ApiKeysModals
        modal={modal}
        creating={creating}
        revoking={revoking}
        copied={copied}
        createName={createName}
        createUserId={createUserId}
        createScope={createScope}
        createAllowedTables={createAllowedTables}
        createAppId={createAppId}
        userEmails={userEmails}
        appsById={appsById}
        appsError={appsError}
        onClose={() => setModal({ kind: "none" })}
        onCreateNameChange={setCreateName}
        onCreateUserIdChange={setCreateUserId}
        onCreateScopeChange={setCreateScope}
        onCreateAllowedTablesChange={setCreateAllowedTables}
        onCreateAppIdChange={setCreateAppId}
        onCreate={handleCreate}
        onRevoke={handleRevoke}
        onCopy={handleCopy}
        onCancelCreate={() => {
          setModal({ kind: "none" });
          resetCreateForm();
        }}
      />
    </div>
  );
}
