import { Loader2 } from "lucide-react";
import type { PITRValidateResponse } from "../../types/backups";
import { formatBytes, formatDate } from "../shared/format";

interface PITRPanelProps {
  hasContext: boolean;
  targetTime: string;
  onTargetTimeChange: (nextTime: string) => void;
  dryRun: boolean;
  onDryRunChange: (enabled: boolean) => void;
  validating: boolean;
  restoring: boolean;
  validationResult: PITRValidateResponse | null;
  onValidate: () => void;
  onRestore: () => void;
}

export function PITRPanel({
  hasContext,
  targetTime,
  onTargetTimeChange,
  dryRun,
  onDryRunChange,
  validating,
  restoring,
  validationResult,
  onValidate,
  onRestore,
}: PITRPanelProps) {
  return (
    <div className="mt-8 border rounded-lg p-4 bg-gray-50 dark:bg-gray-800/30">
      <h2 className="text-md font-semibold mb-3">Point-In-Time Recovery</h2>
      <div className="grid gap-3 md:grid-cols-3">
        <div className="md:col-span-2">
          <label
            htmlFor="pitr-target-time"
            className="block text-xs text-gray-600 dark:text-gray-300 mb-1"
          >
            Target Time
          </label>
          <input
            id="pitr-target-time"
            type="datetime-local"
            aria-label="Target Time"
            value={targetTime}
            onChange={(e) => onTargetTimeChange(e.target.value)}
            className="w-full border rounded px-3 py-2 text-sm"
            disabled={!hasContext}
          />
        </div>
        <div className="flex items-end">
          <button
            onClick={onValidate}
            disabled={!hasContext || !targetTime || validating}
            className="w-full px-3 py-2 text-sm border rounded hover:bg-gray-100 disabled:opacity-50 inline-flex items-center justify-center gap-1.5"
          >
            {validating && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
            Validate PITR
          </button>
        </div>
      </div>

      <div className="mt-3 flex items-center gap-2">
        <input
          id="pitr-dry-run"
          type="checkbox"
          checked={dryRun}
          onChange={(e) => onDryRunChange(e.target.checked)}
          disabled={!hasContext}
        />
        <label htmlFor="pitr-dry-run" className="text-sm">
          Dry run
        </label>
        <button
          onClick={onRestore}
          disabled={!hasContext || !targetTime || !validationResult || restoring}
          className="ml-auto px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 inline-flex items-center gap-1.5"
        >
          {restoring && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
          Start Restore
        </button>
      </div>

      {validationResult && (
        <div className="mt-3 text-xs text-gray-600 dark:text-gray-300 space-y-1">
          <p>Earliest: {formatDate(validationResult.earliest_recoverable)}</p>
          <p>Latest: {formatDate(validationResult.latest_recoverable)}</p>
          <p>
            Estimated WAL: {formatBytes(validationResult.estimated_wal_bytes)} (
            {validationResult.wal_segments_count} segments)
          </p>
        </div>
      )}
    </div>
  );
}
