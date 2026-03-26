import type {
  APIKeyListResponse,
  APIKeyResponse,
  AppResponse,
} from "../types";
import { ChevronLeft, ChevronRight, Trash2 } from "lucide-react";
import { cn } from "../lib/utils";
import { formatAppRateLimit } from "./api-keys-helpers";

interface ApiKeysTableProps {
  data: APIKeyListResponse;
  page: number;
  userEmails: Record<string, string>;
  appsById: Record<string, AppResponse>;
  onPageChange: (page: number) => void;
  onRevoke: (apiKey: APIKeyResponse) => void;
}

export function ApiKeysTable({
  data,
  page,
  userEmails,
  appsById,
  onPageChange,
  onRevoke,
}: ApiKeysTableProps) {
  return (
    <>
      <div className="border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-800 border-b">
            <tr>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Name</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Key</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Scope</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">User</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">App</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Last Used</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Created</th>
              <th className="text-center px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Status</th>
              <th className="text-right px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Actions</th>
            </tr>
          </thead>
          <tbody>
            {data.items.map((apiKey) => (
              <tr key={apiKey.id} className="border-b last:border-0 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800">
                <td className="px-4 py-2.5">
                  <span className="font-medium">{apiKey.name}</span>
                  <div className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">{apiKey.id}</div>
                </td>
                <td className="px-4 py-2.5">
                  <code className="text-xs bg-gray-100 dark:bg-gray-700 px-1.5 py-0.5 rounded text-gray-600 dark:text-gray-300">
                    {apiKey.keyPrefix}...
                  </code>
                </td>
                <td className="px-4 py-2.5">
                  <span
                    className={cn(
                      "inline-block px-1.5 py-0.5 rounded text-[10px] font-medium",
                      apiKey.scope === "*"
                        ? "bg-purple-100 text-purple-700"
                        : apiKey.scope === "readonly"
                          ? "bg-blue-100 text-blue-700"
                          : "bg-yellow-100 text-yellow-700",
                    )}
                  >
                    {apiKey.scope === "*" ? "full access" : apiKey.scope}
                  </span>
                  {apiKey.allowedTables && apiKey.allowedTables.length > 0 && (
                    <div className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">
                      {apiKey.allowedTables.join(", ")}
                    </div>
                  )}
                </td>
                <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400">
                  {userEmails[apiKey.userId] || apiKey.userId}
                </td>
                <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400">
                  {apiKey.appId ? (
                    <>
                      <div className="text-gray-700 dark:text-gray-200">
                        {appsById[apiKey.appId]?.name || apiKey.appId}
                      </div>
                      <div className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">
                        {formatAppRateLimit(appsById[apiKey.appId])}
                      </div>
                    </>
                  ) : (
                    <span className="text-gray-400 dark:text-gray-500">User-scoped</span>
                  )}
                </td>
                <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400">
                  {apiKey.lastUsedAt ? new Date(apiKey.lastUsedAt).toLocaleDateString() : "Never"}
                </td>
                <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400">
                  {new Date(apiKey.createdAt).toLocaleDateString()}
                </td>
                <td className="px-4 py-2.5 text-center">
                  <span
                    className={cn(
                      "inline-block px-2 py-0.5 rounded-full text-[10px] font-medium",
                      apiKey.revokedAt ? "bg-red-100 text-red-700" : "bg-green-100 text-green-700",
                    )}
                  >
                    {apiKey.revokedAt ? "Revoked" : "Active"}
                  </span>
                </td>
                <td className="px-4 py-2.5">
                  <div className="flex justify-end">
                    {!apiKey.revokedAt && (
                      <button
                        onClick={() => onRevoke(apiKey)}
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-red-500 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
                        title="Revoke key"
                        aria-label="Revoke key"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="mt-3 flex items-center justify-between text-sm text-gray-500 dark:text-gray-400">
        <span>
          {data.totalItems} key{data.totalItems !== 1 ? "s" : ""}
        </span>
        <div className="flex items-center gap-2">
          <button
            onClick={() => onPageChange(Math.max(1, page - 1))}
            disabled={page <= 1}
            className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-700 dark:bg-gray-700 disabled:opacity-30"
          >
            <ChevronLeft className="w-4 h-4" />
          </button>
          <span>
            {page} / {data.totalPages || 1}
          </span>
          <button
            onClick={() => onPageChange(Math.min(data.totalPages, page + 1))}
            disabled={page >= data.totalPages}
            className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-700 dark:bg-gray-700 disabled:opacity-30"
          >
            <ChevronRight className="w-4 h-4" />
          </button>
        </div>
      </div>
    </>
  );
}
