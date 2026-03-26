import { Fragment } from "react";
import { Loader2, Send } from "lucide-react";
import type { PushDelivery, PushDeliveryStatus } from "../../types";
import { cn } from "../../lib/utils";
import { DELIVERY_STATUS_OPTIONS, type DeliveryFilters } from "./models";
import { formatDate, statusBadgeClass } from "./helpers";

interface PushNotificationsDeliveriesTabProps {
  deliveryFilters: DeliveryFilters;
  setDeliveryFilters: React.Dispatch<React.SetStateAction<DeliveryFilters>>;
  deliveries: PushDelivery[];
  loadingDeliveries: boolean;
  deliveriesError: string | null;
  detailLoadingID: string | null;
  expandedDeliveryIDs: Set<string>;
  deliveryDetails: Record<string, PushDelivery>;
  onApplyDeliveryFilters: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  onOpenSend: () => void;
  onToggleDeliveryDetail: (id: string) => Promise<void>;
}

export function PushNotificationsDeliveriesTab({
  deliveryFilters,
  setDeliveryFilters,
  deliveries,
  loadingDeliveries,
  deliveriesError,
  detailLoadingID,
  expandedDeliveryIDs,
  deliveryDetails,
  onApplyDeliveryFilters,
  onOpenSend,
  onToggleDeliveryDetail,
}: PushNotificationsDeliveriesTabProps) {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4">
        <form onSubmit={onApplyDeliveryFilters} className="flex items-end gap-3">
          <div>
            <label htmlFor="push-deliveries-app-id" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">
              Filter App ID
            </label>
            <input
              id="push-deliveries-app-id"
              aria-label="Filter App ID"
              value={deliveryFilters.app_id}
              onChange={(event) => setDeliveryFilters((prev) => ({ ...prev, app_id: event.target.value }))}
              className="border rounded px-2 py-1.5 text-sm"
            />
          </div>
          <div>
            <label htmlFor="push-deliveries-user-id" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">
              Filter User ID
            </label>
            <input
              id="push-deliveries-user-id"
              aria-label="Filter User ID"
              value={deliveryFilters.user_id}
              onChange={(event) => setDeliveryFilters((prev) => ({ ...prev, user_id: event.target.value }))}
              className="border rounded px-2 py-1.5 text-sm"
            />
          </div>
          <div>
            <label htmlFor="push-deliveries-status" className="block text-xs text-gray-600 dark:text-gray-300 mb-1">
              Status
            </label>
            <select
              id="push-deliveries-status"
              aria-label="Status"
              value={deliveryFilters.status}
              onChange={(event) =>
                setDeliveryFilters((prev) => ({
                  ...prev,
                  status: event.target.value as "" | PushDeliveryStatus,
                }))
              }
              className="border rounded px-2 py-1.5 text-sm bg-white dark:bg-gray-800"
            >
              {DELIVERY_STATUS_OPTIONS.map((option) => (
                <option key={option.value || "all"} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>
          <button
            type="submit"
            className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
          >
            Apply Filters
          </button>
        </form>

        <button
          onClick={onOpenSend}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          <Send className="w-4 h-4" />
          Send Test Push
        </button>
      </div>

      {loadingDeliveries && deliveries.length === 0 ? (
        <div className="flex items-center justify-center h-40 text-gray-400 dark:text-gray-500">
          <Loader2 className="w-5 h-5 animate-spin mr-2" />
          Loading deliveries...
        </div>
      ) : deliveriesError && deliveries.length === 0 ? (
        <div className="text-center py-10 border rounded bg-red-50 text-sm text-red-600">{deliveriesError}</div>
      ) : deliveries.length === 0 ? (
        <div className="text-center py-10 border rounded bg-gray-50 dark:bg-gray-800 text-sm text-gray-500 dark:text-gray-400">
          No deliveries found
        </div>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Title</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">User</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Provider</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Status</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Error</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Sent At</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Created At</th>
                <th className="text-right px-4 py-2 font-medium text-gray-600 dark:text-gray-300">Details</th>
              </tr>
            </thead>
            <tbody>
              {deliveries.map((item) => {
                const detail = deliveryDetails[item.id] || item;
                const isExpanded = expandedDeliveryIDs.has(item.id);
                return (
                  <Fragment key={item.id}>
                    <tr className="border-b hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800">
                      <td className="px-4 py-2.5 text-xs text-gray-700 dark:text-gray-200">{item.title}</td>
                      <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{item.user_id}</td>
                      <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{item.provider}</td>
                      <td className="px-4 py-2.5">
                        <span className={cn("inline-block px-2 py-0.5 rounded text-xs", statusBadgeClass(item.status))}>
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{item.error_code || "-"}</td>
                      <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{formatDate(item.sent_at)}</td>
                      <td className="px-4 py-2.5 text-xs text-gray-600 dark:text-gray-300">{formatDate(item.created_at)}</td>
                      <td className="px-4 py-2.5 text-right">
                        <button
                          onClick={() => onToggleDeliveryDetail(item.id)}
                          className="px-2.5 py-1 text-xs border rounded hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
                          aria-label={`View delivery ${item.id}`}
                        >
                          {isExpanded ? "Hide" : "View"}
                        </button>
                      </td>
                    </tr>
                    {isExpanded ? (
                      <tr className="border-b last:border-0 bg-gray-50 dark:bg-gray-800">
                        <td colSpan={8} className="px-4 py-3">
                          {detailLoadingID === item.id ? (
                            <p className="text-xs text-gray-500 dark:text-gray-400">Loading detail...</p>
                          ) : (
                            <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                              <div>
                                <p className="font-medium text-gray-700 dark:text-gray-200 mb-1">Body</p>
                                <pre className="whitespace-pre-wrap border rounded bg-white dark:bg-gray-800 p-2">{detail.body}</pre>
                              </div>
                              <div>
                                <p className="font-medium text-gray-700 dark:text-gray-200 mb-1">Data Payload</p>
                                <div className="border rounded bg-white dark:bg-gray-800 p-2 space-y-1">
                                  {detail.data_payload && Object.keys(detail.data_payload).length > 0 ? (
                                    Object.entries(detail.data_payload).map(([key, value]) => (
                                      <div key={key} className="font-mono">
                                        "{key}": "{value}"
                                      </div>
                                    ))
                                  ) : (
                                    <div className="text-gray-500 dark:text-gray-400">No data payload</div>
                                  )}
                                </div>
                              </div>
                              <div>
                                <p className="font-medium text-gray-700 dark:text-gray-200 mb-1">Error Details</p>
                                <div className="border rounded bg-white dark:bg-gray-800 p-2 space-y-1">
                                  <div>
                                    <span className="font-medium">Code:</span> {detail.error_code || "-"}
                                  </div>
                                  <div>
                                    <span className="font-medium">Message:</span> {detail.error_message || "-"}
                                  </div>
                                </div>
                              </div>
                              <div>
                                <p className="font-medium text-gray-700 dark:text-gray-200 mb-1">Job ID</p>
                                <div className="border rounded bg-white dark:bg-gray-800 p-2 font-mono">{detail.job_id || "-"}</div>
                              </div>
                            </div>
                          )}
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
