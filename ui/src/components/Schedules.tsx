import { useCallback, useEffect, useMemo, useState } from "react";
import type { ScheduleListResponse, ScheduleResponse } from "../types";
import {
  createSchedule,
  deleteSchedule,
  disableSchedule,
  enableSchedule,
  listSchedules,
  updateSchedule,
} from "../api";
import {
  AlertCircle,
  Loader2,
  Pencil,
  Plus,
  ToggleLeft,
  ToggleRight,
  Trash2,
} from "lucide-react";
import { cn } from "../lib/utils";
import { ToastContainer, useToast } from "./Toast";

type ModalState =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "edit"; schedule: ScheduleResponse }
  | { kind: "delete"; schedule: ScheduleResponse };

interface ScheduleFormState {
  name: string;
  jobType: string;
  cronExpr: string;
  timezone: string;
  payload: string;
  enabled: boolean;
}

const EMPTY_FORM: ScheduleFormState = {
  name: "",
  jobType: "",
  cronExpr: "0 * * * *",
  timezone: "UTC",
  payload: "{}",
  enabled: true,
};

function isCronValid(expr: string): boolean {
  return expr.trim().split(/\s+/).length === 5;
}

function formatDate(iso: string | null): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

export function Schedules() {
  const [data, setData] = useState<ScheduleListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [modal, setModal] = useState<ModalState>({ kind: "none" });
  const [form, setForm] = useState<ScheduleFormState>(EMPTY_FORM);
  const [submitting, setSubmitting] = useState(false);
  const [cronError, setCronError] = useState<string | null>(null);
  const [payloadError, setPayloadError] = useState<string | null>(null);
  const [togglingId, setTogglingId] = useState<string | null>(null);
  const { toasts, addToast, removeToast } = useToast();

  const isFormModal = modal.kind === "create" || modal.kind === "edit";
  const modalTitle = modal.kind === "edit" ? "Edit Schedule" : "Create Schedule";

  const load = useCallback(async () => {
    try {
      setError(null);
      const res = await listSchedules();
      setData(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load schedules");
      setData(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openCreate = () => {
    setCronError(null);
    setPayloadError(null);
    setForm(EMPTY_FORM);
    setModal({ kind: "create" });
  };

  const openEdit = (schedule: ScheduleResponse) => {
    setCronError(null);
    setPayloadError(null);
    setForm({
      name: schedule.name,
      jobType: schedule.jobType,
      cronExpr: schedule.cronExpr,
      timezone: schedule.timezone,
      payload: JSON.stringify(schedule.payload ?? {}, null, 2),
      enabled: schedule.enabled,
    });
    setModal({ kind: "edit", schedule });
  };

  const payloadParsed = useMemo(() => {
    try {
      return JSON.parse(form.payload || "{}");
    } catch {
      return null;
    }
  }, [form.payload]);

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setCronError(null);
    setPayloadError(null);

    if (!isCronValid(form.cronExpr)) {
      setCronError("Cron expression must have 5 fields.");
      return;
    }

    if (payloadParsed === null || typeof payloadParsed !== "object" || Array.isArray(payloadParsed)) {
      setPayloadError("Payload must be a JSON object.");
      return;
    }

    setSubmitting(true);
    try {
      if (modal.kind === "create") {
        await createSchedule({
          name: form.name.trim(),
          jobType: form.jobType.trim(),
          cronExpr: form.cronExpr.trim(),
          timezone: form.timezone.trim() || "UTC",
          payload: payloadParsed as Record<string, unknown>,
          enabled: form.enabled,
        });
        addToast("success", "Schedule created");
      } else if (modal.kind === "edit") {
        await updateSchedule(modal.schedule.id, {
          cronExpr: form.cronExpr.trim(),
          timezone: form.timezone.trim() || "UTC",
          payload: payloadParsed as Record<string, unknown>,
          enabled: form.enabled,
        });
        addToast("success", "Schedule updated");
      }
      setModal({ kind: "none" });
      await load();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Schedule save failed");
    } finally {
      setSubmitting(false);
    }
  };

  const handleToggle = async (schedule: ScheduleResponse) => {
    setTogglingId(schedule.id);
    try {
      if (schedule.enabled) {
        await disableSchedule(schedule.id);
      } else {
        await enableSchedule(schedule.id);
      }
      await load();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to update schedule");
    } finally {
      setTogglingId(null);
    }
  };

  const handleDelete = async (schedule: ScheduleResponse) => {
    setSubmitting(true);
    try {
      await deleteSchedule(schedule.id);
      addToast("success", "Schedule deleted");
      setModal({ kind: "none" });
      await load();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to delete schedule");
    } finally {
      setSubmitting(false);
    }
  };

  if (loading && !data) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading schedules...
      </div>
    );
  }

  if (error && !data) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-2" />
          <p className="text-red-600 text-sm">{error}</p>
          <button
            onClick={() => {
              setLoading(true);
              load();
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
          <h1 className="text-lg font-semibold">Job Schedules</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Configure recurring jobs using cron expressions and timezones
          </p>
        </div>
        <button
          onClick={openCreate}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
        >
          <Plus className="w-4 h-4" />
          Create Schedule
        </button>
      </div>

      {data && data.items.length === 0 ? (
        <div className="text-center py-12 border rounded-lg bg-gray-50 text-gray-500 text-sm">
          No schedules configured yet
        </div>
      ) : data ? (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 border-b">
              <tr>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Name</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Job Type</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Cron</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Last Run</th>
                <th className="text-left px-4 py-2 font-medium text-gray-600">Next Run</th>
                <th className="text-center px-4 py-2 font-medium text-gray-600">Enabled</th>
                <th className="text-right px-4 py-2 font-medium text-gray-600">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((schedule) => (
                <tr key={schedule.id} className="border-b last:border-0 hover:bg-gray-50">
                  <td className="px-4 py-2.5">
                    <span className="font-medium">{schedule.name}</span>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-600">{schedule.jobType}</td>
                  <td className="px-4 py-2.5">
                    <code className="text-xs bg-gray-100 px-1.5 py-0.5 rounded text-gray-700">
                      {schedule.cronExpr}
                    </code>
                    <div className="text-[10px] text-gray-400 mt-0.5">{schedule.timezone}</div>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-gray-500">{formatDate(schedule.lastRunAt)}</td>
                  <td className="px-4 py-2.5 text-xs text-gray-500">{formatDate(schedule.nextRunAt)}</td>
                  <td className="px-4 py-2.5 text-center">
                    <button
                      onClick={() => handleToggle(schedule)}
                      disabled={togglingId === schedule.id}
                      aria-label={`${schedule.enabled ? "Disable" : "Enable"} schedule ${schedule.id}`}
                      className={cn(
                        "inline-flex items-center gap-1 px-2 py-1 rounded text-xs",
                        schedule.enabled
                          ? "bg-green-100 text-green-700 hover:bg-green-200"
                          : "bg-gray-100 text-gray-700 hover:bg-gray-200",
                      )}
                    >
                      {schedule.enabled ? (
                        <ToggleRight className="w-4 h-4" />
                      ) : (
                        <ToggleLeft className="w-4 h-4" />
                      )}
                      {schedule.enabled ? "On" : "Off"}
                    </button>
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex justify-end gap-1">
                      <button
                        onClick={() => openEdit(schedule)}
                        aria-label={`Edit schedule ${schedule.id}`}
                        className="p-1 text-gray-400 hover:text-blue-500 rounded hover:bg-gray-100"
                      >
                        <Pencil className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => setModal({ kind: "delete", schedule })}
                        aria-label={`Delete schedule ${schedule.id}`}
                        className="p-1 text-gray-400 hover:text-red-500 rounded hover:bg-gray-100"
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
      ) : null}

      {isFormModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="w-full max-w-xl rounded-lg bg-white shadow-xl border">
            <div className="px-5 py-4 border-b">
              <h2 className="text-base font-semibold">{modalTitle}</h2>
            </div>
            <form onSubmit={handleSubmit} className="p-5 space-y-3">
              <div>
                <label htmlFor="schedule-name" className="block text-sm text-gray-700 mb-1">
                  Name
                </label>
                <input
                  id="schedule-name"
                  aria-label="Name"
                  value={form.name}
                  disabled={modal.kind === "edit"}
                  onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
                  className="w-full border rounded px-3 py-2 text-sm disabled:bg-gray-50"
                  required
                />
              </div>
              <div>
                <label htmlFor="schedule-job-type" className="block text-sm text-gray-700 mb-1">
                  Job Type
                </label>
                <input
                  id="schedule-job-type"
                  aria-label="Job Type"
                  value={form.jobType}
                  disabled={modal.kind === "edit"}
                  onChange={(e) => setForm((prev) => ({ ...prev, jobType: e.target.value }))}
                  className="w-full border rounded px-3 py-2 text-sm disabled:bg-gray-50"
                  required
                />
              </div>
              <div>
                <label htmlFor="schedule-cron" className="block text-sm text-gray-700 mb-1">
                  Cron Expression
                </label>
                <input
                  id="schedule-cron"
                  aria-label="Cron Expression"
                  value={form.cronExpr}
                  onChange={(e) => setForm((prev) => ({ ...prev, cronExpr: e.target.value }))}
                  className={cn(
                    "w-full border rounded px-3 py-2 text-sm",
                    cronError ? "border-red-300" : "border-gray-300",
                  )}
                  required
                />
                {cronError && <p className="text-xs text-red-600 mt-1">{cronError}</p>}
              </div>
              <div>
                <label htmlFor="schedule-timezone" className="block text-sm text-gray-700 mb-1">
                  Timezone
                </label>
                <input
                  id="schedule-timezone"
                  aria-label="Timezone"
                  value={form.timezone}
                  onChange={(e) => setForm((prev) => ({ ...prev, timezone: e.target.value }))}
                  className="w-full border rounded px-3 py-2 text-sm"
                  required
                />
              </div>
              <div>
                <label htmlFor="schedule-payload" className="block text-sm text-gray-700 mb-1">
                  Payload JSON
                </label>
                <textarea
                  id="schedule-payload"
                  aria-label="Payload JSON"
                  value={form.payload}
                  onChange={(e) => setForm((prev) => ({ ...prev, payload: e.target.value }))}
                  className={cn(
                    "w-full border rounded px-3 py-2 text-sm font-mono min-h-[90px]",
                    payloadError ? "border-red-300" : "border-gray-300",
                  )}
                />
                {payloadError && <p className="text-xs text-red-600 mt-1">{payloadError}</p>}
              </div>
              <label className="inline-flex items-center gap-2 text-sm text-gray-700">
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) => setForm((prev) => ({ ...prev, enabled: e.target.checked }))}
                />
                Enabled
              </label>
              <div className="pt-2 flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setModal({ kind: "none" })}
                  className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting}
                  className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-70"
                >
                  Save
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {modal.kind === "delete" && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="w-full max-w-md rounded-lg bg-white shadow-xl border">
            <div className="px-5 py-4 border-b">
              <h2 className="text-base font-semibold">Delete schedule?</h2>
            </div>
            <div className="p-5">
              <p className="text-sm text-gray-600">
                This will permanently remove <span className="font-medium">{modal.schedule.name}</span>.
              </p>
              <div className="pt-4 flex justify-end gap-2">
                <button
                  onClick={() => setModal({ kind: "none" })}
                  className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50"
                >
                  Cancel
                </button>
                <button
                  onClick={() => handleDelete(modal.schedule)}
                  disabled={submitting}
                  className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-70"
                >
                  Delete
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
