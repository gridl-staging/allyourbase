import { useEffect, useMemo, useState } from "react";
import { getPerformanceAdvisorReport } from "../api";
import type { DashboardTimeRange, PerformanceQueryStat } from "../types";
import { usePolling } from "../hooks/usePolling";
import { toPanelError } from "./advisors/panelError";

const ranges: DashboardTimeRange[] = ["15m", "1h", "6h", "24h", "7d"];
const PAGE_SIZE = 20;

export function PerformanceAdvisor({ pollMs = 30000 }: { pollMs?: number }) {
  const initial = new URLSearchParams(window.location.search).get("perfRange") as DashboardTimeRange | null;
  const [range, setRange] = useState<DashboardTimeRange>(initial || "1h");
  const [selected, setSelected] = useState<PerformanceQueryStat | null>(null);
  const [page, setPage] = useState(1);

  const { data, loading, error } = usePolling(
    () => getPerformanceAdvisorReport({ range }),
    pollMs,
    { refreshKey: range },
  );

  useEffect(() => {
    const url = new URL(window.location.href);
    const params = new URLSearchParams(url.search);
    params.set("perfRange", range);
    url.search = params.toString();
    window.history.replaceState(null, "", url.toString());
  }, [range]);

  const queries = data?.queries || [];
  const totalPages = Math.max(1, Math.ceil(queries.length / PAGE_SIZE));
  const pageRows = useMemo(() => {
    const start = (page - 1) * PAGE_SIZE;
    return queries.slice(start, start + PAGE_SIZE);
  }, [page, queries]);

  useEffect(() => {
    if (page > totalPages) setPage(totalPages);
  }, [page, totalPages]);

  return (
    <div className="p-6 space-y-4" data-testid="performance-advisor-panel">
      <div className="flex items-center gap-2">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Performance Advisor</h2>
        <label className="ml-auto text-xs flex items-center gap-1">Time range
          <select
            aria-label="Time range"
            value={range}
            onChange={(e) => setRange(e.target.value as DashboardTimeRange)}
            className="border rounded px-2 py-1 bg-white dark:bg-gray-900"
          >
            {ranges.map((r) => <option key={r} value={r}>{r}</option>)}
          </select>
        </label>
      </div>

      {loading && !data && <div className="text-sm text-gray-500">Loading performance telemetry...</div>}
      {Boolean(error) && <div className="text-sm text-red-600">{toPanelError(error)}</div>}
      {data?.stale && <div className="text-sm text-amber-600">Telemetry may be stale</div>}

      {pageRows.length === 0 ? (
        <p className="text-xs text-gray-500">No slow queries</p>
      ) : (
        <table className="w-full text-xs border border-gray-200 dark:border-gray-700 rounded">
          <thead>
            <tr className="text-left text-gray-500">
              <th className="p-2">Fingerprint</th>
              <th className="p-2">Mean ms</th>
              <th className="p-2">Total ms</th>
              <th className="p-2">Calls</th>
              <th className="p-2">Rows</th>
            </tr>
          </thead>
          <tbody>
            {pageRows.map((q) => (
              <tr key={q.fingerprint} className="border-t border-gray-200 dark:border-gray-700">
                <td className="p-2">
                  <button type="button" className="underline" onClick={() => setSelected(q)}>
                    {q.fingerprint}
                  </button>
                </td>
                <td className="p-2">{q.meanMs}</td>
                <td className="p-2">{q.totalMs}</td>
                <td className="p-2">{q.calls}</td>
                <td className="p-2">{q.rows}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {totalPages > 1 && (
        <div className="flex items-center gap-2 text-xs">
          <button type="button" onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page <= 1} className="border rounded px-2 py-1">Prev</button>
          <span>{page}/{totalPages}</span>
          <button type="button" onClick={() => setPage((p) => Math.min(totalPages, p + 1))} disabled={page >= totalPages} className="border rounded px-2 py-1">Next</button>
        </div>
      )}

      {selected && (
        <section className="rounded border border-gray-200 dark:border-gray-700 p-3 text-xs space-y-1">
          <h3 className="text-sm font-semibold">Query Detail: {selected.fingerprint}</h3>
          <pre className="whitespace-pre-wrap text-[11px]">{selected.normalizedQuery}</pre>
          <p>Trend: {selected.trend}</p>
          <p>Endpoints:</p>
          <ul className="list-disc pl-4">
            {selected.endpoints.map((ep) => <li key={ep}>{ep}</li>)}
          </ul>
        </section>
      )}
    </div>
  );
}
