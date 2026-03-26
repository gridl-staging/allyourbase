import { useCallback, useEffect, useState } from "react";
import { AlertCircle, Loader2, Server, Trash2 } from "lucide-react";
import type { ReplicaStatus, AddReplicaRequest } from "../types/replicas";
import {
  listReplicas,
  checkReplicas,
  addReplica,
  removeReplica,
  promoteReplica,
  failover,
} from "../api_replicas";
import { useAppToast } from "./ToastProvider";
import { StatusBadge } from "./shared/StatusBadge";
import { AdminTable, type Column } from "./shared/AdminTable";
import { ConfirmDialog } from "./shared/ConfirmDialog";
import { formatBytes, formatDate } from "./shared/format";

const REPLICA_VARIANT_MAP: Record<string, "success" | "error" | "warning" | "info"> = {
  healthy: "success",
  lagging: "warning",
  down: "error",
  unknown: "info",
};

const COLUMNS: Column<ReplicaStatus>[] = [
  {
    key: "url",
    header: "URL",
    render: (row) => (
      <code className="text-xs break-all">{row.url}</code>
    ),
  },
  {
    key: "state",
    header: "State",
    render: (row) => <StatusBadge status={row.state} variantMap={REPLICA_VARIANT_MAP} />,
  },
  {
    key: "lag_bytes",
    header: "Lag",
    render: (row) => formatBytes(row.lag_bytes),
  },
  {
    key: "connections",
    header: "Connections",
    render: (row) => `${row.connections.in_use}/${row.connections.total}`,
  },
  {
    key: "last_checked_at",
    header: "Last Checked",
    render: (row) => formatDate(row.last_checked_at),
  },
];

type ConfirmAction =
  | { kind: "none" }
  | { kind: "remove"; name: string; url: string; requiresNameInput: boolean }
  | { kind: "promote"; name: string; url: string; requiresNameInput: boolean }
  | { kind: "failover" };

const EMPTY_FORM: AddReplicaRequest = {
  name: "",
  host: "",
  port: 5432,
  database: "",
  ssl_mode: "verify-full",
  weight: 100,
  max_lag_bytes: 0,
};

export function Replicas() {
  const [replicas, setReplicas] = useState<ReplicaStatus[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  const [showAddForm, setShowAddForm] = useState(false);
  const [form, setForm] = useState<AddReplicaRequest>({ ...EMPTY_FORM });
  const [adding, setAdding] = useState(false);
  const [confirmAction, setConfirmAction] = useState<ConfirmAction>({ kind: "none" });
  const [confirming, setConfirming] = useState(false);
  const [failoverInput, setFailoverInput] = useState("");

  const { addToast } = useAppToast();

  const closeConfirmDialog = () => {
    setConfirmAction({ kind: "none" });
    setFailoverInput("");
  };

  const updateReplicaActionName = (nextName: string) => {
    setConfirmAction((current) => {
      if (current.kind !== "remove" && current.kind !== "promote") {
        return current;
      }
      return { ...current, name: nextName };
    });
  };

  const load = useCallback(async () => {
    try {
      setError(null);
      const result = await listReplicas();
      setReplicas(result.replicas);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load replicas");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const handleCheck = async () => {
    setChecking(true);
    try {
      const result = await checkReplicas();
      setReplicas(result.replicas);
      addToast("success", "Health check completed");
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Health check failed");
    } finally {
      setChecking(false);
    }
  };

  const handleAdd = async () => {
    setAdding(true);
    try {
      const result = await addReplica(form);
      setReplicas(result.replicas);
      setShowAddForm(false);
      setForm({ ...EMPTY_FORM });
      addToast("success", `Replica ${result.record.name} added`);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to add replica");
    } finally {
      setAdding(false);
    }
  };

  const handleConfirm = async () => {
    setConfirming(true);
    try {
      if (confirmAction.kind === "remove") {
        const name = confirmAction.name.trim();
        if (!name) {
          addToast("error", "Replica name is required");
          return;
        }
        await removeReplica(name);
        addToast("success", "Replica removed");
      } else if (confirmAction.kind === "promote") {
        const name = confirmAction.name.trim();
        if (!name) {
          addToast("error", "Replica name is required");
          return;
        }
        const result = await promoteReplica(name);
        setReplicas(result.replicas);
        addToast("success", "Replica promoted");
      } else if (confirmAction.kind === "failover") {
        await failover({ target: "", force: false });
        addToast("success", "Failover completed");
      }
      closeConfirmDialog();
      await load();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Operation failed");
    } finally {
      setConfirming(false);
    }
  };

  const isFormValid = form.name && form.host && form.database;

  if (loading && !replicas) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading replicas...
      </div>
    );
  }

  if (error && !replicas) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{error}</p>
          <button
            onClick={() => { setLoading(true); load(); }}
            className="mt-2 text-sm text-blue-600 hover:underline"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  const actionColumns: Column<ReplicaStatus>[] = [
    ...COLUMNS,
    {
      key: "actions",
      header: "Actions",
      render: (row) => (
        <div className="flex gap-1">
          <button
            onClick={() =>
              setConfirmAction({
                kind: "promote",
                name: row.name,
                url: row.url,
                requiresNameInput: !row.name.trim(),
              })}
            className="px-2 py-1 text-xs rounded bg-blue-50 text-blue-700 hover:bg-blue-100"
            aria-label={`Promote ${row.name.trim() || row.url}`}
          >
            Promote
          </button>
          <button
            onClick={() =>
              setConfirmAction({
                kind: "remove",
                name: row.name,
                url: row.url,
                requiresNameInput: !row.name.trim(),
              })}
            className="p-1 text-gray-400 hover:text-red-500 rounded hover:bg-gray-100"
            aria-label={`Remove ${row.name.trim() || row.url}`}
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      ),
    },
  ];
  const isReplicaActionDialogOpen =
    confirmAction.kind === "remove" || confirmAction.kind === "promote";
  const replicaActionTitle =
    confirmAction.kind === "remove"
      ? "Remove Replica"
      : confirmAction.kind === "promote"
        ? "Promote Replica"
        : "";
  const replicaActionMessage =
    confirmAction.kind === "remove"
      ? confirmAction.requiresNameInput
        ? `Enter the replica name for ${confirmAction.url} before removing it. This will disconnect it from the pool.`
        : `Remove replica ${confirmAction.name}? This will disconnect it from the pool.`
      : confirmAction.kind === "promote"
        ? confirmAction.requiresNameInput
          ? `Enter the replica name for ${confirmAction.url} before promoting it to primary.`
          : `Promote ${confirmAction.name} to primary? Current primary will be demoted.`
        : "";
  const replicaActionConfirmLabel =
    confirmAction.kind === "remove"
      ? "Remove"
      : confirmAction.kind === "promote"
        ? "Promote"
        : "";
  const replicaActionName =
    confirmAction.kind === "remove" || confirmAction.kind === "promote"
      ? confirmAction.name
      : "";
  const replicaActionNeedsNameInput =
    confirmAction.kind === "remove" || confirmAction.kind === "promote"
      ? confirmAction.requiresNameInput
      : false;
  const replicaActionConfirmDisabled =
    isReplicaActionDialogOpen &&
    replicaActionNeedsNameInput &&
    !replicaActionName.trim();

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-lg font-semibold">Replicas</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
            Manage read replicas and replication topology
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={handleCheck}
            disabled={checking}
            className="px-3 py-1.5 text-sm border rounded hover:bg-gray-100 dark:hover:bg-gray-700 inline-flex items-center gap-1.5"
          >
            {checking ? (
              <Loader2 className="w-3.5 h-3.5 animate-spin" />
            ) : (
              <Server className="w-3.5 h-3.5" />
            )}
            Check Health
          </button>
          <button
            onClick={() => setShowAddForm(!showAddForm)}
            className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            Add Replica
          </button>
          <button
            onClick={() => setConfirmAction({ kind: "failover" })}
            className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700"
          >
            Failover
          </button>
        </div>
      </div>

      {showAddForm && (
        <div className="mb-6 p-4 border rounded-lg bg-gray-50 dark:bg-gray-800">
          <h3 className="text-sm font-medium mb-3">Add New Replica</h3>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            <div>
              <label htmlFor="replica-name" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">Name</label>
              <input id="replica-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="w-full border rounded px-2 py-1.5 text-sm" />
            </div>
            <div>
              <label htmlFor="replica-host" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">Host</label>
              <input id="replica-host" value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} className="w-full border rounded px-2 py-1.5 text-sm" />
            </div>
            <div>
              <label htmlFor="replica-port" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">Port</label>
              <input id="replica-port" type="number" value={form.port} onChange={(e) => setForm({ ...form, port: Number(e.target.value) })} className="w-full border rounded px-2 py-1.5 text-sm" />
            </div>
            <div>
              <label htmlFor="replica-database" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">Database</label>
              <input id="replica-database" value={form.database} onChange={(e) => setForm({ ...form, database: e.target.value })} className="w-full border rounded px-2 py-1.5 text-sm" />
            </div>
            <div>
              <label htmlFor="replica-ssl" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">SSL Mode</label>
              <select id="replica-ssl" value={form.ssl_mode} onChange={(e) => setForm({ ...form, ssl_mode: e.target.value })} className="w-full border rounded px-2 py-1.5 text-sm bg-white dark:bg-gray-800">
                <option value="disable">disable</option>
                <option value="prefer">prefer</option>
                <option value="require">require</option>
                <option value="verify-full">verify-full</option>
              </select>
            </div>
            <div>
              <label htmlFor="replica-weight" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">Weight</label>
              <input id="replica-weight" type="number" value={form.weight} onChange={(e) => setForm({ ...form, weight: Number(e.target.value) })} className="w-full border rounded px-2 py-1.5 text-sm" />
            </div>
            <div>
              <label htmlFor="replica-max-lag" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">Max Lag (bytes)</label>
              <input id="replica-max-lag" type="number" value={form.max_lag_bytes} onChange={(e) => setForm({ ...form, max_lag_bytes: Number(e.target.value) })} className="w-full border rounded px-2 py-1.5 text-sm" />
            </div>
          </div>
          <div className="flex justify-end gap-2 mt-3">
            <button onClick={() => { setShowAddForm(false); setForm({ ...EMPTY_FORM }); }} className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border">Cancel</button>
            <button onClick={handleAdd} disabled={!isFormValid || adding} className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 inline-flex items-center gap-1.5">
              {adding && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              Add
            </button>
          </div>
        </div>
      )}

      <AdminTable
        columns={actionColumns}
        rows={replicas ?? []}
        rowKey="url"
        emptyMessage="No replicas configured"
      />

      <ConfirmDialog
        open={isReplicaActionDialogOpen}
        title={replicaActionTitle}
        message={replicaActionMessage}
        confirmLabel={replicaActionConfirmLabel}
        destructive={confirmAction.kind === "remove"}
        loading={confirming}
        confirmDisabled={replicaActionConfirmDisabled}
        onConfirm={handleConfirm}
        onCancel={closeConfirmDialog}
      >
        {isReplicaActionDialogOpen && replicaActionNeedsNameInput ? (
          <div>
            <label
              htmlFor="replica-action-name"
              className="block text-xs text-gray-600 dark:text-gray-300 mb-1"
            >
              Replica name
            </label>
            <input
              id="replica-action-name"
              value={replicaActionName}
              onChange={(e) => updateReplicaActionName(e.target.value)}
              className="w-full border rounded px-2 py-1.5 text-sm"
              placeholder="replica-name"
              aria-label="Replica name"
            />
          </div>
        ) : null}
      </ConfirmDialog>

      <ConfirmDialog
        open={confirmAction.kind === "failover"}
        title="Failover"
        message="This will trigger an automatic failover to a replica. This is a topology-altering operation that may cause brief downtime."
        confirmLabel="Execute Failover"
        destructive
        loading={confirming}
        confirmDisabled={failoverInput !== "failover"}
        onConfirm={handleConfirm}
        onCancel={closeConfirmDialog}
      >
        <div>
          <p className="text-sm text-gray-600 dark:text-gray-300 mb-3">
            Type <strong>failover</strong> to confirm:
          </p>
          <input
            value={failoverInput}
            onChange={(e) => setFailoverInput(e.target.value)}
            className="w-full border rounded px-2 py-1.5 text-sm"
            placeholder="failover"
            aria-label="Type failover to confirm"
          />
        </div>
      </ConfirmDialog>
    </div>
  );
}
