import { Plus, Loader2, AlertCircle, Zap } from "lucide-react";
import { cn } from "../../lib/utils";
import { formatTimeout, formatLastInvoked } from "./helpers";
import type { EdgeFunctionResponse } from "../../types";

interface FunctionListProps {
  functions: EdgeFunctionResponse[];
  loading: boolean;
  error: string | null;
  onSelect: (id: string) => void;
  onCreate: () => void;
}

export function FunctionList({ functions, loading, error, onSelect, onCreate }: FunctionListProps) {
  return (
    <>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold">Edge Functions</h1>
        <button
          onClick={onCreate}
          className="flex items-center gap-1.5 px-3 py-1.5 bg-gray-900 text-white rounded text-sm hover:bg-gray-800"
        >
          <Plus className="w-4 h-4" />
          New Function
        </button>
      </div>

      {loading && (
        <div className="flex items-center justify-center py-12 text-gray-400 dark:text-gray-500">
          <Loader2 className="w-5 h-5 animate-spin mr-2" />
          Loading edge functions...
        </div>
      )}

      {error && (
        <div className="flex items-center gap-2 text-red-600 bg-red-50 border border-red-200 rounded-lg p-4">
          <AlertCircle className="w-5 h-5 shrink-0" />
          <span className="text-sm">{error}</span>
        </div>
      )}

      {!loading && !error && functions.length === 0 && (
        <div className="text-center py-12">
          <Zap className="w-10 h-10 text-gray-200 mx-auto mb-3" />
          <p className="text-gray-500 dark:text-gray-400 mb-1">No edge functions deployed yet</p>
          <p className="text-xs text-gray-400 dark:text-gray-500 mb-4">
            Deploy your first edge function to get started.
          </p>
          <button
            onClick={onCreate}
            className="px-4 py-2 bg-gray-900 text-white rounded text-sm hover:bg-gray-800"
          >
            Deploy Function
          </button>
        </div>
      )}

      {!loading && !error && functions.length > 0 && (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Name</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Access</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Timeout</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Created</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Last Invoked</th>
              </tr>
            </thead>
            <tbody>
              {functions.map((fn) => (
                <tr
                  key={fn.id}
                  className="border-b last:border-0 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800 cursor-pointer"
                  onClick={() => onSelect(fn.id)}
                >
                  <td className="px-4 py-2.5 font-medium text-blue-600 hover:underline">
                    {fn.name}
                  </td>
                  <td className="px-4 py-2.5">
                    <span
                      data-testid={`fn-public-${fn.id}`}
                      className={cn(
                        "px-2 py-0.5 rounded text-xs font-medium",
                        fn.public
                          ? "bg-green-100 text-green-700"
                          : "bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300",
                      )}
                    >
                      {fn.public ? "Public" : "Private"}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-gray-600 dark:text-gray-300">
                    {formatTimeout(fn.timeout)}
                  </td>
                  <td className="px-4 py-2.5 text-gray-500 dark:text-gray-400 text-xs">
                    {new Date(fn.createdAt).toLocaleDateString()}
                  </td>
                  <td className="px-4 py-2.5 text-gray-500 dark:text-gray-400 text-xs">
                    {formatLastInvoked(fn.lastInvokedAt)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
