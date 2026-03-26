import { useState, useEffect, useCallback } from "react";
import type { WebhookResponse } from "../types";
import {
  listWebhooks,
  updateWebhook,
  deleteWebhook,
  testWebhook,
} from "../api";
import {
  Plus,
  Trash2,
  Pencil,
  Lock,
  Copy,
  Loader2,
  AlertCircle,
  Webhook,
  Zap,
  History,
} from "lucide-react";
import { cn } from "../lib/utils";
import { useAppToast } from "./ToastProvider";
import { DeliveryHistoryModal } from "./WebhookDeliveryModal";
import { WebhookFormModal } from "./WebhookFormModal";

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
  const { addToast } = useAppToast();

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
      <div className="flex items-center justify-center h-64 text-gray-400 dark:text-gray-500">
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
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
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
        <div className="text-center py-16 border rounded-lg bg-gray-50 dark:bg-gray-800">
          <Webhook className="w-10 h-10 text-gray-300 dark:text-gray-500 mx-auto mb-3" />
          <p className="text-gray-500 dark:text-gray-400 text-sm mb-3">
            No webhooks configured yet
          </p>
          <p className="text-gray-500 dark:text-gray-400 text-sm mb-3">
            Deliver create, update, and delete events to external URLs in real time.
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
            <thead className="bg-gray-50 dark:bg-gray-800 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">
                  URL
                </th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">
                  Events
                </th>
                <th className="text-left px-4 py-2 font-medium text-gray-600 dark:text-gray-300">
                  Tables
                </th>
                <th className="text-center px-4 py-2 font-medium text-gray-600 dark:text-gray-300">
                  Enabled
                </th>
                <th className="text-right px-4 py-2 font-medium text-gray-600 dark:text-gray-300">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {webhooks.map((hook) => (
                <tr
                  key={hook.id}
                  className="border-b last:border-0 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
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
                        className="shrink-0 p-0.5 text-gray-300 dark:text-gray-500 hover:text-gray-500 dark:hover:text-gray-300 dark:text-gray-400"
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
                      <span className="text-gray-400 dark:text-gray-500 text-xs">all tables</span>
                    ) : (
                      <div className="flex gap-1 flex-wrap">
                        {hook.tables.map((t) => (
                          <span
                            key={t}
                            className="px-1.5 py-0.5 rounded bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 text-[10px] font-medium"
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
                          "inline-block h-3.5 w-3.5 rounded-full bg-white dark:bg-gray-800 shadow transition-transform",
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
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-blue-500 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
                        title="Delivery History"
                        aria-label="Delivery History"
                      >
                        <History className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => handleTest(hook)}
                        disabled={testingId === hook.id}
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-amber-500 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700 disabled:opacity-50"
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
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
                        title="Edit"
                        aria-label="Edit"
                      >
                        <Pencil className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() =>
                          setModal({ kind: "delete", webhook: hook })
                        }
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-red-500 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
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
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
            <h3 className="font-semibold mb-2">Delete Webhook</h3>
            <p className="text-sm text-gray-600 dark:text-gray-300 mb-1">
              Are you sure? This cannot be undone.
            </p>
            <p className="text-xs font-mono text-gray-500 dark:text-gray-400 break-all mb-4">
              {modal.webhook.url}
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700 rounded border"
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

    </div>
  );
}
