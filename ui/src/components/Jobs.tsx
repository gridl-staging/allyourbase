import { useCallback, useEffect, useState } from "react";
import type { JobListResponse, JobResponse, JobState, QueueStats } from "../types";
import { cancelJob, getQueueStats, listJobs, retryJob } from "../api";
import { AlertCircle, Loader2, RefreshCw, XCircle } from "lucide-react";
import { cn } from "../lib/utils";
import { ToastContainer, useToast } from "./Toast";

const STATE_OPTIONS: Array<{ value: ""; label: string } | { value: JobState; label: string }> = [
  { value: "", label: "All states" },
  { value: "queued", label: "queued" },
  { value: "running", label: "running" },
  { value: "completed", label: "completed" },
  { value: "failed", label: "failed" },
  { value: "canceled", label: "canceled" },
];

function stateBadgeClass(state: JobState): string {
  switch (state) {
    case "queued":
      return "bg-blue-100 text-blue-700";
    case "running":
      return "bg-yellow-100 text-yellow-700";
    case "completed":
      return "bg-green-100 text-green-700";
    case "failed":
      return "bg-red-100 text-red-700";
    case "canceled":
      return "bg-gray-100 text-gray-700";
    default:
      return "bg-gray-100 text-gray-700";
  }
}

function lastErrorPreview(job: JobResponse): string {
  if (!job.lastError) return "-";
  return job.lastError.length > 90 ? `${job.lastError.slice(0, 90)}...` : job.lastError;
}

function formatDate(iso: string | null): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

interface AppliedFilters {
  state?: JobState;
  type?: string;
}

export function Jobs() {
  const [jobs, setJobs] = useState<JobListResponse | null>(null);
  const [stats, setStats] = useState<QueueStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [retryingId, setRetryingId] = useState<string | null>(null);
  const [cancelingId, setCancelingId] = useState<string | null>(null);

  const [stateFilter, setStateFilter] = useState<JobState | "">("");
  const [typeFilter, setTypeFilter] = useState("");
  const [appliedFilters, setAppliedFilters] = useState<AppliedFilters>({});

  const { toasts, addToast, removeToast } = useToast();

  const load = useCallback(
    async (filters: AppliedFilters) => {
      try {
        setError(null);
        const [jobsRes, statsRes] = await Promise.all([
          listJobs(filters),
          getQueueStats(),
        ]);
        setJobs(jobsRes);
        setStats(statsRes);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load jobs");
        setJobs(null);
      } finally {
        setLoading(false);
      }
    },
    [],
  );

  useEffect(() => {
    load(appliedFilters);
  }, [load, appliedFilters]);

  const handleApplyFilters = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const next: AppliedFilters = {};
    if (stateFilter) next.state = stateFilter;
    if (typeFilter.trim()) next.type = typeFilter.trim();
    setLoading(true);
    setAppliedFilters(next);
  };

  const handleRetry = async (job: JobResponse) => {
    setRetryingId(job.id);
    try {
      await retryJob(job.id);
      addToast("success", `Retried job ${job.id}`);
      await load(appliedFilters);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to retry job");
    } finally {
      setRetryingId(null);
    }
  };

  const handleCancel = async (job: JobResponse) => {
    setCancelingId(job.id);
    try {
      await cancelJob(job.id);
      addToast("success", `Canceled job ${job.id}`);
      await load(appliedFilters);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to cancel job");
    } finally {
      setCancelingId(null);
    }
  };

  if (loading && !jobs) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading jobs...
      </div>
    );
  }

  if (error && !jobs) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{error}</p>
          <button
            onClick={() => {
              setLoading(true);
              load(appliedFilters);
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
          <h1 className="text-lg font-semibold">Job Queue</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Manage background jobs and monitor queue health
          </p>
        </div>
      </div>

      {stats && (
        <div className="mb-4 grid grid-cols-2 md:grid-cols-3 xl:grid-cols-6 gap-2 text-xs">
          <div className="border rounded px-3 py-2 bg-gray-50">Queued: {stats.queued}</div>
          <div className="border rounded px-3 py-2 bg-gray-50">Running: {stats.running}</div>
          <div className="border rounded px-3 py-2 bg-gray-50">Completed: {stats.completed}</div>
          <div className="border rounded px-3 py-2 bg-gray-50">Failed: {stats.failed}</div>
          <div className="border rounded px-3 py-2 bg-gray-50">Canceled: {stats.canceled}</div>
          <div className="border rounded px-3 py-2 bg-gray-50">
            Oldest queued age: {stats.oldestQueuedAgeSec ?? "-"}s
          </div>
        </div>
      )}

      <form onSubmit={handleApplyFilters} className="mb-4 flex items-end gap-3">
        <div>
          <label htmlFor="jobs-state-filter" className="block text-xs text-gray-600 mb-1">
            State
          </label>
          <select
            id="jobs-state-filter"
            aria-label="State"
            value={stateFilter}
            onChange={(e) => setStateFilter(e.target.value as JobState | "")}
            className="border rounded px-2 py-1.5 text-sm bg-white"
          >
            {STATE_OPTIONS.map((opt) => (
              <option key={opt.value || "all"} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="jobs-type-filter" className="block text-xs text-gray-600 mb-1">
            Type
          </label>
          <input
            id="jobs-type-filter"
            aria-label="Type"
            value={typeFilter}
            onChange={(e) => setTypeFilter(e.target.value)}
            className="border rounded px-2 py-1.5 text-sm"
            placeholder="e.g. webhook_delivery_prune"
          />
        </div>
        <button
          type="submit"
          className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          Apply Filters
        </button>
      </form>

      {jobs && jobs.items.length === 0 ? (
        <div className="text-center py-12 border rounded-lg bg-gray-50 text-gray-500 text-sm">
          No jobs found for the selected filters
        </div>
      ) : jobs ? (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium text-gray-600">State</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Type</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Created</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Attempts</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Last Error</th>
                <th className="text-right px-4 py-2 font-medium text-gray-600">Actions</th>
              </tr>
            </thead>
            <tbody>
              {jobs.items.map((job) => (
                <tr key={job.id} className="border-b last:border-0 hover:bg-gray-50">
                  <td className="px-4 py-2.5">
                    <span
                      className={cn(
                        "inline-block px-2 py-0.5 rounded-full text-[10px] font-medium",
                        stateBadgeClass(job.state),
                      )}
                    >
                      {job.state}
                    </span>
                  </td>
                  <td className="px-4 py-2.5">
                    <code className="text-xs bg-gray-100 px-1.5 py-0.5 rounded text-gray-700">
                      {job.type}
                    </code>
                    <div className="text-[10px] text-gray-400 mt-0.5">{job.id}</div>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-500">
                    {formatDate(job.createdAt)}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-500">
                    {job.attempts} / {job.maxAttempts}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-500 max-w-[320px]">
                    {lastErrorPreview(job)}
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex justify-end gap-2">
                      {job.state === "failed" && (
                        <button
                          onClick={() => handleRetry(job)}
                          disabled={retryingId === job.id}
                          aria-label={`Retry job ${job.id}`}
                          className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded bg-blue-50 text-blue-700 hover:bg-blue-100 disabled:opacity-60"
                        >
                          <RefreshCw className="w-3.5 h-3.5" />
                          Retry
                        </button>
                      )}
                      {job.state === "queued" && (
                        <button
                          onClick={() => handleCancel(job)}
                          disabled={cancelingId === job.id}
                          aria-label={`Cancel job ${job.id}`}
                          className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded bg-red-50 text-red-700 hover:bg-red-100 disabled:opacity-60"
                        >
                          <XCircle className="w-3.5 h-3.5" />
                          Cancel
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
