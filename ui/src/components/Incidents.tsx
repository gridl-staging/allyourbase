import { useState } from "react";
import {
  listIncidents,
  createIncident,
  updateIncident,
  addIncidentUpdate,
} from "../api_incidents";
import type {
  Incident,
  IncidentStatus,
  IncidentUpdateEntry,
} from "../types/incidents";
import { useAdminResource } from "../hooks/useAdminResource";
import { AdminTable, type Column } from "./shared/AdminTable";
import { StatusBadge } from "./shared/StatusBadge";

const STATUSES: IncidentStatus[] = [
  "investigating",
  "identified",
  "monitoring",
  "resolved",
];

const statusMap: Record<string, "warning" | "error" | "info" | "success"> = {
  investigating: "warning",
  identified: "error",
  monitoring: "info",
  resolved: "success",
};

export function Incidents() {
  const [activeOnly, setActiveOnly] = useState(true);
  const { data, loading, error, actionLoading, runAction, refresh } =
    useAdminResource(() => listIncidents(activeOnly || undefined));

  const [showCreate, setShowCreate] = useState(false);
  const [title, setTitle] = useState("");
  const [status, setStatus] = useState<string>("investigating");
  const [services, setServices] = useState("");
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [updateMessage, setUpdateMessage] = useState("");
  const [updateStatus, setUpdateStatus] = useState<string>("investigating");

  const resetCreateForm = () => {
    setTitle("");
    setStatus("investigating");
    setServices("");
  };

  const handleCreate = () =>
    runAction(async () => {
      await createIncident({
        title: title.trim(),
        status,
        affected_services: services
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
      });
      setShowCreate(false);
      resetCreateForm();
    });

  const handleResolve = (id: string) =>
    runAction(async () => {
      await updateIncident(id, { status: "resolved" });
    });

  const handleAddUpdate = (id: string) =>
    runAction(async () => {
      await addIncidentUpdate(id, {
        message: updateMessage.trim(),
        status: updateStatus,
      });
      setUpdateMessage("");
    });

  const handleToggleFilter = () => {
    setActiveOnly(!activeOnly);
    // Trigger re-fetch via refresh after state updates
    setTimeout(() => refresh(), 0);
  };

  const columns: Column<Incident>[] = [
    { key: "title", header: "Title" },
    {
      key: "status",
      header: "Status",
      render: (row) => (
        <StatusBadge status={row.status} variantMap={statusMap} />
      ),
    },
    {
      key: "affectedServices",
      header: "Affected Services",
      render: (row) => row.affectedServices?.join(", ") || "-",
    },
    {
      key: "createdAt",
      header: "Created",
      render: (row) => new Date(row.createdAt).toLocaleString(),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <div className="flex gap-2">
          <button
            onClick={() =>
              setExpandedId(expandedId === row.id ? null : row.id)
            }
            className="text-xs text-blue-500 hover:text-blue-600"
          >
            Details
          </button>
          {row.status !== "resolved" && (
            <button
              onClick={() => handleResolve(row.id)}
              className="text-xs text-green-600 hover:text-green-700"
            >
              Resolve
            </button>
          )}
        </div>
      ),
    },
  ];

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Incidents
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  const expandedIncident = data?.find((i) => i.id === expandedId);

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          Incidents
        </h2>
        <div className="flex gap-2">
          <button
            onClick={handleToggleFilter}
            className="px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-300 border border-gray-300 dark:border-gray-600 rounded hover:bg-gray-50 dark:hover:bg-gray-800"
          >
            {activeOnly ? "Show All" : "Active Only"}
          </button>
          <button
            onClick={() => setShowCreate(true)}
            className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded"
          >
            Create Incident
          </button>
        </div>
      </div>

      {showCreate && (
        <div className="mb-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900">
          <div className="grid grid-cols-3 gap-3 mb-3">
            <input
              placeholder="Incident title"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
            />
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value)}
              className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
            >
              {STATUSES.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
            <input
              placeholder="Affected services (comma-separated)"
              value={services}
              onChange={(e) => setServices(e.target.value)}
              className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
            />
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleCreate}
              disabled={!title.trim() || actionLoading}
              className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded disabled:opacity-50"
            >
              Create
            </button>
            <button
              onClick={() => {
                setShowCreate(false);
                resetCreateForm();
              }}
              className="px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-300 border border-gray-300 dark:border-gray-600 rounded hover:bg-gray-50 dark:hover:bg-gray-800"
            >
              Cancel
            </button>
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
          emptyMessage="No incidents"
        />
      )}

      {expandedIncident && (
        <IncidentTimeline
          incident={expandedIncident}
          onAddUpdate={handleAddUpdate}
          updateMessage={updateMessage}
          setUpdateMessage={setUpdateMessage}
          updateStatus={updateStatus}
          setUpdateStatus={setUpdateStatus}
          actionLoading={actionLoading}
        />
      )}
    </div>
  );
}

function IncidentTimeline({
  incident,
  onAddUpdate,
  updateMessage,
  setUpdateMessage,
  updateStatus,
  setUpdateStatus,
  actionLoading,
}: {
  incident: Incident;
  onAddUpdate: (id: string) => void;
  updateMessage: string;
  setUpdateMessage: (v: string) => void;
  updateStatus: string;
  setUpdateStatus: (v: string) => void;
  actionLoading: boolean;
}) {
  return (
    <div className="mt-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-gray-50 dark:bg-gray-900">
      <h4 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-3">
        Timeline — {incident.title}
      </h4>
      {(incident.updates ?? []).length === 0 ? (
        <p className="text-xs text-gray-500 dark:text-gray-400 mb-3">
          No updates yet
        </p>
      ) : (
        <div className="space-y-2 mb-3">
          {(incident.updates ?? []).map((u: IncidentUpdateEntry) => (
            <div
              key={u.id}
              className="flex gap-3 text-xs text-gray-700 dark:text-gray-300"
            >
              <span className="text-gray-400 shrink-0">
                {new Date(u.createdAt).toLocaleString()}
              </span>
              <StatusBadge status={u.status} variantMap={statusMap} />
              <span>{u.message}</span>
            </div>
          ))}
        </div>
      )}

      <div className="flex gap-2 items-end">
        <textarea
          placeholder="Update message"
          value={updateMessage}
          onChange={(e) => setUpdateMessage(e.target.value)}
          rows={2}
          className="flex-1 px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
        />
        <select
          value={updateStatus}
          onChange={(e) => setUpdateStatus(e.target.value)}
          className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
        >
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <button
          onClick={() => onAddUpdate(incident.id)}
          disabled={!updateMessage.trim() || actionLoading}
          className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded disabled:opacity-50"
        >
          Add Update
        </button>
      </div>
    </div>
  );
}
