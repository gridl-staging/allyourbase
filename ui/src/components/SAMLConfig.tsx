import { useState } from "react";
import {
  listSAMLProviders,
  createSAMLProvider,
  updateSAMLProvider,
  deleteSAMLProvider,
} from "../api_saml";
import type { SAMLProvider, SAMLUpsertRequest } from "../types/saml";
import { useAdminResource } from "../hooks/useAdminResource";
import { AdminTable, type Column } from "./shared/AdminTable";
import { ConfirmDialog } from "./shared/ConfirmDialog";

interface FormState {
  name: string;
  entity_id: string;
  idp_metadata_url: string;
  idp_metadata_xml: string;
  attribute_mapping: string;
}

const emptyForm: FormState = {
  name: "",
  entity_id: "",
  idp_metadata_url: "",
  idp_metadata_xml: "",
  attribute_mapping: "",
};

export function SAMLConfig() {
  const { data, loading, error, setError, actionLoading, runAction } =
    useAdminResource(listSAMLProviders);
  const [showForm, setShowForm] = useState(false);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);

  const handleSubmit = () => {
    let parsedMapping: Record<string, string> | undefined;
    if (form.attribute_mapping) {
      try {
        parsedMapping = JSON.parse(form.attribute_mapping);
      } catch {
        setError("Attribute Mapping is not valid JSON");
        return;
      }
    }
    const req: SAMLUpsertRequest = {
      name: form.name,
      entity_id: form.entity_id,
      idp_metadata_url: form.idp_metadata_url || undefined,
      idp_metadata_xml: form.idp_metadata_xml || undefined,
      attribute_mapping: parsedMapping,
    };
    runAction(async () => {
      if (editingName) {
        await updateSAMLProvider(editingName, req);
      } else {
        await createSAMLProvider(req);
      }
      closeForm();
    });
  };

  const handleEdit = (provider: SAMLProvider) => {
    setForm({
      name: provider.name,
      entity_id: provider.entity_id,
      idp_metadata_url: "",
      idp_metadata_xml: provider.idp_metadata_xml ?? "",
      attribute_mapping: provider.attribute_mapping
        ? JSON.stringify(provider.attribute_mapping, null, 2)
        : "",
    });
    setEditingName(provider.name);
    setShowForm(true);
  };

  const handleDelete = () => {
    if (!deleteTarget) return;
    runAction(async () => {
      await deleteSAMLProvider(deleteTarget);
      setDeleteTarget(null);
    });
  };

  const closeForm = () => {
    setShowForm(false);
    setEditingName(null);
    setForm(emptyForm);
  };

  const formValid = form.name.length > 0 && form.entity_id.length > 0;

  const columns: Column<SAMLProvider>[] = [
    { key: "name", header: "Name" },
    { key: "entity_id", header: "Entity ID" },
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
          <button
            aria-label={`Edit ${row.name}`}
            onClick={() => handleEdit(row)}
            className="text-xs text-blue-500 hover:text-blue-600"
          >
            Edit
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
          SAML Configuration
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          SAML Configuration
        </h2>
        <button
          onClick={() => {
            setForm(emptyForm);
            setEditingName(null);
            setShowForm(true);
          }}
          className="px-3 py-1.5 text-xs bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 rounded hover:bg-gray-800 dark:hover:bg-gray-200 font-medium"
        >
          Add Provider
        </button>
      </div>

      {showForm && (
        <div className="mb-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900">
          <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-3">
            {editingName ? `Edit ${editingName}` : "New Provider"}
          </h3>
          <div className="flex flex-col gap-2 max-w-lg">
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Name
              <input
                type="text"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Entity ID
              <input
                type="text"
                value={form.entity_id}
                onChange={(e) =>
                  setForm({ ...form, entity_id: e.target.value })
                }
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Metadata URL
              <input
                type="text"
                value={form.idp_metadata_url}
                onChange={(e) =>
                  setForm({ ...form, idp_metadata_url: e.target.value })
                }
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Metadata XML
              <textarea
                value={form.idp_metadata_xml}
                onChange={(e) =>
                  setForm({ ...form, idp_metadata_xml: e.target.value })
                }
                rows={4}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 font-mono"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Attribute Mapping (JSON)
              <textarea
                value={form.attribute_mapping}
                onChange={(e) =>
                  setForm({ ...form, attribute_mapping: e.target.value })
                }
                rows={3}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100 font-mono"
              />
            </label>
            <div className="flex gap-2 mt-2">
              <button
                onClick={handleSubmit}
                disabled={!formValid || actionLoading}
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
      ) : (
        <AdminTable
          columns={columns}
          rows={data ?? []}
          rowKey="name"
          page={1}
          totalPages={1}
          onPageChange={() => {}}
          emptyMessage="No SAML providers configured"
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete Provider"
        message={`Delete SAML provider ${deleteTarget}? This cannot be undone.`}
        confirmLabel="Delete"
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
        destructive
        loading={actionLoading}
      />
    </div>
  );
}
