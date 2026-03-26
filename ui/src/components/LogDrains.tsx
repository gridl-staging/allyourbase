import { useState } from "react";
import { listDrains, createDrain, deleteDrain } from "../api_drains";
import type { DrainInfo } from "../types/drains";
import { useAdminResource } from "../hooks/useAdminResource";
import { AdminTable, type Column } from "./shared/AdminTable";
import { ConfirmDialog } from "./shared/ConfirmDialog";

export function LogDrains() {
  const { data, loading, error, actionLoading, runAction } =
    useAdminResource(listDrains);
  const [showCreate, setShowCreate] = useState(false);
  const [drainType, setDrainType] = useState("http");
  const [url, setUrl] = useState("");
  const [headersJson, setHeadersJson] = useState("");
  const [batchSize, setBatchSize] = useState("");
  const [flushInterval, setFlushInterval] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<DrainInfo | null>(null);

  const resetForm = () => {
    setDrainType("http");
    setUrl("");
    setHeadersJson("");
    setBatchSize("");
    setFlushInterval("");
    setFormError(null);
  };

  const parseHeaders = (): Record<string, string> | undefined => {
    if (!headersJson.trim()) {
      return undefined;
    }

    let parsed: unknown;
    try {
      parsed = JSON.parse(headersJson);
    } catch {
      throw new Error("Headers must be valid JSON");
    }

    if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") {
      throw new Error("Headers must be a JSON object");
    }

    return Object.entries(parsed).reduce<Record<string, string>>(
      (headers, [key, value]) => {
        if (typeof value !== "string") {
          throw new Error("Headers values must be strings");
        }
        headers[key] = value;
        return headers;
      },
      {},
    );
  };

  const handleCreate = () => {
    let headers: Record<string, string> | undefined;
    try {
      headers = parseHeaders();
      setFormError(null);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Invalid headers JSON");
      return;
    }

    runAction(async () => {
      await createDrain({
        type: drainType,
        url,
        headers,
        batch_size: batchSize ? Number(batchSize) : undefined,
        flush_interval_seconds: flushInterval
          ? Number(flushInterval)
          : undefined,
      });
      setShowCreate(false);
      resetForm();
    });
  };

  const handleDelete = () => {
    if (!deleteTarget) return;
    runAction(async () => {
      await deleteDrain(deleteTarget.id);
      setDeleteTarget(null);
    });
  };

  const columns: Column<DrainInfo>[] = [
    { key: "name", header: "Name" },
    {
      key: "sent",
      header: "Sent",
      render: (row) => String(row.stats.sent),
    },
    {
      key: "failed",
      header: "Failed",
      render: (row) => String(row.stats.failed),
    },
    {
      key: "dropped",
      header: "Dropped",
      render: (row) => String(row.stats.dropped),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <button
          aria-label={`Delete ${row.name}`}
          onClick={() => setDeleteTarget(row)}
          className="text-xs text-red-500 hover:text-red-600"
        >
          Delete
        </button>
      ),
    },
  ];

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Log Drains
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          Log Drains
        </h2>
        <button
          onClick={() => setShowCreate(true)}
          className="px-3 py-1.5 text-xs bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 rounded hover:bg-gray-800 dark:hover:bg-gray-200 font-medium"
        >
          Create Drain
        </button>
      </div>

      {showCreate && (
        <div className="mb-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900">
          <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-3">
            New Log Drain
          </h3>
          <div className="flex flex-col gap-2 max-w-md">
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Type
              <select
                value={drainType}
                onChange={(e) => setDrainType(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800"
              >
                <option value="http">HTTP</option>
                <option value="datadog">Datadog</option>
                <option value="loki">Loki</option>
              </select>
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              URL
              <input
                type="text"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://logs.example.com/ingest"
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Headers (JSON, optional)
              <textarea
                value={headersJson}
                onChange={(e) => {
                  setHeadersJson(e.target.value);
                  if (formError) {
                    setFormError(null);
                  }
                }}
                placeholder='{"Authorization":"Bearer token"}'
                rows={4}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 font-mono"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Batch Size
              <input
                type="number"
                value={batchSize}
                onChange={(e) => setBatchSize(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Flush Interval (seconds)
              <input
                type="number"
                value={flushInterval}
                onChange={(e) => setFlushInterval(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            {formError && (
              <p className="text-xs text-red-600 dark:text-red-400">{formError}</p>
            )}
            <div className="flex gap-2 mt-2">
              <button
                onClick={handleCreate}
                disabled={!url || actionLoading}
                className="px-3 py-1.5 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 font-medium disabled:opacity-50"
              >
                Create
              </button>
              <button
                onClick={() => {
                  setShowCreate(false);
                  resetForm();
                }}
                className="px-3 py-1.5 text-xs text-gray-600 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-200"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {loading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading...</p>
      ) : (
        <AdminTable
          columns={columns}
          rows={data ?? []}
          rowKey="id"
          page={1}
          totalPages={1}
          onPageChange={() => {}}
          emptyMessage="No log drains configured"
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete Drain"
        message={`Delete log drain ${deleteTarget?.name}? This cannot be undone.`}
        confirmLabel="Delete"
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
        destructive
        loading={actionLoading}
      />
    </div>
  );
}
