import { useState } from "react";
import { listVectorIndexes, createVectorIndex } from "../api_vector";
import type { VectorIndexInfo } from "../types/vector";
import { useAdminResource } from "../hooks/useAdminResource";
import { AdminTable, type Column } from "./shared/AdminTable";

export function VectorIndexes() {
  const { data, loading, error, actionLoading, runAction } =
    useAdminResource(listVectorIndexes);
  const [showCreate, setShowCreate] = useState(false);
  const [schema, setSchema] = useState("public");
  const [table, setTable] = useState("");
  const [column, setColumn] = useState("");
  const [method, setMethod] = useState("");
  const [metric, setMetric] = useState("");
  const [indexName, setIndexName] = useState("");

  const resetForm = () => {
    setSchema("public");
    setTable("");
    setColumn("");
    setMethod("");
    setMetric("");
    setIndexName("");
  };

  const handleCreate = () =>
    runAction(async () => {
      await createVectorIndex({
        schema,
        table,
        column,
        method,
        metric,
        index_name: indexName || undefined,
      });
      setShowCreate(false);
      resetForm();
    });

  const formValid =
    table.length > 0 && column.length > 0 && method.length > 0 && metric.length > 0;

  const columns: Column<VectorIndexInfo>[] = [
    { key: "name", header: "Name" },
    { key: "schema", header: "Schema" },
    { key: "table", header: "Table" },
    { key: "method", header: "Method" },
  ];

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Vector Indexes
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          Vector Indexes
        </h2>
        <button
          onClick={() => setShowCreate(true)}
          className="px-3 py-1.5 text-xs bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 rounded hover:bg-gray-800 dark:hover:bg-gray-200 font-medium"
        >
          Create Index
        </button>
      </div>

      {showCreate && (
        <div
          role="region"
          aria-labelledby="vector-index-create-heading"
          data-testid="vector-index-create-panel"
          className="mb-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900"
        >
          <h3 id="vector-index-create-heading" className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-3">
            New Vector Index
          </h3>
          <div className="flex flex-col gap-2 max-w-md">
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Schema
              <input
                type="text"
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Table
              <input
                type="text"
                value={table}
                onChange={(e) => setTable(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Column
              <input
                type="text"
                value={column}
                onChange={(e) => setColumn(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Method
              <select
                value={method}
                onChange={(e) => setMethod(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800"
              >
                <option value="">Select method</option>
                <option value="hnsw">hnsw</option>
                <option value="ivfflat">ivfflat</option>
              </select>
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Metric
              <input
                type="text"
                value={metric}
                onChange={(e) => setMetric(e.target.value)}
                placeholder="cosine, l2, inner_product"
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Index Name (optional)
              <input
                type="text"
                value={indexName}
                onChange={(e) => setIndexName(e.target.value)}
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <div className="flex gap-2 mt-2">
              <button
                onClick={handleCreate}
                disabled={!formValid || actionLoading}
                className="px-3 py-1.5 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 font-medium disabled:opacity-50"
              >
                Create
              </button>
              <button
                onClick={() => {
                  setShowCreate(false);
                  resetForm();
                }}
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
          emptyMessage="No vector indexes found"
        />
      )}
    </div>
  );
}
