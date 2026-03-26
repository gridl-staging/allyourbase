import { getStats } from "../api_stats";
import type { StatsOverview as StatsData } from "../types/stats";
import { usePolling } from "../hooks/usePolling";

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h ${mins}m`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

interface StatCardProps {
  label: string;
  value: string | number;
}

function statsCardTestID(label: string): string {
  return `stats-card-${label.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")}`;
}

function StatCard({ label, value }: StatCardProps) {
  return (
    <div
      data-testid={statsCardTestID(label)}
      className="bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 rounded p-4"
    >
      <p className="text-xs text-gray-500 dark:text-gray-400 mb-1">{label}</p>
      <p className="text-lg font-semibold text-gray-900 dark:text-gray-100">
        {value}
      </p>
    </div>
  );
}

export function StatsOverview() {
  const { data, error } = usePolling<StatsData>(getStats, 5000);

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Stats
        </h2>
        <p className="text-red-600 dark:text-red-400">
          {error instanceof Error ? error.message : "Failed to load"}
        </p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Stats
        </h2>
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading...</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
        Stats
      </h2>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <StatCard label="Uptime" value={formatUptime(data.uptime_seconds)} />
        <StatCard label="Go Version" value={data.go_version} />
        <StatCard label="Goroutines" value={data.goroutines} />
        <StatCard label="GC Cycles" value={data.gc_cycles} />
      </div>

      <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-3">
        Memory
      </h3>
      <div className="grid grid-cols-2 gap-4 mb-6">
        <StatCard label="Alloc" value={formatBytes(data.memory_alloc)} />
        <StatCard label="Sys" value={formatBytes(data.memory_sys)} />
      </div>

      {data.db_pool_max !== undefined && (
        <>
          <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-3">
            DB Pool
          </h3>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <StatCard label="Total" value={data.db_pool_total ?? 0} />
            <StatCard label="Idle" value={data.db_pool_idle ?? 0} />
            <StatCard label="In Use" value={data.db_pool_in_use ?? 0} />
            <StatCard label="Max" value={data.db_pool_max} />
          </div>
        </>
      )}
    </div>
  );
}
