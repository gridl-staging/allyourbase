import { useCallback, useEffect, useState } from "react";
import { AlertCircle, Loader2 } from "lucide-react";
import type {
  BackupListResponse,
  PITRValidateResponse,
  RestoreJobListResponse,
} from "../types/backups";
import {
  abandonRestoreJob,
  listAllBackups,
  listRestoreJobs,
  restorePITR,
  triggerBackup,
  validatePITR,
} from "../api_backups";
import { useAppToast } from "./ToastProvider";
import { usePolling } from "../hooks/usePolling";
import { useDraftFilters } from "../hooks/useDraftFilters";
import { StatusBadge } from "./shared/StatusBadge";
import { ConfirmDialog } from "./shared/ConfirmDialog";
import { AdminTable, type Column } from "./shared/AdminTable";
import { FilterBar, type FilterField } from "./shared/FilterBar";
import { formatBytes } from "./shared/format";
import {
  buildPITRContextOptions,
  resolveSelectedPITRContextKey,
  type PITRContextOption,
} from "./backups/context";
import { PITRPanel } from "./backups/PITRPanel";
import { RestoreContextSelector } from "./backups/RestoreContextSelector";
import { RestoreJobsSection } from "./backups/RestoreJobsSection";
import { formatDate } from "./shared/format";
import { normalizeLocalDateTimeInput } from "./backups/format";

type BackupRow = BackupListResponse["backups"][number];
type RestoreJobRow = RestoreJobListResponse["jobs"][number];
interface RestoreJobsPollResult {
  contextKey: string;
  response: RestoreJobListResponse;
}

const STATUS_VARIANT_MAP: Record<string, "success" | "error" | "warning" | "info"> = {
  completed: "success",
  running: "warning",
  failed: "error",
  pending: "info",
};

const FILTER_FIELDS: FilterField[] = [
  {
    name: "status",
    label: "Status",
    type: "select",
    options: [
      { value: "", label: "All statuses" },
      { value: "completed", label: "Completed" },
      { value: "running", label: "Running" },
      { value: "failed", label: "Failed" },
      { value: "pending", label: "Pending" },
    ],
  },
  { name: "backup_type", label: "Type", type: "text", placeholder: "full" },
];

const INITIAL_BACKUP_FILTER_VALUES = {
  status: "",
  backup_type: "",
};

const BACKUP_COLUMNS: Column<BackupRow>[] = [
  {
    key: "status",
    header: "Status",
    render: (row) => <StatusBadge status={row.status} variantMap={STATUS_VARIANT_MAP} />,
  },
  {
    key: "backup_type",
    header: "Type",
    render: (row) => (
      <code className="text-xs bg-gray-100 dark:bg-gray-700 px-1.5 py-0.5 rounded">
        {row.backup_type}
      </code>
    ),
  },
  {
    key: "db_name",
    header: "Database",
  },
  {
    key: "size_bytes",
    header: "Size",
    render: (row) => formatBytes(row.size_bytes),
  },
  {
    key: "started_at",
    header: "Started",
    render: (row) => formatDate(row.started_at),
  },
  {
    key: "triggered_by",
    header: "Triggered By",
  },
];

function filterBackupsByType(backups: BackupRow[], backupType: string): BackupRow[] {
  const normalizedBackupType = backupType.trim().toLowerCase();
  if (!normalizedBackupType) {
    return backups;
  }

  return backups.filter((backup) => backup.backup_type.toLowerCase() === normalizedBackupType);
}

export function Backups() {
  const [data, setData] = useState<BackupListResponse | null>(null);
  const [pitrContexts, setPitrContexts] = useState<PITRContextOption[]>([]);
  const [selectedPitrContextKey, setSelectedPitrContextKey] = useState("");
  const [pitrTargetTime, setPitrTargetTime] = useState("");
  const [pitrValidation, setPitrValidation] = useState<PITRValidateResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [triggering, setTriggering] = useState(false);
  const [validating, setValidating] = useState(false);
  const [restoring, setRestoring] = useState(false);
  const [dryRun, setDryRun] = useState(false);
  const {
    draftValues: draftFilterValues,
    appliedValues: appliedFilterValues,
    setDraftValue: setDraftFilterValue,
    applyValues: applyFilterValues,
    resetValues: resetFilterValues,
  } = useDraftFilters(INITIAL_BACKUP_FILTER_VALUES);
  const [abandonTarget, setAbandonTarget] = useState<RestoreJobRow | null>(null);
  const [abandoning, setAbandoning] = useState(false);

  const pitrContext =
    pitrContexts.find((context) => context.key === selectedPitrContextKey) ?? null;
  const { addToast } = useAppToast();

  const load = useCallback(async (filters: Record<string, string>) => {
    try {
      setError(null);
      const result = await listAllBackups(
        filters.status ? { status: filters.status } : undefined,
      );
      const filteredBackups = filterBackupsByType(result.backups, filters.backup_type ?? "");
      setData({ ...result, backups: filteredBackups, total: filteredBackups.length });
      const nextContexts = buildPITRContextOptions(result.backups);
      setPitrContexts(nextContexts);
      setSelectedPitrContextKey((currentKey) =>
        resolveSelectedPITRContextKey(nextContexts, currentKey),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load backups");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load(appliedFilterValues);
  }, [appliedFilterValues, load]);

  const {
    data: restoreJobsData,
    refresh: refreshRestoreJobs,
  } = usePolling<RestoreJobsPollResult>(
    async () => {
      if (!pitrContext) {
        return {
          contextKey: "",
          response: { jobs: [], count: 0 },
        };
      }
      return {
        contextKey: pitrContext.key,
        response: await listRestoreJobs(pitrContext.projectId, pitrContext.databaseId),
      };
    },
    5000,
    {
      enabled: Boolean(pitrContext),
      refreshKey: pitrContext?.key ?? "no-context",
    },
  );

  const restoreJobs: RestoreJobListResponse = pitrContext
    ? restoreJobsData?.contextKey === pitrContext.key
      ? restoreJobsData.response
      : { jobs: [], count: 0 }
    : { jobs: [], count: 0 };

  useEffect(() => {
    setPitrValidation(null);
  }, [selectedPitrContextKey, pitrTargetTime]);

  const handleTrigger = async () => {
    setTriggering(true);
    try {
      await triggerBackup();
      addToast("success", "Backup triggered");
      await load(appliedFilterValues);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to trigger backup");
    } finally {
      setTriggering(false);
    }
  };

  const handleValidatePITR = async () => {
    if (!pitrContext || !pitrTargetTime) return;
    const normalizedTargetTime = normalizeLocalDateTimeInput(pitrTargetTime);
    if (!normalizedTargetTime) {
      addToast("error", "Target time must be a valid local date and time");
      return;
    }
    setValidating(true);
    try {
      const result = await validatePITR(
        pitrContext.projectId,
        pitrContext.databaseId,
        normalizedTargetTime,
      );
      setPitrValidation(result);
      addToast("success", "PITR window validated");
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to validate PITR window");
    } finally {
      setValidating(false);
    }
  };

  const handleRestore = async () => {
    if (!pitrContext || !pitrTargetTime) return;
    const normalizedTargetTime = normalizeLocalDateTimeInput(pitrTargetTime);
    if (!normalizedTargetTime) {
      addToast("error", "Target time must be a valid local date and time");
      return;
    }
    setRestoring(true);
    try {
      const result = await restorePITR(
        pitrContext.projectId,
        pitrContext.databaseId,
        normalizedTargetTime,
        dryRun,
      );
      if ("earliest_recoverable" in result) {
        setPitrValidation(result);
        addToast("success", "Dry run validated restore plan");
      } else {
        addToast("success", `Restore job started (${result.phase})`);
      }
      await refreshRestoreJobs();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to start restore");
    } finally {
      setRestoring(false);
    }
  };

  const handleAbandon = async () => {
    if (!abandonTarget) return;
    setAbandoning(true);
    try {
      await abandonRestoreJob(abandonTarget.id);
      addToast("success", "Restore job abandoned");
      setAbandonTarget(null);
      await refreshRestoreJobs();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to abandon job");
    } finally {
      setAbandoning(false);
    }
  };

  if (loading && !data) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading backups...
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
            onClick={() => { setLoading(true); load(appliedFilterValues); }}
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
          <h1 className="text-lg font-semibold">Backups & PITR</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
            Manage database backups and point-in-time recovery
          </p>
        </div>
        <button
          onClick={handleTrigger}
          disabled={triggering}
          className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 inline-flex items-center gap-1.5"
        >
          {triggering && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
          Trigger Backup
        </button>
      </div>

      <FilterBar
        fields={FILTER_FIELDS}
        values={draftFilterValues}
        onChange={setDraftFilterValue}
        onApply={(vals) => {
          setLoading(true);
          applyFilterValues(vals);
        }}
        onReset={() => {
          resetFilterValues();
          setLoading(true);
        }}
      />

      <AdminTable
        columns={BACKUP_COLUMNS}
        rows={data?.backups ?? []}
        rowKey="id"
        emptyMessage="No backups found"
      />

      <RestoreContextSelector
        contexts={pitrContexts}
        selectedContextKey={selectedPitrContextKey}
        onSelectContext={setSelectedPitrContextKey}
      />

      <PITRPanel
        hasContext={Boolean(pitrContext)}
        targetTime={pitrTargetTime}
        onTargetTimeChange={setPitrTargetTime}
        dryRun={dryRun}
        onDryRunChange={setDryRun}
        validating={validating}
        restoring={restoring}
        validationResult={pitrValidation}
        onValidate={handleValidatePITR}
        onRestore={handleRestore}
      />

      <RestoreJobsSection
        jobs={restoreJobs.jobs}
        onAbandonJob={setAbandonTarget}
        statusVariantMap={STATUS_VARIANT_MAP}
      />

      <ConfirmDialog
        open={abandonTarget !== null}
        title="Abandon Restore Job"
        message="This will cancel the in-progress restore operation. This action cannot be undone."
        confirmLabel="Abandon"
        destructive
        loading={abandoning}
        onConfirm={handleAbandon}
        onCancel={() => setAbandonTarget(null)}
      />
    </div>
  );
}
