import { useState } from "react";
import type { StorageTriggerResponse } from "../types";
import {
  createStorageTrigger,
  deleteStorageTrigger,
  enableStorageTrigger,
  disableStorageTrigger,
} from "../api";
import { Plus, Trash2, Loader2, X } from "lucide-react";
import { cn } from "../lib/utils";
import { STORAGE_EVENTS } from "./EdgeFunctionTriggersShared";
import type { AddToastFn } from "./EdgeFunctionTriggersShared";

// --- Storage Trigger Panel ---

interface StorageTriggerPanelProps {
  functionId: string;
  triggers: StorageTriggerResponse[];
  loading: boolean;
  onRefresh: () => void;
  onCreateSuccess: (trigger: unknown) => void;
  addToast: AddToastFn;
}

export function StorageTriggerPanel({ functionId, triggers, loading, onRefresh, onCreateSuccess, addToast }: StorageTriggerPanelProps) {
  const [showForm, setShowForm] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

  const handleToggle = async (trigger: StorageTriggerResponse) => {
    try {
      if (trigger.enabled) {
        await disableStorageTrigger(functionId, trigger.id);
      } else {
        await enableStorageTrigger(functionId, trigger.id);
      }
      onRefresh();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to toggle trigger");
    }
  };

  const handleDelete = async (triggerId: string) => {
    try {
      await deleteStorageTrigger(functionId, triggerId);
      setConfirmDeleteId(null);
      addToast("success", "Storage trigger deleted");
      onRefresh();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to delete trigger");
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
        <span className="text-sm font-medium text-gray-700 dark:text-gray-200">Storage Triggers</span>
        <button
          data-testid="add-storage-trigger-btn"
          onClick={() => setShowForm(!showForm)}
          className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700"
        >
          <Plus className="w-3.5 h-3.5" />
          Add Trigger
        </button>
      </div>

      {showForm && (
        <StorageTriggerForm
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
          No storage triggers configured.
        </p>
      )}

      {triggers.length > 0 && (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800 border-b">
              <tr>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Bucket</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Events</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Prefix</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Suffix</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Status</th>
                <th className="text-right px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Actions</th>
              </tr>
            </thead>
            <tbody>
              {triggers.map((t) => (
                <tr key={t.id} className="border-b last:border-0">
                  <td className="px-3 py-2 font-mono text-xs">{t.bucket}</td>
                  <td className="px-3 py-2 text-xs">{t.eventTypes.join(", ")}</td>
                  <td className="px-3 py-2 font-mono text-xs text-gray-500 dark:text-gray-400">
                    {t.prefixFilter || "-"}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-gray-500 dark:text-gray-400">
                    {t.suffixFilter || "-"}
                  </td>
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

// --- Storage Trigger Form ---

interface StorageTriggerFormProps {
  functionId: string;
  onCreated: (trigger: unknown) => void;
  onCancel: () => void;
  addToast: AddToastFn;
}

function StorageTriggerForm({ functionId, onCreated, onCancel, addToast }: StorageTriggerFormProps) {
  const [bucket, setBucket] = useState("");
  const [eventTypes, setEventTypes] = useState<Set<string>>(new Set());
  const [prefixFilter, setPrefixFilter] = useState("");
  const [suffixFilter, setSuffixFilter] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const isValid = bucket.trim() !== "" && eventTypes.size > 0;

  const toggleEventType = (et: string) => {
    setEventTypes((prev) => {
      const next = new Set(prev);
      if (next.has(et)) next.delete(et);
      else next.add(et);
      return next;
    });
  };

  const handleSubmit = async () => {
    if (!isValid) return;
    setSubmitting(true);
    try {
      const created = await createStorageTrigger(functionId, {
        bucket: bucket.trim(),
        event_types: Array.from(eventTypes),
        prefix_filter: prefixFilter.trim(),
        suffix_filter: suffixFilter.trim(),
      });
      addToast("success", "Storage trigger created");
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
        <span className="text-sm font-medium">New Storage Trigger</span>
        <button onClick={onCancel} className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300">
          <X className="w-4 h-4" />
        </button>
      </div>

      <div>
        <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">Bucket</label>
        <input
          data-testid="storage-trigger-bucket"
          type="text"
          value={bucket}
          onChange={(e) => setBucket(e.target.value)}
          placeholder="uploads"
          className="w-full px-2 py-1.5 border rounded text-sm"
        />
      </div>

      <div>
        <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">Event Types</label>
        <div className="flex gap-3">
          {STORAGE_EVENTS.map((et) => (
            <label key={et} className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input
                data-testid={`storage-event-${et}`}
                type="checkbox"
                checked={eventTypes.has(et)}
                onChange={() => toggleEventType(et)}
              />
              {et}
            </label>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">
            Prefix Filter <span className="text-gray-400 dark:text-gray-500">(optional)</span>
          </label>
          <input
            data-testid="storage-trigger-prefix"
            type="text"
            value={prefixFilter}
            onChange={(e) => setPrefixFilter(e.target.value)}
            placeholder="images/"
            className="w-full px-2 py-1.5 border rounded text-sm font-mono"
          />
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">
            Suffix Filter <span className="text-gray-400 dark:text-gray-500">(optional)</span>
          </label>
          <input
            data-testid="storage-trigger-suffix"
            type="text"
            value={suffixFilter}
            onChange={(e) => setSuffixFilter(e.target.value)}
            placeholder=".jpg"
            className="w-full px-2 py-1.5 border rounded text-sm font-mono"
          />
        </div>
      </div>

      <div className="flex justify-end gap-2 pt-1">
        <button
          onClick={onCancel}
          className="px-3 py-1 text-xs text-gray-600 dark:text-gray-300 hover:text-gray-800 dark:text-gray-200"
        >
          Cancel
        </button>
        <button
          data-testid="storage-trigger-submit"
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
