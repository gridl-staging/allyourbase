import { useState } from "react";
import {
  listServers,
  createServer,
  dropServer,
  listTables,
  importTables,
  dropTable,
} from "../api_fdw";
import type { ForeignServer, ForeignTable } from "../types/fdw";
import { useAdminResource } from "../hooks/useAdminResource";
import { AdminTable, type Column } from "./shared/AdminTable";
import { ConfirmDialog } from "./shared/ConfirmDialog";

export function FDWManagement() {
  const servers = useAdminResource(listServers);
  const tables = useAdminResource(listTables);

  const [showCreateServer, setShowCreateServer] = useState(false);
  const [showImport, setShowImport] = useState(false);
  const [dropServerTarget, setDropServerTarget] = useState<string | null>(null);
  const [dropServerCascade, setDropServerCascade] = useState(false);
  const [dropTableTarget, setDropTableTarget] = useState<{
    schema: string;
    name: string;
  } | null>(null);

  // Create server form state
  const [serverName, setServerName] = useState("");
  const [fdwType, setFdwType] = useState("postgres_fdw");
  const [serverHost, setServerHost] = useState("");
  const [serverPort, setServerPort] = useState("5432");
  const [serverDbname, setServerDbname] = useState("");
  const [serverFilename, setServerFilename] = useState("");
  const [mappingUser, setMappingUser] = useState("");
  const [mappingPassword, setMappingPassword] = useState("");

  // Import tables form state
  const [importServer, setImportServer] = useState("");
  const [remoteSchema, setRemoteSchema] = useState("");
  const [localSchema, setLocalSchema] = useState("public");

  const resetCreateForm = () => {
    setServerName("");
    setFdwType("postgres_fdw");
    setServerHost("");
    setServerPort("5432");
    setServerDbname("");
    setServerFilename("");
    setMappingUser("");
    setMappingPassword("");
  };

  const resetImportForm = () => {
    setImportServer("");
    setRemoteSchema("");
    setLocalSchema("public");
  };

  const isPostgresFDW = fdwType === "postgres_fdw";
  const createValid = isPostgresFDW
    ? serverName.trim() !== "" && serverHost.trim() !== ""
    : serverName.trim() !== "" && serverFilename.trim() !== "";
  const importValid =
    importServer.trim() !== "" && remoteSchema.trim() !== "";

  const handleCreateServer = () =>
    servers.runAction(async () => {
      await createServer({
        name: serverName.trim(),
        fdw_type: fdwType,
        options: isPostgresFDW
          ? { host: serverHost, port: serverPort, dbname: serverDbname }
          : { filename: serverFilename },
        ...(isPostgresFDW
          ? {
              user_mapping: {
                user: mappingUser,
                password: mappingPassword,
              },
            }
          : {}),
      });
      setShowCreateServer(false);
      resetCreateForm();
      await tables.refresh();
    });

  const handleDropServer = () => {
    if (!dropServerTarget) return;
    servers.runAction(async () => {
      await dropServer(dropServerTarget, dropServerCascade);
      setDropServerTarget(null);
      setDropServerCascade(false);
      await tables.refresh();
    });
  };

  const handleImportTables = () =>
    tables.runAction(async () => {
      await importTables(importServer, {
        remote_schema: remoteSchema,
        local_schema: localSchema,
      });
      setShowImport(false);
      resetImportForm();
    });

  const handleDropTable = () => {
    if (!dropTableTarget) return;
    tables.runAction(async () => {
      await dropTable(dropTableTarget.schema, dropTableTarget.name);
      setDropTableTarget(null);
    });
  };

  const serverColumns: Column<ForeignServer>[] = [
    { key: "name", header: "Name" },
    { key: "fdw_type", header: "Type" },
    {
      key: "created_at",
      header: "Created",
      render: (row) => new Date(row.created_at).toLocaleDateString(),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <button
          onClick={() => setDropServerTarget(row.name)}
          className="text-xs text-red-500 hover:text-red-600"
          aria-label={`Drop ${row.name}`}
        >
          Drop
        </button>
      ),
    },
  ];

  const tableColumns: Column<ForeignTable>[] = [
    { key: "schema", header: "Schema" },
    { key: "name", header: "Table" },
    { key: "server_name", header: "Server" },
    {
      key: "columns",
      header: "Columns",
      render: (row) => String(row.columns?.length ?? 0),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <button
          onClick={() => setDropTableTarget({ schema: row.schema, name: row.name })}
          className="text-xs text-red-500 hover:text-red-600"
          aria-label={`Drop ${row.schema}.${row.name}`}
        >
          Drop
        </button>
      ),
    },
  ];

  const error = servers.error || tables.error;

  if (error) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          FDW Management
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
        FDW Management
      </h2>

      {/* Servers Section */}
      <div className="mb-8">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
            Foreign Servers
          </h3>
          <button
            onClick={() => setShowCreateServer(true)}
            className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded"
          >
            Add Server
          </button>
        </div>

        {showCreateServer && (
          <div className="mb-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900">
            <div className="grid grid-cols-2 gap-3 mb-3">
              <input
                placeholder="Server name"
                value={serverName}
                onChange={(e) => setServerName(e.target.value)}
                className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
              <select
                aria-label="Type"
                value={fdwType}
                onChange={(e) => setFdwType(e.target.value)}
                className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              >
                <option value="postgres_fdw">postgres_fdw</option>
                <option value="file_fdw">file_fdw</option>
              </select>
              {isPostgresFDW ? (
                <>
                  <input
                    placeholder="Host"
                    value={serverHost}
                    onChange={(e) => setServerHost(e.target.value)}
                    className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                  />
                  <input
                    placeholder="Port"
                    value={serverPort}
                    onChange={(e) => setServerPort(e.target.value)}
                    className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                  />
                  <input
                    placeholder="Database name"
                    value={serverDbname}
                    onChange={(e) => setServerDbname(e.target.value)}
                    className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                  />
                </>
              ) : (
                <input
                  placeholder="Filename"
                  value={serverFilename}
                  onChange={(e) => setServerFilename(e.target.value)}
                  className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                />
              )}
            </div>
            {isPostgresFDW && (
              <div className="grid grid-cols-2 gap-3 mb-3">
                <input
                  placeholder="User mapping user"
                  value={mappingUser}
                  onChange={(e) => setMappingUser(e.target.value)}
                  className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                />
                <input
                  type="password"
                  placeholder="User mapping password"
                  value={mappingPassword}
                  onChange={(e) => setMappingPassword(e.target.value)}
                  className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                />
              </div>
            )}
            <div className="flex gap-2">
              <button
                onClick={handleCreateServer}
                disabled={!createValid || servers.actionLoading}
                className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded disabled:opacity-50"
              >
                Create
              </button>
              <button
                onClick={() => {
                  setShowCreateServer(false);
                  resetCreateForm();
                }}
                className="px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-300 border border-gray-300 dark:border-gray-600 rounded hover:bg-gray-50 dark:hover:bg-gray-800"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {servers.loading ? (
          <p className="text-sm text-gray-500 dark:text-gray-400">Loading...</p>
        ) : (
          <AdminTable
            columns={serverColumns}
            rows={servers.data ?? []}
            rowKey="name"
            page={1}
            totalPages={1}
            onPageChange={() => {}}
            emptyMessage="No foreign servers"
          />
        )}
      </div>

      {/* Tables Section */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300">
            Foreign Tables
          </h3>
          <button
            onClick={() => setShowImport(true)}
            className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded"
          >
            Import Tables
          </button>
        </div>

        {showImport && (
          <div className="mb-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900">
            <div className="grid grid-cols-3 gap-3 mb-3">
              <select
                value={importServer}
                onChange={(e) => setImportServer(e.target.value)}
                className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              >
                <option value="">Select server...</option>
                {(servers.data ?? []).map((s) => (
                  <option key={s.name} value={s.name}>
                    {s.name}
                  </option>
                ))}
              </select>
              <input
                placeholder="Remote schema"
                value={remoteSchema}
                onChange={(e) => setRemoteSchema(e.target.value)}
                className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
              <input
                placeholder="Local schema"
                value={localSchema}
                onChange={(e) => setLocalSchema(e.target.value)}
                className="px-2 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </div>
            <div className="flex gap-2">
              <button
                onClick={handleImportTables}
                disabled={!importValid || tables.actionLoading}
                className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded disabled:opacity-50"
              >
                Import
              </button>
              <button
                onClick={() => {
                  setShowImport(false);
                  resetImportForm();
                }}
                className="px-3 py-1.5 text-xs font-medium text-gray-700 dark:text-gray-300 border border-gray-300 dark:border-gray-600 rounded hover:bg-gray-50 dark:hover:bg-gray-800"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {tables.loading ? (
          <p className="text-sm text-gray-500 dark:text-gray-400">Loading...</p>
        ) : (
          <AdminTable
            columns={tableColumns}
            rows={tables.data ?? []}
            rowKey="name"
            page={1}
            totalPages={1}
            onPageChange={() => {}}
            emptyMessage="No foreign tables"
          />
        )}
      </div>

      {/* Drop Server Confirm */}
      <ConfirmDialog
        open={dropServerTarget !== null}
        title="Drop Server"
        message={`Drop foreign server "${dropServerTarget}"? This will remove all associated foreign tables.`}
        confirmLabel="Drop"
        onConfirm={handleDropServer}
        onCancel={() => {
          setDropServerTarget(null);
          setDropServerCascade(false);
        }}
        destructive
        loading={servers.actionLoading}
      >
        <label className="flex items-center gap-2 mt-2 text-sm text-gray-700 dark:text-gray-300">
          <input
            type="checkbox"
            checked={dropServerCascade}
            onChange={(e) => setDropServerCascade(e.target.checked)}
          />
          CASCADE (drop dependent objects)
        </label>
      </ConfirmDialog>

      {/* Drop Table Confirm */}
      <ConfirmDialog
        open={dropTableTarget !== null}
        title="Drop Table"
        message={`Drop foreign table "${dropTableTarget?.schema}.${dropTableTarget?.name}"?`}
        confirmLabel="Drop"
        onConfirm={handleDropTable}
        onCancel={() => setDropTableTarget(null)}
        destructive
        loading={tables.actionLoading}
      />
    </div>
  );
}
