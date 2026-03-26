import { useState } from "react";
import {
  listExtensions,
  enableExtension,
  disableExtension,
} from "../api_extensions";
import type { ExtensionInfo } from "../types/extensions";
import { useAdminResource } from "../hooks/useAdminResource";
import { AdminTable, type Column } from "./shared/AdminTable";
import { ConfirmDialog } from "./shared/ConfirmDialog";
import { StatusBadge } from "./shared/StatusBadge";

const extensionStatusMap: Record<string, "success" | "warning" | "default"> = {
  installed: "success",
  available: "warning",
};

export function Extensions() {
  const { data, loading, error, actionLoading, runAction } = useAdminResource(
    () => listExtensions().then((r) => r.extensions),
  );
  const [disableTarget, setDisableTarget] = useState<string | null>(null);

  const handleEnable = (name: string) =>
    runAction(() => enableExtension(name));

  const handleDisable = () => {
    if (!disableTarget) return;
    runAction(async () => {
      await disableExtension(disableTarget);
      setDisableTarget(null);
    });
  };

  const columns: Column<ExtensionInfo>[] = [
    { key: "name", header: "Name" },
    {
      key: "status",
      header: "Status",
      render: (row) => (
        <StatusBadge
          status={row.installed ? "installed" : "available"}
          variantMap={extensionStatusMap}
        />
      ),
    },
    {
      key: "installed_version",
      header: "Version",
      render: (row) => row.installed_version || row.default_version || "-",
    },
    {
      key: "comment",
      header: "Description",
      render: (row) => row.comment || "-",
    },
    {
      key: "actions",
      header: "",
      render: (row) =>
        row.installed ? (
          <button
            onClick={() => setDisableTarget(row.name)}
            disabled={actionLoading}
            className="text-xs text-red-500 hover:text-red-600 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Disable
          </button>
        ) : (
          <button
            onClick={() => handleEnable(row.name)}
            disabled={actionLoading}
            className="text-xs text-blue-500 hover:text-blue-600 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Enable
          </button>
        ),
    },
  ];

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Extensions
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
        Extensions
      </h2>

      {loading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading...</p>
      ) : (
        <AdminTable
          columns={columns}
          rows={data ?? []}
          rowKey="name"
          page={1}
          totalPages={1}
          onPageChange={() => {}}
          emptyMessage="No extensions available"
        />
      )}

      <ConfirmDialog
        open={disableTarget !== null}
        title="Disable Extension"
        message={`Disable extension ${disableTarget}? This may break dependent objects.`}
        confirmLabel="Disable"
        onConfirm={handleDisable}
        onCancel={() => setDisableTarget(null)}
        destructive
        loading={actionLoading}
      />
    </div>
  );
}
