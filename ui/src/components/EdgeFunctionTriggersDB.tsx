import { useState } from "react";
import type { DBTriggerResponse, DBTriggerEvent } from "../types";
import {
  createDBTrigger,
  deleteDBTrigger,
  enableDBTrigger,
  disableDBTrigger,
} from "../api";
import { Plus, Trash2, Loader2, X } from "lucide-react";
import { cn } from "../lib/utils";
import { DB_EVENTS } from "./EdgeFunctionTriggersShared";
import type { AddToastFn } from "./EdgeFunctionTriggersShared";

// --- DB Trigger Panel ---

interface DBTriggerPanelProps {
  functionId: string;
  triggers: DBTriggerResponse[];
  loading: boolean;
  onRefresh: () => void;
  onCreateSuccess: (trigger: unknown) => void;
  addToast: AddToastFn;
}

export function DBTriggerPanel({ functionId, triggers, loading, onRefresh, onCreateSuccess, addToast }: DBTriggerPanelProps) {
  const [showForm, setShowForm] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

  const handleToggle = async (trigger: DBTriggerResponse) => {
    try {
      if (trigger.enabled) {
        await disableDBTrigger(functionId, trigger.id);
      } else {
        await enableDBTrigger(functionId, trigger.id);
      }
      onRefresh();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to toggle trigger");
    }
  };

  const handleDelete = async (triggerId: string) => {
    try {
      await deleteDBTrigger(functionId, triggerId);
      setConfirmDeleteId(null);
      addToast("success", "DB trigger deleted");
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
        <span className="text-sm font-medium text-gray-700 dark:text-gray-200">Database Triggers</span>
        <button
          data-testid="add-db-trigger-btn"
          onClick={() => setShowForm(!showForm)}
          className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700"
        >
          <Plus className="w-3.5 h-3.5" />
          Add Trigger
        </button>
      </div>

      {showForm && (
        <DBTriggerForm
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
          No database triggers configured.
        </p>
      )}

      {triggers.length > 0 && (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800 border-b">
              <tr>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Table</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Schema</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Events</th>
                <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Status</th>
                <th className="text-right px-3 py-2 font-medium text-gray-600 dark:text-gray-300">Actions</th>
              </tr>
            </thead>
            <tbody>
              {triggers.map((t) => (
                <tr key={t.id} className="border-b last:border-0">
                  <td className="px-3 py-2 font-mono text-xs">{t.tableName}</td>
                  <td className="px-3 py-2 text-gray-500 dark:text-gray-400 text-xs">{t.schema}</td>
                  <td className="px-3 py-2 text-xs">{t.events.join(", ")}</td>
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

// --- DB Trigger Form ---

interface DBTriggerFormProps {
  functionId: string;
  onCreated: (trigger: unknown) => void;
  onCancel: () => void;
  addToast: AddToastFn;
}

function DBTriggerForm({ functionId, onCreated, onCancel, addToast }: DBTriggerFormProps) {
  const [tableName, setTableName] = useState("");
  const [schema, setSchema] = useState("public");
  const [events, setEvents] = useState<Set<DBTriggerEvent>>(new Set());
  const [filterColumns, setFilterColumns] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const isValid = tableName.trim() !== "" && events.size > 0;

  const toggleEvent = (ev: DBTriggerEvent) => {
    setEvents((prev) => {
      const next = new Set(prev);
      if (next.has(ev)) next.delete(ev);
      else next.add(ev);
      return next;
    });
  };

  const handleSubmit = async () => {
    if (!isValid) return;
    setSubmitting(true);
    try {
      const created = await createDBTrigger(functionId, {
        table_name: tableName.trim(),
        schema: schema.trim() || "public",
        events: Array.from(events),
        filter_columns: filterColumns.trim()
          ? filterColumns.split(",").map((c) => c.trim()).filter(Boolean)
          : [],
      });
      addToast("success", "DB trigger created");
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
        <span className="text-sm font-medium">New Database Trigger</span>
        <button onClick={onCancel} className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300">
          <X className="w-4 h-4" />
        </button>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">Table Name</label>
          <input
            data-testid="db-trigger-table"
            type="text"
            value={tableName}
            onChange={(e) => setTableName(e.target.value)}
            placeholder="users"
            className="w-full px-2 py-1.5 border rounded text-sm"
          />
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">Schema</label>
          <input
            data-testid="db-trigger-schema"
            type="text"
            value={schema}
            onChange={(e) => setSchema(e.target.value)}
            placeholder="public"
            className="w-full px-2 py-1.5 border rounded text-sm"
          />
        </div>
      </div>

      <div>
        <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">Events</label>
        <div className="flex gap-3">
          {DB_EVENTS.map((ev) => (
            <label key={ev} className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input
                data-testid={`db-event-${ev}`}
                type="checkbox"
                checked={events.has(ev)}
                onChange={() => toggleEvent(ev)}
              />
              {ev}
            </label>
          ))}
        </div>
      </div>

      <div>
        <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">
          Filter Columns <span className="text-gray-400 dark:text-gray-500">(optional, comma-separated)</span>
        </label>
        <input
          data-testid="db-trigger-filter-columns"
          type="text"
          value={filterColumns}
          onChange={(e) => setFilterColumns(e.target.value)}
          placeholder="name, email"
          className="w-full px-2 py-1.5 border rounded text-sm"
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
          data-testid="db-trigger-submit"
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
