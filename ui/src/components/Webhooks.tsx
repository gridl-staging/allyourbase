import { useState, useEffect, useCallback } from "react";
import type { WebhookResponse, WebhookRequest, WebhookDelivery } from "../types";
import {
  listWebhooks,
  createWebhook,
  updateWebhook,
  deleteWebhook,
  testWebhook,
  listWebhookDeliveries,
} from "../api";
import {
  Plus,
  Trash2,
  Pencil,
  Lock,
  Copy,
  X,
  Loader2,
  AlertCircle,
  Webhook,
  Zap,
  History,
  CheckCircle2,
  XCircle,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { cn } from "../lib/utils";
import { ToastContainer, useToast } from "./Toast";

type Modal =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "edit"; webhook: WebhookResponse }
  | { kind: "delete"; webhook: WebhookResponse }
  | { kind: "deliveries"; webhook: WebhookResponse };

export function Webhooks() {
  const [webhooks, setWebhooks] = useState<WebhookResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [modal, setModal] = useState<Modal>({ kind: "none" });
  const [testingId, setTestingId] = useState<string | null>(null);
  const { toasts, addToast, removeToast } = useToast();

  const fetchWebhooks = useCallback(async () => {
    try {
      setError(null);
      const data = await listWebhooks();
      setWebhooks(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load webhooks");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchWebhooks();
  }, [fetchWebhooks]);

  const handleToggleEnabled = async (hook: WebhookResponse) => {
    try {
      await updateWebhook(hook.id, { enabled: !hook.enabled });
      setWebhooks((prev) =>
        prev.map((w) =>
          w.id === hook.id ? { ...w, enabled: !w.enabled } : w,
        ),
      );
      addToast("success", `Webhook ${hook.enabled ? "disabled" : "enabled"}`);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Update failed");
    }
  };

  const handleDelete = async (hook: WebhookResponse) => {
    try {
      await deleteWebhook(hook.id);
      setWebhooks((prev) => prev.filter((w) => w.id !== hook.id));
      setModal({ kind: "none" });
      addToast("success", "Webhook deleted");
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Delete failed");
    }
  };

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    addToast("success", `${label} copied`);
  };

  const handleTest = async (hook: WebhookResponse) => {
    setTestingId(hook.id);
    try {
      const result = await testWebhook(hook.id);
      if (result.success) {
        addToast("success", `Test passed (${result.statusCode} in ${result.durationMs}ms)`);
      } else {
        addToast("error", result.error || `Test failed (${result.statusCode})`);
      }
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Test request failed");
    } finally {
      setTestingId(null);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading webhooks...
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
            onClick={() => {
              setLoading(true);
              fetchWebhooks();
            }}
            className="mt-2 text-sm text-blue-600 hover:underline"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-lg font-semibold">Webhooks</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Manage event notifications sent to external URLs
          </p>
        </div>
        <button
          onClick={() => setModal({ kind: "create" })}
          className="flex items-center gap-1.5 px-3 py-1.5 bg-gray-900 text-white text-sm rounded hover:bg-gray-800"
        >
          <Plus className="w-4 h-4" />
          Add Webhook
        </button>
      </div>

      {webhooks.length === 0 ? (
        <div className="text-center py-16 border rounded-lg bg-gray-50">
          <Webhook className="w-10 h-10 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500 text-sm mb-3">
            No webhooks configured yet
          </p>
          <button
            onClick={() => setModal({ kind: "create" })}
            className="text-sm text-blue-600 hover:underline"
          >
            Create your first webhook
          </button>
        </div>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium text-gray-600">
                  URL
                </th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">
                  Events
                </th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">
                  Tables
                </th>
                <th className="text-center px-4 py-2 font-medium text-gray-600">
                  Enabled
                </th>
                <th className="text-right px-4 py-2 font-medium text-gray-600">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {webhooks.map((hook) => (
                <tr
                  key={hook.id}
                  className="border-b last:border-0 hover:bg-gray-50"
                >
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-1.5 max-w-xs">
                      {hook.hasSecret && (
                        <span title="HMAC secret configured">
                          <Lock className="w-3 h-3 text-green-500 shrink-0" />
                        </span>
                      )}
                      <span
                        className="truncate font-mono text-xs"
                        title={hook.url}
                      >
                        {hook.url}
                      </span>
                      <button
                        onClick={() => copyToClipboard(hook.url, "URL")}
                        className="shrink-0 p-0.5 text-gray-300 hover:text-gray-500"
                        title="Copy URL"
                        aria-label="Copy URL"
                      >
                        <Copy className="w-3 h-3" />
                      </button>
                    </div>
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex gap-1 flex-wrap">
                      {hook.events.map((e) => (
                        <span
                          key={e}
                          className={cn(
                            "px-1.5 py-0.5 rounded text-[10px] font-medium",
                            e === "create" && "bg-green-100 text-green-700",
                            e === "update" && "bg-blue-100 text-blue-700",
                            e === "delete" && "bg-red-100 text-red-700",
                          )}
                        >
                          {e}
                        </span>
                      ))}
                    </div>
                  </td>
                  <td className="px-4 py-2.5">
                    {hook.tables.length === 0 ? (
                      <span className="text-gray-400 text-xs">all tables</span>
                    ) : (
                      <div className="flex gap-1 flex-wrap">
                        {hook.tables.map((t) => (
                          <span
                            key={t}
                            className="px-1.5 py-0.5 rounded bg-gray-100 text-gray-600 text-[10px] font-medium"
                          >
                            {t}
                          </span>
                        ))}
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-center">
                    <button
                      onClick={() => handleToggleEnabled(hook)}
                      className={cn(
                        "relative inline-flex h-5 w-9 items-center rounded-full transition-colors",
                        hook.enabled ? "bg-green-500" : "bg-gray-300",
                      )}
                      title={hook.enabled ? "Disable" : "Enable"}
                      role="switch"
                      aria-checked={hook.enabled}
                      aria-label={hook.enabled ? "Disable webhook" : "Enable webhook"}
                    >
                      <span
                        className={cn(
                          "inline-block h-3.5 w-3.5 rounded-full bg-white shadow transition-transform",
                          hook.enabled ? "translate-x-4.5" : "translate-x-1",
                        )}
                      />
                    </button>
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex gap-1 justify-end">
                      <button
                        onClick={() =>
                          setModal({ kind: "deliveries", webhook: hook })
                        }
                        className="p-1 text-gray-400 hover:text-blue-500 rounded hover:bg-gray-100"
                        title="Delivery History"
                        aria-label="Delivery History"
                      >
                        <History className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => handleTest(hook)}
                        disabled={testingId === hook.id}
                        className="p-1 text-gray-400 hover:text-amber-500 rounded hover:bg-gray-100 disabled:opacity-50"
                        title="Test"
                        aria-label="Test"
                      >
                        {testingId === hook.id ? (
                          <Loader2 className="w-3.5 h-3.5 animate-spin" />
                        ) : (
                          <Zap className="w-3.5 h-3.5" />
                        )}
                      </button>
                      <button
                        onClick={() =>
                          setModal({ kind: "edit", webhook: hook })
                        }
                        className="p-1 text-gray-400 hover:text-gray-600 rounded hover:bg-gray-100"
                        title="Edit"
                        aria-label="Edit"
                      >
                        <Pencil className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() =>
                          setModal({ kind: "delete", webhook: hook })
                        }
                        className="p-1 text-gray-400 hover:text-red-500 rounded hover:bg-gray-100"
                        title="Delete"
                        aria-label="Delete"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Create / Edit Modal */}
      {(modal.kind === "create" || modal.kind === "edit") && (
        <WebhookFormModal
          initial={modal.kind === "edit" ? modal.webhook : undefined}
          onClose={() => setModal({ kind: "none" })}
          onSaved={(hook) => {
            if (modal.kind === "create") {
              setWebhooks((prev) => [...prev, hook]);
              addToast("success", "Webhook created");
            } else {
              setWebhooks((prev) =>
                prev.map((w) => (w.id === hook.id ? hook : w)),
              );
              addToast("success", "Webhook updated");
            }
            setModal({ kind: "none" });
          }}
        />
      )}

      {/* Delete Confirmation */}
      {modal.kind === "delete" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
            <h3 className="font-semibold mb-2">Delete Webhook</h3>
            <p className="text-sm text-gray-600 mb-1">
              Are you sure? This cannot be undone.
            </p>
            <p className="text-xs font-mono text-gray-500 break-all mb-4">
              {modal.webhook.url}
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={() => handleDelete(modal.webhook)}
                className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700"
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delivery History Modal */}
      {modal.kind === "deliveries" && (
        <DeliveryHistoryModal
          webhook={modal.webhook}
          onClose={() => setModal({ kind: "none" })}
        />
      )}

      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Form Modal                                                         */
/* ------------------------------------------------------------------ */

const EVENT_OPTIONS = ["create", "update", "delete"] as const;

interface WebhookFormModalProps {
  initial?: WebhookResponse;
  onClose: () => void;
  onSaved: (hook: WebhookResponse) => void;
}

/* ------------------------------------------------------------------ */
/* Delivery History Modal                                              */
/* ------------------------------------------------------------------ */

interface DeliveryHistoryModalProps {
  webhook: WebhookResponse;
  onClose: () => void;
}

function DeliveryHistoryModal({ webhook, onClose }: DeliveryHistoryModalProps) {
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
      <div className="bg-white rounded-lg shadow-xl w-full max-w-2xl mx-4 max-h-[80vh] flex flex-col">
        <div className="flex items-center justify-between px-5 py-3 border-b shrink-0">
          <div>
            <h3 className="font-semibold">Delivery History</h3>
            <p className="text-xs text-gray-500 font-mono truncate max-w-md">
              {webhook.url}
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-gray-400 hover:text-gray-600 rounded hover:bg-gray-100"
            aria-label="Close"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="flex-1 overflow-auto p-4">
          {loading ? (
            <div className="flex items-center justify-center h-32 text-gray-400">
              <Loader2 className="w-5 h-5 animate-spin mr-2" />
              Loading deliveries...
            </div>
          ) : error ? (
            <div className="text-center py-8">
              <AlertCircle className="w-6 h-6 text-red-400 mx-auto mb-2" />
              <p className="text-red-600 text-sm">{error}</p>
            </div>
          ) : deliveries.length === 0 ? (
            <div className="text-center py-12 text-gray-400 text-sm">
              No deliveries recorded yet
            </div>
          ) : (
            <div className="space-y-1">
              {deliveries.map((del) => (
                <div key={del.id} className="border rounded">
                  <button
                    onClick={() =>
                      setExpandedId(expandedId === del.id ? null : del.id)
                    }
                    className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-gray-50 text-sm"
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
                    <span className="text-gray-400 text-xs">
                      {del.eventTable}
                    </span>
                    <span className="ml-auto text-gray-400 text-xs flex items-center gap-2">
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
                    <div className="px-3 pb-3 border-t bg-gray-50 space-y-2">
                      <div className="grid grid-cols-2 gap-2 text-xs pt-2">
                        <div>
                          <span className="text-gray-500">Attempt:</span>{" "}
                          {del.attempt}
                        </div>
                        <div>
                          <span className="text-gray-500">Duration:</span>{" "}
                          {del.durationMs}ms
                        </div>
                        <div>
                          <span className="text-gray-500">Status:</span>{" "}
                          {del.statusCode || "N/A"}
                        </div>
                        <div>
                          <span className="text-gray-500">Time:</span>{" "}
                          {formatTime(del.deliveredAt)}
                        </div>
                      </div>
                      {del.error && (
                        <div>
                          <p className="text-[10px] font-medium text-gray-500 mb-0.5">
                            Error
                          </p>
                          <pre className="text-xs bg-red-50 text-red-700 p-2 rounded border border-red-200 overflow-x-auto">
                            {del.error}
                          </pre>
                        </div>
                      )}
                      {del.requestBody && (
                        <div>
                          <p className="text-[10px] font-medium text-gray-500 mb-0.5">
                            Request Body
                          </p>
                          <pre className="text-xs bg-white p-2 rounded border overflow-x-auto max-h-32">
                            {del.requestBody}
                          </pre>
                        </div>
                      )}
                      {del.responseBody && (
                        <div>
                          <p className="text-[10px] font-medium text-gray-500 mb-0.5">
                            Response Body
                          </p>
                          <pre className="text-xs bg-white p-2 rounded border overflow-x-auto max-h-32">
                            {del.responseBody}
                          </pre>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>

        {totalPages > 1 && (
          <div className="flex items-center justify-between px-5 py-3 border-t text-sm shrink-0">
            <span className="text-gray-500 text-xs">
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
              <span className="text-xs text-gray-500 py-1">
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

/* ------------------------------------------------------------------ */
/* Form Modal                                                         */
/* ------------------------------------------------------------------ */

function WebhookFormModal({
  initial,
  onClose,
  onSaved,
}: WebhookFormModalProps) {
  const isEdit = !!initial;
  const [url, setUrl] = useState(initial?.url ?? "");
  const [secret, setSecret] = useState("");
  const [events, setEvents] = useState<string[]>(
    initial?.events ?? ["create", "update", "delete"],
  );
  const [tables, setTables] = useState(initial?.tables.join(", ") ?? "");
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const toggleEvent = (e: string) => {
    setEvents((prev) =>
      prev.includes(e) ? prev.filter((x) => x !== e) : [...prev, e],
    );
  };

  const generateSecret = () => {
    const arr = new Uint8Array(32);
    crypto.getRandomValues(arr);
    setSecret(
      Array.from(arr)
        .map((b) => b.toString(16).padStart(2, "0"))
        .join(""),
    );
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!url.trim()) return;
    setSaving(true);
    setError(null);

    const data: WebhookRequest = {
      url: url.trim(),
      events,
      tables: tables
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean),
      enabled,
    };
    if (secret) data.secret = secret;

    try {
      const result = isEdit
        ? await updateWebhook(initial!.id, data)
        : await createWebhook(data);
      onSaved(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md mx-4">
        <div className="flex items-center justify-between px-5 py-3 border-b">
          <h3 className="font-semibold">
            {isEdit ? "Edit Webhook" : "New Webhook"}
          </h3>
          <button
            onClick={onClose}
            className="p-1 text-gray-400 hover:text-gray-600 rounded hover:bg-gray-100"
            aria-label="Close"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-5 space-y-4">
          {error && (
            <div className="text-sm text-red-600 bg-red-50 px-3 py-2 rounded border border-red-200">
              {error}
            </div>
          )}

          <div>
            <label htmlFor="webhook-url" className="block text-xs font-medium text-gray-700 mb-1">
              URL <span className="text-red-500">*</span>
            </label>
            <input
              id="webhook-url"
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://example.com/webhook"
              required
              className="w-full px-3 py-1.5 border rounded text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
            />
          </div>

          <div>
            <label htmlFor="webhook-secret" className="block text-xs font-medium text-gray-700 mb-1">
              HMAC Secret
            </label>
            <div className="flex gap-2">
              <input
                id="webhook-secret"
                type="text"
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
                placeholder={isEdit && initial?.hasSecret ? "(unchanged)" : "Optional"}
                className="flex-1 px-3 py-1.5 border rounded text-sm font-mono focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
              />
              <button
                type="button"
                onClick={generateSecret}
                className="px-2 py-1.5 text-xs border rounded text-gray-600 hover:bg-gray-50 whitespace-nowrap"
              >
                Generate
              </button>
              {secret && (
                <button
                  type="button"
                  onClick={() => navigator.clipboard.writeText(secret)}
                  className="p-1.5 border rounded text-gray-400 hover:text-gray-600"
                  title="Copy secret"
                  aria-label="Copy secret"
                >
                  <Copy className="w-3.5 h-3.5" />
                </button>
              )}
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1.5">
              Events
            </label>
            <div className="flex gap-3">
              {EVENT_OPTIONS.map((evt) => (
                <label
                  key={evt}
                  className="flex items-center gap-1.5 text-sm cursor-pointer"
                >
                  <input
                    type="checkbox"
                    checked={events.includes(evt)}
                    onChange={() => toggleEvent(evt)}
                    className="rounded border-gray-300"
                  />
                  {evt}
                </label>
              ))}
            </div>
          </div>

          <div>
            <label htmlFor="webhook-tables" className="block text-xs font-medium text-gray-700 mb-1">
              Tables
            </label>
            <input
              id="webhook-tables"
              type="text"
              value={tables}
              onChange={(e) => setTables(e.target.value)}
              placeholder="All tables (or comma-separated: users, posts)"
              className="w-full px-3 py-1.5 border rounded text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500 outline-none"
            />
            <p className="text-[11px] text-gray-400 mt-0.5">
              Leave empty to receive events from all tables
            </p>
          </div>

          <div className="flex items-center gap-2">
            <label className="text-xs font-medium text-gray-700">
              Enabled
            </label>
            <button
              type="button"
              onClick={() => setEnabled(!enabled)}
              className={cn(
                "relative inline-flex h-5 w-9 items-center rounded-full transition-colors",
                enabled ? "bg-green-500" : "bg-gray-300",
              )}
              role="switch"
              aria-checked={enabled}
              aria-label="Enabled"
            >
              <span
                className={cn(
                  "inline-block h-3.5 w-3.5 rounded-full bg-white shadow transition-transform",
                  enabled ? "translate-x-4.5" : "translate-x-1",
                )}
              />
            </button>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={saving || !url.trim()}
              className="px-4 py-1.5 text-sm bg-gray-900 text-white rounded hover:bg-gray-800 disabled:opacity-50"
            >
              {saving ? "Saving..." : isEdit ? "Update" : "Create"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
