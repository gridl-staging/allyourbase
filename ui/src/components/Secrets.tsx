import { useState } from "react";
import {
  listSecrets,
  getSecret,
  createSecret,
  updateSecret,
  deleteSecret,
  rotateSecrets,
} from "../api_secrets";
import type { SecretMetadata } from "../types/secrets";
import { useAdminResource } from "../hooks/useAdminResource";
import { AdminTable, type Column } from "./shared/AdminTable";
import { ConfirmDialog } from "./shared/ConfirmDialog";
import { KeyRound } from "lucide-react";

export function Secrets() {
  const { data, loading, error, actionLoading, runAction } =
    useAdminResource(listSecrets);
  const [showCreate, setShowCreate] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createValue, setCreateValue] = useState("");
  const [editingName, setEditingName] = useState<string | null>(null);
  const [revealedValues, setRevealedValues] = useState<Record<string, string>>(
    {},
  );
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [showRotate, setShowRotate] = useState(false);

  const closeForm = () => {
    setShowCreate(false);
    setEditingName(null);
    setCreateName("");
    setCreateValue("");
  };

  const openCreateForm = () => {
    setEditingName(null);
    setCreateName("");
    setCreateValue("");
    setShowCreate(true);
  };

  const handleReveal = (name: string) =>
    runAction(async () => {
      const secret = await getSecret(name);
      setRevealedValues((prev) => ({ ...prev, [name]: secret.value }));
    });

  const handleSubmit = () =>
    runAction(async () => {
      if (editingName) {
        await updateSecret(editingName, { value: createValue });
      } else {
        await createSecret({ name: createName, value: createValue });
      }
      closeForm();
    });

  const handleDelete = () => {
    if (!deleteTarget) return;
    runAction(async () => {
      await deleteSecret(deleteTarget);
      setDeleteTarget(null);
    });
  };

  const handleRotate = () =>
    runAction(async () => {
      await rotateSecrets();
      setShowRotate(false);
    });

  const columns: Column<SecretMetadata>[] = [
    { key: "name", header: "Name" },
    {
      key: "created_at",
      header: "Created",
      render: (row) => new Date(row.created_at).toLocaleDateString(),
    },
    {
      key: "updated_at",
      header: "Updated",
      render: (row) => new Date(row.updated_at).toLocaleDateString(),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <div className="flex gap-2">
          {revealedValues[row.name] ? (
            <span className="text-xs font-mono bg-gray-100 dark:bg-gray-800 px-2 py-0.5 rounded">
              {revealedValues[row.name]}
            </span>
          ) : (
            <button
              aria-label={`Reveal ${row.name}`}
              onClick={() => handleReveal(row.name)}
              disabled={actionLoading}
              className="text-xs text-blue-500 hover:text-blue-600 disabled:opacity-50"
            >
              Reveal
            </button>
          )}
          <button
            aria-label={`Update ${row.name}`}
            onClick={() => {
              setEditingName(row.name);
              setCreateName(row.name);
              setCreateValue("");
              setShowCreate(true);
            }}
            disabled={actionLoading}
            className="text-xs text-amber-600 hover:text-amber-700 disabled:opacity-50"
          >
            Update
          </button>
          <button
            aria-label={`Delete ${row.name}`}
            onClick={() => setDeleteTarget(row.name)}
            className="text-xs text-red-500 hover:text-red-600"
          >
            Delete
          </button>
        </div>
      ),
    },
  ];

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Secrets
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          Secrets
        </h2>
        <div className="flex gap-2">
          <button
            onClick={openCreateForm}
            className="px-3 py-1.5 text-xs bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 rounded hover:bg-gray-800 dark:hover:bg-gray-200 font-medium"
          >
            Create Secret
          </button>
          <button
            onClick={() => setShowRotate(true)}
            className="px-3 py-1.5 text-xs bg-red-600 text-white rounded hover:bg-red-700 font-medium"
          >
            Rotate JWT Secret
          </button>
        </div>
      </div>

      {showCreate && (
        <div className="mb-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900">
          <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-3">
            {editingName ? `Update ${editingName}` : "New Secret"}
          </h3>
          <div className="flex flex-col gap-2 max-w-md">
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Name
              <input
                type="text"
                value={createName}
                onChange={(e) => setCreateName(e.target.value)}
                disabled={editingName !== null}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Value
              <input
                type="text"
                value={createValue}
                onChange={(e) => setCreateValue(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <div className="flex gap-2 mt-2">
              <button
                onClick={handleSubmit}
                disabled={!createName || !createValue || actionLoading}
                className="px-3 py-1.5 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 font-medium disabled:opacity-50"
              >
                {editingName ? "Update" : "Create"}
              </button>
              <button
                onClick={closeForm}
                className="px-3 py-1.5 text-xs text-gray-600 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-200"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {loading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading...</p>
      ) : (data ?? []).length === 0 ? (
        <div className="text-center py-12 border rounded-lg bg-gray-50 dark:bg-gray-800 px-6">
          <KeyRound className="w-9 h-9 text-gray-300 dark:text-gray-500 mx-auto mb-3" />
          <h3 className="text-sm font-medium text-gray-800 dark:text-gray-200">No secrets configured yet</h3>
          <p className="text-gray-500 dark:text-gray-400 text-sm mt-1">
            Store API tokens, signing keys, and other sensitive values for your backend.
          </p>
          <button
            onClick={openCreateForm}
            className="mt-3 text-sm text-blue-600 hover:underline"
          >
            Create your first secret
          </button>
        </div>
      ) : (
        <AdminTable
          columns={columns}
          rows={data ?? []}
          rowKey="name"
          page={1}
          totalPages={1}
          onPageChange={() => {}}
          emptyMessage=""
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete Secret"
        message={`Delete secret ${deleteTarget}? This cannot be undone.`}
        confirmLabel="Delete"
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
        destructive
        loading={actionLoading}
      />

      <ConfirmDialog
        open={showRotate}
        title="Rotate JWT Secret"
        message="This will invalidate all existing JWT tokens. All users will need to re-authenticate."
        confirmLabel="Rotate"
        onConfirm={handleRotate}
        onCancel={() => setShowRotate(false)}
        destructive
        loading={actionLoading}
      />
    </div>
  );
}
