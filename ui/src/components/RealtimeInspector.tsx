import { useMemo, useState } from "react";
import { getRealtimeInspectorSnapshot } from "../api";
import { usePolling } from "../hooks/usePolling";
import { toPanelError } from "./advisors/panelError";

export function RealtimeInspector({ pollMs = 10000 }: { pollMs?: number }) {
  const [subFilter, setSubFilter] = useState("");

  const { data, loading, error, refresh } = usePolling(
    () => getRealtimeInspectorSnapshot(),
    pollMs,
  );

  const filteredSubs = useMemo(() => {
    const subs = data?.subscriptions || [];
    const lower = subFilter.trim().toLowerCase();
    return subs
      .filter((s) => !lower || s.name.toLowerCase().includes(lower))
      .sort((a, b) => b.count - a.count);
  }, [data, subFilter]);

  return (
    <div className="p-6 space-y-4" data-testid="realtime-inspector-panel">
      <div className="flex items-center gap-2">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Realtime Inspector</h2>
        <div className="ml-auto flex items-center gap-2">
          <button
            type="button"
            onClick={() => void refresh()}
            className="px-3 py-1 text-xs rounded bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900"
          >
            Refresh
          </button>
        </div>
      </div>

      {Boolean(error) && <div className="text-sm text-red-600">{toPanelError(error)}</div>}
      {loading && !data && <div className="text-sm text-gray-500">Loading realtime telemetry...</div>}

      <div className="grid grid-cols-2 lg:grid-cols-5 gap-2">
        <MetricCard label="Total" value={data?.connections.total ?? 0} testId="realtime-total-metric" />
        <MetricCard label="SSE" value={data?.connections.sse ?? 0} testId="realtime-sse-metric" />
        <MetricCard label="WebSocket" value={data?.connections.ws ?? 0} testId="realtime-ws-metric" />
        <MetricCard label="Dropped" value={data?.counters.droppedMessages ?? 0} testId="realtime-dropped-metric" />
        <MetricCard
          label="HB Failures"
          value={data?.counters.heartbeatFailures ?? 0}
          testId="realtime-heartbeat-failures-metric"
        />
      </div>

      <div className="rounded border border-gray-200 dark:border-gray-700 p-3">
        <div className="flex items-center mb-2">
          <h3 className="text-sm font-medium">Subscriptions</h3>
          <input
            aria-label="Filter subscriptions"
            placeholder="Filter subscriptions"
            value={subFilter}
            onChange={(e) => setSubFilter(e.target.value)}
            className="ml-auto text-xs border border-gray-300 dark:border-gray-700 rounded px-2 py-1 bg-white dark:bg-gray-900"
          />
        </div>
        {!data ? null : filteredSubs.length === 0 ? (
          <p className="text-xs text-gray-500">No active subscriptions</p>
        ) : (
          <table className="w-full text-xs">
            <thead>
              <tr className="text-left text-gray-500">
                <th className="py-1">Name</th>
                <th className="py-1">Type</th>
                <th className="py-1">Count</th>
              </tr>
            </thead>
            <tbody>
              {filteredSubs.map((s, i) => (
                <tr key={`${s.type}-${s.name}-${i}`}>
                  <td className="py-1">{s.name}</td>
                  <td className="py-1">
                    <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-medium bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400">
                      {s.type}
                    </span>
                  </td>
                  <td className="py-1">{s.count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

function MetricCard({ label, value, testId }: { label: string; value: number; testId: string }) {
  return (
    <div className="rounded border border-gray-200 dark:border-gray-700 p-3" data-testid={testId}>
      <div className="text-[11px] text-gray-500">{label}</div>
      <div className="text-lg font-semibold" data-testid={`${testId}-value`}>{value}</div>
    </div>
  );
}
