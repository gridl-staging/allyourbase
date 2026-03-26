import type { RlsPolicy } from "../types";

export type RlsPolicyModalState =
  | { kind: "none" }
  | { kind: "delete"; policy: RlsPolicy }
  | { kind: "sql-preview"; sql: string };

interface RlsPolicyActionModalsProps {
  modal: RlsPolicyModalState;
  isDeleting: boolean;
  onClose: () => void;
  onConfirmDelete: () => void;
}

export function RlsPolicyActionModals({
  modal,
  isDeleting,
  onClose,
  onConfirmDelete,
}: RlsPolicyActionModalsProps) {
  if (modal.kind === "none") {
    return null;
  }

  if (modal.kind === "delete") {
    return (
      <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-lg max-w-md w-full mx-4">
          <div className="px-6 py-4 border-b">
            <h2 className="font-semibold">Delete Policy</h2>
          </div>

          <div className="px-6 py-4">
            <p className="text-sm text-gray-600 dark:text-gray-300">
              This will permanently drop the policy <strong>{modal.policy.policyName}</strong> from{" "}
              <strong>{modal.policy.tableName}</strong>. This action cannot be undone.
            </p>
          </div>

          <div className="px-6 py-3 border-t flex justify-end gap-2">
            <button
              onClick={onClose}
              className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
            >
              Cancel
            </button>
            <button
              onClick={onConfirmDelete}
              disabled={isDeleting}
              className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
            >
              {isDeleting ? "Deleting..." : "Delete"}
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-lg max-w-lg w-full mx-4">
        <div className="px-6 py-4 border-b">
          <h2 className="font-semibold">SQL Preview</h2>
        </div>

        <div className="px-6 py-4">
          <pre className="p-3 text-xs font-mono bg-gray-900 text-gray-100 rounded overflow-x-auto whitespace-pre-wrap">
            {modal.sql}
          </pre>
        </div>

        <div className="px-6 py-3 border-t flex justify-end">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
