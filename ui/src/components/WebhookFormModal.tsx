import { useState } from "react";
import type { WebhookResponse, WebhookRequest } from "../types";
import { createWebhook, updateWebhook } from "../api";
import { X, Copy } from "lucide-react";
import { cn } from "../lib/utils";

const EVENT_OPTIONS = ["create", "update", "delete"] as const;

export interface WebhookFormModalProps {
  initial?: WebhookResponse;
  onClose: () => void;
  onSaved: (hook: WebhookResponse) => void;
}

export function WebhookFormModal({
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
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl w-full max-w-md mx-4">
        <div className="flex items-center justify-between px-5 py-3 border-b">
          <h3 className="font-semibold">
            {isEdit ? "Edit Webhook" : "New Webhook"}
          </h3>
          <button
            onClick={onClose}
            className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
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
            <label htmlFor="webhook-url" className="block text-xs font-medium text-gray-700 dark:text-gray-200 mb-1">
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
            <label htmlFor="webhook-secret" className="block text-xs font-medium text-gray-700 dark:text-gray-200 mb-1">
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
                className="px-2 py-1.5 text-xs border rounded text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800 whitespace-nowrap"
              >
                Generate
              </button>
              {secret && (
                <button
                  type="button"
                  onClick={() => navigator.clipboard.writeText(secret)}
                  className="p-1.5 border rounded text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300"
                  title="Copy secret"
                  aria-label="Copy secret"
                >
                  <Copy className="w-3.5 h-3.5" />
                </button>
              )}
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-gray-700 dark:text-gray-200 mb-1.5">
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
                    className="rounded border-gray-300 dark:border-gray-600"
                  />
                  {evt}
                </label>
              ))}
            </div>
          </div>

          <div>
            <label htmlFor="webhook-tables" className="block text-xs font-medium text-gray-700 dark:text-gray-200 mb-1">
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
            <p className="text-[11px] text-gray-400 dark:text-gray-500 mt-0.5">
              Leave empty to receive events from all tables
            </p>
          </div>

          <div className="flex items-center gap-2">
            <label className="text-xs font-medium text-gray-700 dark:text-gray-200">
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
                  "inline-block h-3.5 w-3.5 rounded-full bg-white dark:bg-gray-800 shadow transition-transform",
                  enabled ? "translate-x-4.5" : "translate-x-1",
                )}
              />
            </button>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700 rounded border"
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
