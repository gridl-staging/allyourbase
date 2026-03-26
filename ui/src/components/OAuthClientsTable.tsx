import type {
  AppResponse,
  OAuthClientListResponse,
  OAuthClientResponse,
} from "../types";
import { ChevronLeft, ChevronRight, RefreshCw, Trash2 } from "lucide-react";
import { cn } from "../lib/utils";

interface OAuthClientsTableProps {
  data: OAuthClientListResponse;
  page: number;
  appsById: Record<string, AppResponse>;
  onPageChange: (page: number) => void;
  onRevoke: (client: OAuthClientResponse) => void;
  onRotate: (client: OAuthClientResponse) => void;
}

export function OAuthClientsTable({
  data,
  page,
  appsById,
  onPageChange,
  onRevoke,
  onRotate,
}: OAuthClientsTableProps) {
  return (
    <>
      <div className="border rounded-lg overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 dark:bg-gray-800 border-b">
            <tr>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Name</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Client ID</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Type</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">App</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Scopes</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Redirect URIs</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Created</th>
              <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Token Stats</th>
              <th className="text-center px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Status</th>
              <th className="text-right px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Actions</th>
            </tr>
          </thead>
          <tbody>
            {data.items.map((client) => (
              <tr key={client.id} className="border-b last:border-0 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800">
                <td className="px-4 py-2.5">
                  <span className="font-medium">{client.name}</span>
                </td>
                <td className="px-4 py-2.5">
                  <code className="text-xs bg-gray-100 dark:bg-gray-700 px-1.5 py-0.5 rounded text-gray-600 dark:text-gray-300">
                    {client.clientId}
                  </code>
                </td>
                <td className="px-4 py-2.5">
                  <span
                    className={cn(
                      "inline-block px-1.5 py-0.5 rounded text-[10px] font-medium",
                      client.clientType === "confidential"
                        ? "bg-purple-100 text-purple-700"
                        : "bg-blue-100 text-blue-700",
                    )}
                  >
                    {client.clientType}
                  </span>
                </td>
                <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400">
                  {appsById[client.appId]?.name || client.appId}
                </td>
                <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400">
                  {client.scopes.join(", ")}
                </td>
                <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400 max-w-[200px]">
                  {client.redirectUris.map((uri, index) => (
                    <div key={index} className="truncate">{uri}</div>
                  ))}
                </td>
                <td className="px-4 py-2.5 text-xs text-gray-500 dark:text-gray-400">
                  {new Date(client.createdAt).toLocaleDateString()}
                </td>
                <td className="px-4 py-2.5">
                  <div className="text-xs text-gray-700 dark:text-gray-200">
                    {`Access ${client.activeAccessTokenCount} / Refresh ${client.activeRefreshTokenCount} / Grants ${client.totalGrants}`}
                  </div>
                  <div className="text-[10px] text-gray-500 dark:text-gray-400 mt-0.5">
                    {`Last issued ${client.lastTokenIssuedAt ? new Date(client.lastTokenIssuedAt).toLocaleString() : "never"}`}
                  </div>
                </td>
                <td className="px-4 py-2.5 text-center">
                  <span
                    className={cn(
                      "inline-block px-2 py-0.5 rounded-full text-[10px] font-medium",
                      client.revokedAt ? "bg-red-100 text-red-700" : "bg-green-100 text-green-700",
                    )}
                  >
                    {client.revokedAt ? "Revoked" : "Active"}
                  </span>
                </td>
                <td className="px-4 py-2.5">
                  <div className="flex justify-end gap-1">
                    {!client.revokedAt && client.clientType === "confidential" && (
                      <button
                        onClick={() => onRotate(client)}
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-blue-500 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
                        title="Rotate secret"
                        aria-label="Rotate secret"
                      >
                        <RefreshCw className="w-3.5 h-3.5" />
                      </button>
                    )}
                    {!client.revokedAt && (
                      <button
                        onClick={() => onRevoke(client)}
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-red-500 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
                        title="Revoke client"
                        aria-label="Revoke client"
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
          {data.totalItems} client{data.totalItems !== 1 ? "s" : ""}
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
