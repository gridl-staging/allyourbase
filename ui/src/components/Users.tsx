import { useState, useEffect, useCallback } from "react";
import type { AdminUser, UserListResponse } from "../types";
import { listUsers, deleteUser } from "../api";
import {
  Search,
  Trash2,
  X,
  Loader2,
  AlertCircle,
  Users as UsersIcon,
  ChevronLeft,
  ChevronRight,
  CheckCircle,
  XCircle,
} from "lucide-react";
import { ToastContainer, useToast } from "./Toast";

const PER_PAGE = 20;

type Modal =
  | { kind: "none" }
  | { kind: "delete"; user: AdminUser };

export function Users() {
  const [data, setData] = useState<UserListResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [appliedSearch, setAppliedSearch] = useState("");
  const [modal, setModal] = useState<Modal>({ kind: "none" });
  const [deleting, setDeleting] = useState(false);
  const { toasts, addToast, removeToast } = useToast();

  const fetchUsers = useCallback(async () => {
    try {
      setError(null);
      const result = await listUsers({
        page,
        perPage: PER_PAGE,
        search: appliedSearch || undefined,
      });
      setData(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load users");
      setData(null);
    } finally {
      setLoading(false);
    }
  }, [page, appliedSearch]);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  const handleSearch = useCallback(() => {
    setAppliedSearch(search);
    setPage(1);
  }, [search]);

  const handleDelete = async (user: AdminUser) => {
    setDeleting(true);
    try {
      await deleteUser(user.id);
      setModal({ kind: "none" });
      addToast("success", `User ${user.email} deleted`);
      fetchUsers();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Delete failed");
    } finally {
      setDeleting(false);
    }
  };

  if (loading && !data) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading users...
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
              fetchUsers();
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
          <h1 className="text-lg font-semibold">Users</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Manage registered user accounts
          </p>
        </div>
      </div>

      {/* Search bar */}
      <div className="mb-4 flex items-center gap-2">
        <div className="flex items-center gap-2 flex-1 max-w-md border rounded px-3 py-1.5 bg-white">
          <Search className="w-4 h-4 text-gray-400" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleSearch()}
            placeholder="Search by email..."
            className="flex-1 bg-transparent text-sm outline-none placeholder-gray-400"
            aria-label="Search users"
          />
          {search && (
            <button
              onClick={() => {
                setSearch("");
                setAppliedSearch("");
                setPage(1);
              }}
              className="text-gray-400 hover:text-gray-600"
              aria-label="Clear search"
            >
              <X className="w-4 h-4" />
            </button>
          )}
        </div>
        <button
          onClick={handleSearch}
          className="px-3 py-1.5 text-xs bg-gray-200 hover:bg-gray-300 rounded font-medium"
        >
          Search
        </button>
      </div>

      {data && data.items.length === 0 ? (
        <div className="text-center py-16 border rounded-lg bg-gray-50">
          <UsersIcon className="w-10 h-10 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500 text-sm">
            {appliedSearch ? "No users matching search" : "No users registered yet"}
          </p>
        </div>
      ) : data ? (
        <>
          <div className="border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    Email
                  </th>
                  <th className="text-center px-4 py-2 font-medium text-gray-600">
                    Verified
                  </th>
                  <th className="text-left px-4 py-2 font-medium text-gray-600">
                    Created
                  </th>
                  <th className="text-right px-4 py-2 font-medium text-gray-600">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((user) => (
                  <tr
                    key={user.id}
                    className="border-b last:border-0 hover:bg-gray-50"
                  >
                    <td className="px-4 py-2.5">
                      <span className="font-mono text-xs">{user.email}</span>
                      <div className="text-[10px] text-gray-400 mt-0.5">
                        {user.id}
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-center">
                      {user.emailVerified ? (
                        <CheckCircle className="w-4 h-4 text-green-500 inline" />
                      ) : (
                        <XCircle className="w-4 h-4 text-gray-300 inline" />
                      )}
                    </td>
                    <td className="px-4 py-2.5 text-gray-500 text-xs">
                      {new Date(user.createdAt).toLocaleDateString()}
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex justify-end">
                        <button
                          onClick={() => setModal({ kind: "delete", user })}
                          className="p-1 text-gray-400 hover:text-red-500 rounded hover:bg-gray-100"
                          title="Delete user"
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

          {/* Pagination */}
          <div className="mt-3 flex items-center justify-between text-sm text-gray-500">
            <span>
              {data.totalItems} user{data.totalItems !== 1 ? "s" : ""}
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
                className="p-1 rounded hover:bg-gray-200 disabled:opacity-30"
              >
                <ChevronLeft className="w-4 h-4" />
              </button>
              <span>
                {page} / {data.totalPages || 1}
              </span>
              <button
                onClick={() => setPage((p) => Math.min(data.totalPages, p + 1))}
                disabled={page >= data.totalPages}
                className="p-1 rounded hover:bg-gray-200 disabled:opacity-30"
              >
                <ChevronRight className="w-4 h-4" />
              </button>
            </div>
          </div>
        </>
      ) : null}

      {/* Delete confirmation */}
      {modal.kind === "delete" && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
          <div className="bg-white rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
            <h3 className="font-semibold mb-2">Delete User</h3>
            <p className="text-sm text-gray-600 mb-1">
              This will permanently delete the user and all their sessions.
            </p>
            <p className="text-xs font-mono text-gray-500 break-all mb-4">
              {modal.user.email}
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-100 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={() => handleDelete(modal.user)}
                disabled={deleting}
                className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
              >
                {deleting ? "Deleting..." : "Delete"}
              </button>
            </div>
          </div>
        </div>
      )}

      <ToastContainer toasts={toasts} onRemove={removeToast} />
    </div>
  );
}
