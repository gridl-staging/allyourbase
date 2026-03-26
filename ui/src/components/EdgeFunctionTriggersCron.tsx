import { useState } from "react";
import type { CronTriggerResponse } from "../types";
import {
  createCronTrigger,
  deleteCronTrigger,
  enableCronTrigger,
  disableCronTrigger,
  manualRunCronTrigger,
} from "../api";
import { Plus, Trash2, Loader2, Play, X } from "lucide-react";
import { cn } from "../lib/utils";
import type { AddToastFn } from "./EdgeFunctionTriggersShared";

// --- Cron Trigger Panel ---

interface CronTriggerPanelProps {
  functionId: string;
  triggers: CronTriggerResponse[];
  loading: boolean;
  onRefresh: () => void;
  onCreateSuccess: (trigger: unknown) => void;
  addToast: AddToastFn;
}

export function CronTriggerPanel({ functionId, triggers, loading, onRefresh, onCreateSuccess, addToast }: CronTriggerPanelProps) {
  const [showForm, setShowForm] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

  const handleToggle = async (trigger: CronTriggerResponse) => {
    try {
      if (trigger.enabled) {
        await disableCronTrigger(functionId, trigger.id);
      } else {
        await enableCronTrigger(functionId, trigger.id);
      }
      onRefresh();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to toggle trigger");
    }
  };

  const handleDelete = async (triggerId: string) => {
    try {
      await deleteCronTrigger(functionId, triggerId);
      setConfirmDeleteId(null);
      addToast("success", "Cron trigger deleted");
      onRefresh();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to delete trigger");
    }
  };

  const handleManualRun = async (triggerId: string) => {
    try {
      const result = await manualRunCronTrigger(functionId, triggerId);
      addToast("success", `Manual run completed: status ${result.statusCode}`);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Manual run failed");
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8 text-gray-400 dark:text-gray-500">
        <Loader2 className="w-4 h-4 animate-spin mr-2" />
        Loading triggers...
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <span className="text-sm font-medium text-gray-700 dark:text-gray-200">Cron Triggers</span>
        <button
          data-testid="add-cron-trigger-btn"
          onClick={() => setShowForm(!showForm)}
          className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700"
        >
          <Plus className="w-3.5 h-3.5" />
          Add Trigger
        </button>
      </div>

      {showForm && (
        <CronTriggerForm
          functionId={functionId}
          onCreated={(trigger) => {
            onCreateSuccess(trigger);
            setShowForm(false);
            onRefresh();
          }}
          onCancel={() => setShowForm(false)}
          addToast={addToast}
        />
      )}

      {triggers.length === 0 && !showForm && (
        <p className="text-sm text-gray-400 dark:text-gray-500 py-4 text-center">
          No cron triggers configured.
        </p>
      )}

      {triggers.length > 0 && (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800 border-b">
              <tr>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Expression</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Timezone</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Status</th>
                <th className="text-right px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Actions</th>
              </tr>
            </thead>
            <tbody>
              {triggers.map((t) => (
                <tr key={t.id} className="border-b last:border-0">
                  <td className="px-3 py-2 font-mono text-xs">{t.cronExpr}</td>
                  <td className="px-3 py-2 text-xs">{t.timezone}</td>
                  <td className="px-3 py-2">
                    <span
                      data-testid={`trigger-enabled-${t.id}`}
                      className={cn(
                        "px-1.5 py-0.5 rounded text-xs font-medium",
                        t.enabled
                          ? "bg-green-100 text-green-700"
                          : "bg-gray-100 dark:bg-gray-700 text-gray-500 dark:text-gray-400",
                      )}
                    >
                      {t.enabled ? "Enabled" : "Disabled"}
                    </span>
                  </td>
                  <td className="px-3 py-2 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <button
                        data-testid={`trigger-run-${t.id}`}
                        onClick={() => handleManualRun(t.id)}
                        className="p-1 text-gray-500 dark:text-gray-400 hover:text-blue-600"
                        title="Run now"
                      >
                        <Play className="w-3.5 h-3.5" />
                      </button>
                      <button
                        data-testid={`trigger-toggle-${t.id}`}
                        onClick={() => handleToggle(t)}
                        className="px-2 py-0.5 text-xs rounded border hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
                      >
                        {t.enabled ? "Disable" : "Enable"}
                      </button>
                      {confirmDeleteId === t.id ? (
                        <div className="flex items-center gap-1">
                          <button
                            data-testid={`trigger-confirm-delete-${t.id}`}
                            onClick={() => handleDelete(t.id)}
                            className="px-2 py-0.5 text-xs rounded bg-red-600 text-white hover:bg-red-700"
                          >
                            Confirm
                          </button>
                          <button
                            onClick={() => setConfirmDeleteId(null)}
                            className="px-2 py-0.5 text-xs rounded text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 dark:text-gray-200"
                          >
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <button
                          data-testid={`trigger-delete-${t.id}`}
                          onClick={() => setConfirmDeleteId(t.id)}
                          className="p-1 text-gray-400 dark:text-gray-500 hover:text-red-500"
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
      )}
    </div>
  );
}

// --- Cron Trigger Form ---

interface CronTriggerFormProps {
  functionId: string;
  onCreated: (trigger: unknown) => void;
  onCancel: () => void;
  addToast: AddToastFn;
}

function CronTriggerForm({ functionId, onCreated, onCancel, addToast }: CronTriggerFormProps) {
  const [cronExpr, setCronExpr] = useState("");
  const [timezone, setTimezone] = useState("UTC");
  const [payload, setPayload] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const isValid = cronExpr.trim() !== "";

  const handleSubmit = async () => {
    if (!isValid) return;
    setSubmitting(true);
    try {
      let parsedPayload: unknown;
      if (payload.trim()) {
        try {
          parsedPayload = JSON.parse(payload);
        } catch {
          addToast("error", "Invalid JSON payload");
          setSubmitting(false);
          return;
        }
      }
      const created = await createCronTrigger(functionId, {
        cron_expr: cronExpr.trim(),
        timezone: timezone.trim() || "UTC",
        payload: parsedPayload,
      });
      addToast("success", "Cron trigger created");
      onCreated(created);
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to create trigger");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="border rounded-lg p-4 mb-4 bg-gray-50 dark:bg-gray-800 space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">New Cron Trigger</span>
        <button onClick={onCancel} className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300">
          <X className="w-4 h-4" />
        </button>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">Cron Expression</label>
          <input
            data-testid="cron-trigger-expr"
            type="text"
            value={cronExpr}
            onChange={(e) => setCronExpr(e.target.value)}
            placeholder="*/5 * * * *"
            className="w-full px-2 py-1.5 border rounded text-sm font-mono"
          />
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">Timezone</label>
          <input
            data-testid="cron-trigger-timezone"
            type="text"
            value={timezone}
            onChange={(e) => setTimezone(e.target.value)}
            placeholder="UTC"
            className="w-full px-2 py-1.5 border rounded text-sm"
          />
        </div>
      </div>

      <div>
        <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">
          Payload <span className="text-gray-400 dark:text-gray-500">(optional, JSON)</span>
        </label>
        <textarea
          data-testid="cron-trigger-payload"
          value={payload}
          onChange={(e) => setPayload(e.target.value)}
          placeholder='{"key": "value"}'
          className="w-full px-2 py-1.5 border rounded text-sm font-mono h-16"
        />
      </div>

      <div className="flex justify-end gap-2 pt-1">
        <button
          onClick={onCancel}
          className="px-3 py-1 text-xs text-gray-600 dark:text-gray-300 hover:text-gray-800 dark:text-gray-200"
        >
          Cancel
        </button>
        <button
          data-testid="cron-trigger-submit"
          onClick={handleSubmit}
          disabled={!isValid || submitting}
          className="flex items-center gap-1 px-3 py-1 bg-gray-900 text-white rounded text-xs hover:bg-gray-800 disabled:opacity-50"
        >
          {submitting && <Loader2 className="w-3 h-3 animate-spin" />}
          Create Trigger
        </button>
      </div>
    </div>
  );
}
