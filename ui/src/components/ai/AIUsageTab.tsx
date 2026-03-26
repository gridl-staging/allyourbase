import type { DailyUsage, UsageSummary } from "../../types/ai";

interface AIUsageTabProps {
  usage: UsageSummary | null;
  dailyUsage: DailyUsage[];
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="border rounded px-4 py-3 bg-gray-50 dark:bg-gray-800">
      <div className="text-xs text-gray-500 dark:text-gray-400">{label}</div>
      <div className="text-lg font-semibold mt-0.5">{value}</div>
    </div>
  );
}

export function AIUsageTab({ usage, dailyUsage }: AIUsageTabProps) {
  if (!usage) return null;

  return (
    <div>
      <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-6 gap-3 mb-6">
        <StatCard label="Total Calls" value={String(usage.total_calls)} />
        <StatCard label="Total Tokens" value={usage.total_tokens.toLocaleString()} />
        <StatCard label="Input Tokens" value={usage.total_input_tokens.toLocaleString()} />
        <StatCard label="Output Tokens" value={usage.total_output_tokens.toLocaleString()} />
        <StatCard label="Total Cost" value={`$${usage.total_cost_usd.toFixed(2)}`} />
        <StatCard label="Errors" value={String(usage.error_count)} />
      </div>

      {Object.keys(usage.by_provider).length > 0 && (
        <>
          <h3 className="text-sm font-medium mb-2">By Provider</h3>
          <div className="border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 dark:bg-gray-800 border-b">
                <tr>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Provider</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Calls</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Tokens</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Cost</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Errors</th>
                </tr>
              </thead>
              <tbody>
                {Object.entries(usage.by_provider).map(([name, provider]) => (
                  <tr key={name} className="border-b last:border-0">
                    <td className="px-4 py-2.5 font-medium">{name}</td>
                    <td className="px-4 py-2.5">{provider.calls}</td>
                    <td className="px-4 py-2.5">{provider.total_tokens.toLocaleString()}</td>
                    <td className="px-4 py-2.5">${provider.total_cost_usd.toFixed(2)}</td>
                    <td className="px-4 py-2.5">{provider.error_count}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      <div className="mt-6">
        <h3 className="text-sm font-medium mb-2">Daily Usage</h3>
        {dailyUsage.length === 0 ? (
          <p className="text-xs text-gray-500 dark:text-gray-400">No daily usage records</p>
        ) : (
          <div className="border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 dark:bg-gray-800 border-b">
                <tr>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Day</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Provider</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Model</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Calls</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Tokens</th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Cost</th>
                </tr>
              </thead>
              <tbody>
                {dailyUsage.map((entry) => (
                  <tr
                    key={`${entry.day}-${entry.provider}-${entry.model}`}
                    className="border-b last:border-0"
                  >
                    <td className="px-4 py-2.5">
                      {new Date(entry.day).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-2.5">{entry.provider}</td>
                    <td className="px-4 py-2.5">{entry.model}</td>
                    <td className="px-4 py-2.5">{entry.calls}</td>
                    <td className="px-4 py-2.5">{entry.total_tokens.toLocaleString()}</td>
                    <td className="px-4 py-2.5">${entry.total_cost_usd.toFixed(2)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
