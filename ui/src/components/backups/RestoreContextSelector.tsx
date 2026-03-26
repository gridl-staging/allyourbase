import type { PITRContextOption } from "./context";

interface RestoreContextSelectorProps {
  contexts: PITRContextOption[];
  selectedContextKey: string;
  onSelectContext: (contextKey: string) => void;
}

export function RestoreContextSelector({
  contexts,
  selectedContextKey,
  onSelectContext,
}: RestoreContextSelectorProps) {
  if (contexts.length === 0) {
    return (
      <p className="mt-3 text-xs text-gray-500 dark:text-gray-400">
        Restore jobs require backups with `project_id` and `database_id`.
      </p>
    );
  }

  if (contexts.length === 1) {
    return null;
  }

  return (
    <>
      <div className="mt-4 max-w-sm">
        <label
          htmlFor="restore-context"
          className="block text-xs text-gray-600 dark:text-gray-300 mb-1"
        >
          Restore Context
        </label>
        <select
          id="restore-context"
          value={selectedContextKey}
          onChange={(e) => onSelectContext(e.target.value)}
          className="w-full border rounded px-3 py-2 text-sm bg-white dark:bg-gray-800"
        >
          <option value="">Select project / database</option>
          {contexts.map((context) => (
            <option key={context.key} value={context.key}>
              {context.label}
            </option>
          ))}
        </select>
        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Choose which project/database to use for restore jobs and PITR actions.
        </p>
      </div>

      {!selectedContextKey && (
        <p className="mt-3 text-xs text-gray-500 dark:text-gray-400">
          Select a project/database to load restore jobs and PITR actions.
        </p>
      )}
    </>
  );
}
