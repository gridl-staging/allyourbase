import { useState, useEffect, useCallback } from "react";
import type { SMSHealthResponse, SMSWindowStats } from "../types";
import { getSMSHealth } from "../api";
import { Loader2, AlertCircle } from "lucide-react";

const WINDOW_LABELS: { key: keyof Pick<SMSHealthResponse, "today" | "last_7d" | "last_30d">; label: string }[] = [
  { key: "today", label: "Today" },
  { key: "last_7d", label: "Last 7 Days" },
  { key: "last_30d", label: "Last 30 Days" },
];

function StatsCard({ windowKey, label, stats }: { windowKey: string; label: string; stats: SMSWindowStats }) {
  return (
    <div data-testid={`sms-stats-${windowKey}`} className="border rounded-lg p-4">
      <h3 className="text-sm font-medium text-gray-700 mb-3">{label}</h3>
      <div className="space-y-2 text-sm">
        <div className="flex justify-between">
          <span className="text-gray-500">Sent</span>
          <span className="font-medium">{stats.sent}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-gray-500">Confirmed</span>
          <span className="font-medium text-green-600">{stats.confirmed}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-gray-500">Failed</span>
          <span className="font-medium text-red-600">{stats.failed}</span>
        </div>
        <div className="flex justify-between border-t pt-2">
          <span className="text-gray-500">Conversion Rate</span>
          <span className="font-medium">{stats.conversion_rate.toFixed(1)}%</span>
        </div>
      </div>
    </div>
  );
}

export function SMSHealth() {
  const [data, setData] = useState<SMSHealthResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchHealth = useCallback(async () => {
    try {
      setError(null);
      setLoading(true);
      const res = await getSMSHealth();
      setData(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load SMS health");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchHealth();
  }, [fetchHealth]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading...
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{error}</p>
          <button
            onClick={fetchHealth}
            className="mt-2 text-sm text-blue-600 hover:underline"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (!data) return null;

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold">SMS Health</h2>
      {data.warning && (
        <div
          data-testid="sms-warning-badge"
          className="flex items-center gap-2 px-4 py-2 bg-amber-50 border border-amber-200 rounded-lg text-amber-800 text-sm"
        >
          <AlertCircle className="w-4 h-4 shrink-0" />
          {data.warning}
        </div>
      )}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {WINDOW_LABELS.map(({ key, label }) => (
          <StatsCard key={key} windowKey={key} label={label} stats={data[key]} />
        ))}
      </div>
    </div>
  );
}
