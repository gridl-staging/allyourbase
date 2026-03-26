import { useEffect, useMemo, useState } from "react";
import { getSecurityAdvisorReport } from "../api";
import type { AdvisorSeverity, SecurityFinding } from "../types";
import { usePolling } from "../hooks/usePolling";
import { toPanelError } from "./advisors/panelError";

const severityOrder: AdvisorSeverity[] = ["critical", "high", "medium", "low"];

export function SecurityAdvisor({ pollMs = 30000 }: { pollMs?: number }) {
  const initial = new URLSearchParams(window.location.search);
  const [severity, setSeverity] = useState<string>(initial.get("secSeverity") || "all");
  const [category, setCategory] = useState<string>(initial.get("secCategory") || "all");
  const [status, setStatus] = useState<string>(initial.get("secStatus") || "all");
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const { data, loading, error } = usePolling(() => getSecurityAdvisorReport(), pollMs);

  useEffect(() => {
    const url = new URL(window.location.href);
    const params = new URLSearchParams(url.search);
    if (severity === "all") params.delete("secSeverity"); else params.set("secSeverity", severity);
    if (category === "all") params.delete("secCategory"); else params.set("secCategory", category);
    if (status === "all") params.delete("secStatus"); else params.set("secStatus", status);
    url.search = params.toString();
    window.history.replaceState(null, "", url.toString());
  }, [severity, category, status]);

  const findings = useMemo(() => {
    const list = data?.findings || [];
    return list.filter((f) => {
      if (severity !== "all" && f.severity !== severity) return false;
      if (category !== "all" && f.category !== category) return false;
      if (status !== "all" && f.status !== status) return false;
      return true;
    });
  }, [data, severity, category, status]);

  const categories = useMemo(() => {
    return Array.from(new Set((data?.findings || []).map((f) => f.category))).sort();
  }, [data]);

  const stale = Boolean(data?.stale);

  return (
    <div className="p-6 space-y-4" data-testid="security-advisor-panel">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Security Advisor</h2>
      {loading && !data && <div className="text-sm text-gray-500">Loading security telemetry...</div>}
      {Boolean(error) && <div className="text-sm text-red-600">{toPanelError(error)}</div>}
      {stale && <div className="text-sm text-amber-600">Telemetry may be stale</div>}

      <div className="flex flex-wrap items-center gap-2 text-xs">
        <label className="flex items-center gap-1">Severity
          <select aria-label="Severity" value={severity} onChange={(e) => setSeverity(e.target.value)} className="border rounded px-2 py-1 bg-white dark:bg-gray-900">
            <option value="all">all</option>
            {severityOrder.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
        </label>
        <label className="flex items-center gap-1">Category
          <select aria-label="Category" value={category} onChange={(e) => setCategory(e.target.value)} className="border rounded px-2 py-1 bg-white dark:bg-gray-900">
            <option value="all">all</option>
            {categories.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        </label>
        <label className="flex items-center gap-1">Status
          <select aria-label="Status" value={status} onChange={(e) => setStatus(e.target.value)} className="border rounded px-2 py-1 bg-white dark:bg-gray-900">
            <option value="all">all</option>
            <option value="open">open</option>
            <option value="accepted">accepted</option>
            <option value="resolved">resolved</option>
          </select>
        </label>
      </div>

      {findings.length === 0 ? (
        <p className="text-xs text-gray-500">No findings for current filters.</p>
      ) : (
        severityOrder.map((sev) => {
          const rows = findings.filter((f) => f.severity === sev);
          if (!rows.length) return null;
          return (
            <section key={sev} className="rounded border border-gray-200 dark:border-gray-700 p-3">
              <h3 className="text-sm font-semibold capitalize mb-2">{sev}</h3>
              <ul className="space-y-2">
                {rows.map((f) => (
                  <li key={f.id}>
                    <button
                      type="button"
                      className="text-left w-full text-sm font-medium"
                      onClick={() => setExpandedId((prev) => prev === f.id ? null : f.id)}
                    >
                      {f.title}
                    </button>
                    {expandedId === f.id && <FindingDetails finding={f} />}
                  </li>
                ))}
              </ul>
            </section>
          );
        })
      )}

      <p className="text-[11px] text-gray-500">Last evaluated: {data?.evaluatedAt || "n/a"}</p>
    </div>
  );
}

function FindingDetails({ finding }: { finding: SecurityFinding }) {
  return (
    <div className="mt-1 rounded bg-gray-50 dark:bg-gray-900 p-2 text-xs space-y-1">
      <p>{finding.description}</p>
      <p className="text-gray-600 dark:text-gray-300">{finding.remediation}</p>
    </div>
  );
}
