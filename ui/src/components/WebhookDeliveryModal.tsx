import { useState, useEffect, useCallback } from "react";
import type { WebhookResponse, WebhookDelivery } from "../types";
import { listWebhookDeliveries } from "../api";
import {
  X,
  Loader2,
  AlertCircle,
  CheckCircle2,
  XCircle,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { cn } from "../lib/utils";

export interface DeliveryHistoryModalProps {
  webhook: WebhookResponse;
  onClose: () => void;
}

export function DeliveryHistoryModal({ webhook, onClose }: DeliveryHistoryModalProps) {
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(0);
  const [totalItems, setTotalItems] = useState(0);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const fetchDeliveries = useCallback(
    async (p: number) => {
      setLoading(true);
      setError(null);
      try {
        const res = await listWebhookDeliveries(webhook.id, {
          page: p,
          perPage: 20,
        });
        setDeliveries(res.items);
        setTotalPages(res.totalPages);
        setTotalItems(res.totalItems);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load deliveries");
      } finally {
        setLoading(false);
      }
    },
    [webhook.id],
  );

  useEffect(() => {
    fetchDeliveries(page);
  }, [fetchDeliveries, page]);

  const formatTime = (iso: string) => {
    const d = new Date(iso);
    return d.toLocaleString();
  };

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="webhook-delivery-history-title"
        className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-2xl mx-4 max-h-[80vh] flex flex-col"
      >
        <div className="flex items-center justify-between px-5 py-3 border-b shrink-0">
          <div>
            <h3 id="webhook-delivery-history-title" className="font-semibold">
              Delivery History
            </h3>
            <p className="text-xs text-gray-500 dark:text-gray-400 font-mono truncate max-w-md">
              {webhook.url}
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
            aria-label="Close"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="flex-1 overflow-auto p-4">
          {loading ? (
            <div className="flex items-center justify-center h-32 text-gray-400 dark:text-gray-500">
              <Loader2 className="w-5 h-5 animate-spin mr-2" />
              Loading deliveries...
            </div>
          ) : error ? (
            <div className="text-center py-8">
              <AlertCircle className="w-6 h-6 text-red-400 mx-auto mb-2" />
              <p className="text-red-600 text-sm">{error}</p>
            </div>
          ) : deliveries.length === 0 ? (
            <div className="text-center py-12 text-gray-400 dark:text-gray-500 text-sm">
              No deliveries recorded yet
            </div>
          ) : (
            <div className="space-y-1">
              {deliveries.map((del) => {
                const summaryId = `webhook-delivery-summary-${del.id}`;
                const detailId = `webhook-delivery-detail-${del.id}`;
                return (
                  <div key={del.id} className="border rounded">
                    <button
                      id={summaryId}
                      aria-expanded={expandedId === del.id}
                      aria-controls={detailId}
                      onClick={() =>
                        setExpandedId(expandedId === del.id ? null : del.id)
                      }
                      className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800 text-sm"
                    >
                      {del.success ? (
                        <CheckCircle2 className="w-4 h-4 text-green-500 shrink-0" />
                      ) : (
                        <XCircle className="w-4 h-4 text-red-500 shrink-0" />
                      )}
                      <span className="font-mono text-xs">
                        {del.statusCode || "ERR"}
                      </span>
                      <span
                        className={cn(
                          "px-1.5 py-0.5 rounded text-[10px] font-medium",
                          del.eventAction === "create" &&
                            "bg-green-100 text-green-700",
                          del.eventAction === "update" &&
                            "bg-blue-100 text-blue-700",
                          del.eventAction === "delete" &&
                            "bg-red-100 text-red-700",
                          del.eventAction === "test" &&
                            "bg-amber-100 text-amber-700",
                        )}
                      >
                        {del.eventAction}
                      </span>
                      <span className="text-gray-400 dark:text-gray-500 text-xs">
                        {del.eventTable}
                      </span>
                      <span className="ml-auto text-gray-400 dark:text-gray-500 text-xs flex items-center gap-2">
                        <span>{del.durationMs}ms</span>
                        <span>{formatTime(del.deliveredAt)}</span>
                        {expandedId === del.id ? (
                          <ChevronDown className="w-3 h-3" />
                        ) : (
                          <ChevronRight className="w-3 h-3" />
                        )}
                      </span>
                    </button>
                    {expandedId === del.id && (
                      <div
                        id={detailId}
                        data-testid={detailId}
                        role="region"
                        aria-labelledby={summaryId}
                        className="px-3 pb-3 border-t bg-gray-50 dark:bg-gray-800 space-y-2"
                      >
                        <div className="grid grid-cols-2 gap-2 text-xs pt-2">
                          <div>
                            <span className="text-gray-500 dark:text-gray-400">Attempt:</span>{" "}
                            {del.attempt}
                          </div>
                          <div>
                            <span className="text-gray-500 dark:text-gray-400">Duration:</span>{" "}
                            {del.durationMs}ms
                          </div>
                          <div>
                            <span className="text-gray-500 dark:text-gray-400">Status:</span>{" "}
                            {del.statusCode || "N/A"}
                          </div>
                          <div>
                            <span className="text-gray-500 dark:text-gray-400">Time:</span>{" "}
                            {formatTime(del.deliveredAt)}
                          </div>
                        </div>
                        {del.error && (
                          <div>
                            <p className="text-[10px] font-medium text-gray-500 dark:text-gray-400 mb-0.5">
                              Error
                            </p>
                            <pre className="text-xs bg-red-50 text-red-700 p-2 rounded border border-red-200 overflow-x-auto">
                              {del.error}
                            </pre>
                          </div>
                        )}
                        {del.requestBody && (
                          <div>
                            <p className="text-[10px] font-medium text-gray-500 dark:text-gray-400 mb-0.5">
                              Request Body
                            </p>
                            <pre className="text-xs bg-white dark:bg-gray-800 p-2 rounded border overflow-x-auto max-h-32">
                              {del.requestBody}
                            </pre>
                          </div>
                        )}
                        {del.responseBody && (
                          <div>
                            <p className="text-[10px] font-medium text-gray-500 dark:text-gray-400 mb-0.5">
                              Response Body
                            </p>
                            <pre className="text-xs bg-white dark:bg-gray-800 p-2 rounded border overflow-x-auto max-h-32">
                              {del.responseBody}
                            </pre>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {totalPages > 1 && (
          <div className="flex items-center justify-between px-5 py-3 border-t text-sm shrink-0">
            <span className="text-gray-500 dark:text-gray-400 text-xs">
              {totalItems} {totalItems === 1 ? "delivery" : "deliveries"}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
                className="px-2 py-1 text-xs border rounded disabled:opacity-40"
              >
                Previous
              </button>
              <span className="text-xs text-gray-500 dark:text-gray-400 py-1">
                {page} / {totalPages}
              </span>
              <button
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
                className="px-2 py-1 text-xs border rounded disabled:opacity-40"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
