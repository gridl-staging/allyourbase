import { useState } from "react";
import {
  listDomains,
  createDomain,
  deleteDomain,
  verifyDomain,
} from "../api_domains";
import type { DomainBinding } from "../types/domains";
import { useAdminResource } from "../hooks/useAdminResource";
import { AdminTable, type Column } from "./shared/AdminTable";
import { ConfirmDialog } from "./shared/ConfirmDialog";
import { StatusBadge } from "./shared/StatusBadge";

const domainStatusMap: Record<string, "success" | "error" | "warning" | "info" | "default"> = {
  active: "success",
  verified: "success",
  pending_verification: "warning",
  verification_failed: "error",
  tombstoned: "default",
  verification_lapsed: "error",
};

export function CustomDomains() {
  const { data, loading, error, actionLoading, runAction } = useAdminResource(
    () => listDomains().then((r) => r.items),
  );
  const [showCreate, setShowCreate] = useState(false);
  const [hostname, setHostname] = useState("");
  const [environment, setEnvironment] = useState("production");
  const [redirectMode, setRedirectMode] = useState("none");
  const [deleteTarget, setDeleteTarget] = useState<DomainBinding | null>(null);

  const resetForm = () => {
    setHostname("");
    setEnvironment("production");
    setRedirectMode("none");
  };

  const handleCreate = () =>
    runAction(async () => {
      await createDomain({ hostname, environment, redirectMode });
      setShowCreate(false);
      resetForm();
    });

  const handleDelete = () => {
    if (!deleteTarget) return;
    runAction(async () => {
      await deleteDomain(deleteTarget.id);
      setDeleteTarget(null);
    });
  };

  const handleVerify = (domain: DomainBinding) =>
    runAction(() => verifyDomain(domain.id).then(() => {}));

  const columns: Column<DomainBinding>[] = [
    { key: "hostname", header: "Hostname" },
    {
      key: "status",
      header: "Status",
      render: (row) => (
        <StatusBadge status={row.status} variantMap={domainStatusMap} />
      ),
    },
    { key: "environment", header: "Environment" },
    {
      key: "verificationRecord",
      header: "DNS TXT Record",
      className: "px-4 py-2.5 font-mono text-xs break-all",
      render: (row) => row.verificationRecord ?? "-",
    },
    {
      key: "certExpiry",
      header: "Cert Expiry",
      render: (row) =>
        row.certExpiry ? new Date(row.certExpiry).toLocaleDateString() : "-",
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <div className="flex gap-2">
          {row.status === "pending_verification" && (
            <button
              onClick={() => handleVerify(row)}
              disabled={actionLoading}
              className="text-xs text-blue-500 hover:text-blue-600 disabled:opacity-50"
            >
              Verify
            </button>
          )}
          <button
            aria-label={`Delete ${row.hostname}`}
            onClick={() => setDeleteTarget(row)}
            className="text-xs text-red-500 hover:text-red-600"
          >
            Delete
          </button>
        </div>
      ),
    },
  ];

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Custom Domains
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          Custom Domains
        </h2>
        <button
          onClick={() => setShowCreate(true)}
          className="px-3 py-1.5 text-xs bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 rounded hover:bg-gray-800 dark:hover:bg-gray-200 font-medium"
        >
          Add Domain
        </button>
      </div>

      {showCreate && (
        <div className="mb-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900">
          <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-3">
            New Domain
          </h3>
          <div className="flex flex-col gap-2 max-w-md">
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Hostname
              <input
                type="text"
                value={hostname}
                onChange={(e) => setHostname(e.target.value)}
                placeholder="api.example.com"
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Environment
              <select
                value={environment}
                onChange={(e) => setEnvironment(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800"
              >
                <option value="production">Production</option>
                <option value="staging">Staging</option>
                <option value="development">Development</option>
              </select>
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Redirect Mode
              <select
                value={redirectMode}
                onChange={(e) => setRedirectMode(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800"
              >
                <option value="none">None</option>
                <option value="https">HTTPS</option>
              </select>
            </label>
            <div className="flex gap-2 mt-2">
              <button
                onClick={handleCreate}
                disabled={!hostname || actionLoading}
                className="px-3 py-1.5 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 font-medium disabled:opacity-50"
              >
                Add
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
          emptyMessage="No custom domains configured"
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete Domain"
        message={`Delete domain ${deleteTarget?.hostname}? This cannot be undone.`}
        confirmLabel="Delete"
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
        destructive
        loading={actionLoading}
      />
    </div>
  );
}
