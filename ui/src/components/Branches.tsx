import { useState, useEffect, useCallback } from "react";
import type { BranchRecord } from "../types";
import { listBranches, createBranch, deleteBranch, ApiError } from "../api";
import {
  Plus,
  Trash2,
  X,
  Loader2,
  AlertCircle,
  GitBranch,
} from "lucide-react";
import { useAppToast } from "./ToastProvider";

type Modal =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "delete"; branch: BranchRecord };

const STATUS_BADGE: Record<BranchRecord["status"], { label: string; cls: string }> = {
  ready: {
    label: "Ready",
    cls: "bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300",
  },
  creating: {
    label: "Creating",
    cls: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/40 dark:text-yellow-300",
  },
  failed: {
    label: "Failed",
    cls: "bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300",
  },
  deleting: {
    label: "Deleting",
    cls: "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400",
  },
};

export function Branches() {
  const [branches, setBranches] = useState<BranchRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [modal, setModal] = useState<Modal>({ kind: "none" });
  const { addToast } = useAppToast();

  const fetchBranches = useCallback(async () => {
    try {
      setError(null);
      const data = await listBranches();
      setBranches(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load branches");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchBranches();
  }, [fetchBranches]);

  const handleCreate = async (name: string, from: string) => {
    try {
      await createBranch(name, from || undefined);
      setModal({ kind: "none" });
      addToast("success", `Branch "${name}" created`);
      fetchBranches();
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        addToast("error", e.message);
      } else {
        addToast("error", e instanceof Error ? e.message : "Create failed");
      }
    }
  };

  const handleDelete = async (branch: BranchRecord) => {
    try {
      await deleteBranch(branch.name);
      setModal({ kind: "none" });
      addToast("success", `Branch "${branch.name}" deleted`);
      fetchBranches();
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        addToast("error", e.message);
      } else {
        addToast("error", e instanceof Error ? e.message : "Delete failed");
      }
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="w-6 h-6 animate-spin text-gray-400" />
      </div>
    );
  }

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Branches</h1>
        <button
          onClick={() => setModal({ kind: "create" })}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="w-4 h-4" />
          Add Branch
        </button>
      </div>

      {error && (
        <div className="flex items-center gap-2 p-4 mb-4 rounded bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300">
          <AlertCircle className="w-4 h-4 shrink-0" />
          <span className="text-sm">{error}</span>
          <button
            onClick={fetchBranches}
            className="ml-auto text-sm font-medium underline hover:no-underline"
          >
            Retry
          </button>
        </div>
      )}

      {branches.length === 0 && !error ? (
        <div className="text-center py-12 text-gray-500 dark:text-gray-400">
          <GitBranch className="w-10 h-10 mx-auto mb-3 text-gray-300 dark:text-gray-600" />
          <p>No branches yet</p>
          <p className="text-sm mt-1">Create a branch to get started.</p>
        </div>
      ) : (
        <div className="border border-gray-200 dark:border-gray-700 rounded overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800/50 text-left text-gray-500 dark:text-gray-400">
              <tr>
                <th className="px-4 py-2 font-medium">Name</th>
                <th className="px-4 py-2 font-medium">Status</th>
                <th className="px-4 py-2 font-medium">Source</th>
                <th className="px-4 py-2 font-medium">Created</th>
                <th className="px-4 py-2 font-medium w-16" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {branches.map((b) => {
                const badge = STATUS_BADGE[b.status];
                return (
                  <tr key={b.id} className="hover:bg-gray-50 dark:hover:bg-gray-800/30">
                    <td className="px-4 py-2 font-medium text-gray-900 dark:text-gray-100">
                      {b.name}
                    </td>
                    <td className="px-4 py-2">
                      <span className={`inline-block px-2 py-0.5 text-xs font-medium rounded-full ${badge.cls}`}>
                        {badge.label}
                      </span>
                    </td>
                    <td className="px-4 py-2 text-gray-600 dark:text-gray-400 truncate max-w-xs">
                      {b.source_database}
                    </td>
                    <td className="px-4 py-2 text-gray-600 dark:text-gray-400">
                      {new Date(b.created_at).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-2 text-right">
                      <button
                        onClick={() => setModal({ kind: "delete", branch: b })}
                        className="p-1 text-gray-400 hover:text-red-600 dark:hover:text-red-400 rounded hover:bg-gray-100 dark:hover:bg-gray-800"
                        aria-label="Delete"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {modal.kind === "create" && <CreateModal onClose={() => setModal({ kind: "none" })} onCreate={handleCreate} />}
      {modal.kind === "delete" && (
        <DeleteModal branch={modal.branch} onClose={() => setModal({ kind: "none" })} onConfirm={() => handleDelete(modal.branch)} />
      )}
    </div>
  );
}

function CreateModal({ onClose, onCreate }: { onClose: () => void; onCreate: (name: string, from: string) => void }) {
  const [name, setName] = useState("");
  const [from, setFrom] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setSubmitting(true);
    await onCreate(name.trim(), from.trim());
    setSubmitting(false);
  };

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
      <div className="bg-white dark:bg-gray-900 rounded-lg shadow-xl w-full max-w-md p-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Create Branch</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300">
            <X className="w-5 h-5" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Branch Name
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Branch name"
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 text-sm focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              autoFocus
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
              Source Database URL <span className="text-gray-400 font-normal">(optional)</span>
            </label>
            <input
              type="text"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
              placeholder="postgres://... (defaults to main database)"
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 text-sm focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!name.trim() || submitting}
              className="px-3 py-1.5 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600 disabled:opacity-50"
            >
              {submitting ? "Creating…" : "Create"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

function DeleteModal({ branch, onClose, onConfirm }: { branch: BranchRecord; onClose: () => void; onConfirm: () => void }) {
  const [submitting, setSubmitting] = useState(false);

  const handleConfirm = async () => {
    setSubmitting(true);
    await onConfirm();
    setSubmitting(false);
  };

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
      <div className="bg-white dark:bg-gray-900 rounded-lg shadow-xl w-full max-w-sm p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2">Delete Branch</h2>
        <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">
          Are you sure you want to delete <strong>{branch.name}</strong>? This action cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-sm rounded border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            disabled={submitting}
            className="px-3 py-1.5 text-sm font-medium rounded bg-red-600 text-white hover:bg-red-700 disabled:opacity-50"
          >
            {submitting ? "Deleting…" : "Confirm"}
          </button>
        </div>
      </div>
    </div>
  );
}
