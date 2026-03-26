import { AdminTable, type Column } from "../shared/AdminTable";
import { StatusBadge } from "../shared/StatusBadge";
import type { RestoreJobListResponse } from "../../types/backups";
import { formatDate } from "../shared/format";

export type RestoreJobRow = RestoreJobListResponse["jobs"][number];

interface RestoreJobsSectionProps {
  jobs: RestoreJobRow[];
  onAbandonJob: (job: RestoreJobRow) => void;
  statusVariantMap: Record<string, "success" | "error" | "warning" | "info">;
}

export function RestoreJobsSection({
  jobs,
  onAbandonJob,
  statusVariantMap,
}: RestoreJobsSectionProps) {
  if (jobs.length === 0) {
    return null;
  }

  const columns: Column<RestoreJobRow>[] = [
    {
      key: "status",
      header: "Status",
      render: (row) => <StatusBadge status={row.status} variantMap={statusVariantMap} />,
    },
    { key: "phase", header: "Phase" },
    {
      key: "started_at",
      header: "Started",
      render: (row) => formatDate(row.started_at),
    },
    {
      key: "actions",
      header: "Actions",
      render: (row) =>
        row.status === "running" ? (
          <button
            onClick={() => onAbandonJob(row)}
            className="text-xs text-red-600 hover:underline"
          >
            Abandon Job
          </button>
        ) : null,
    },
  ];

  return (
    <div className="mt-8">
      <h2 className="text-md font-semibold mb-3">Restore Jobs</h2>
      <AdminTable columns={columns} rows={jobs} rowKey="id" />
    </div>
  );
}
