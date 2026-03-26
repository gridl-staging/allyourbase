import { useState, useEffect, useMemo, useCallback } from "react";
import { Loader2, Trash2, RotateCcw, AlertCircle } from "lucide-react";
import CodeMirror from "@uiw/react-codemirror";
import { javascript } from "@codemirror/lang-javascript";
import { EnvVarEditor, envVarsToMap, hasEnvVarErrors } from "./EnvVarEditor";
import type { EnvVar } from "./EnvVarEditor";
import { updateEdgeFunction, deleteEdgeFunction } from "../../api";
import type { EdgeFunctionResponse } from "../../types";
import { useCodeMirrorTheme } from "../codeMirrorTheme";

interface FunctionEditorProps {
  fn: EdgeFunctionResponse;
  onFnUpdate: (fn: EdgeFunctionResponse) => void;
  onDelete: () => void;
  addToast: (type: "success" | "error", message: string) => void;
}

function envVarsFromFn(fn: EdgeFunctionResponse): EnvVar[] {
  return fn.envVars
    ? Object.entries(fn.envVars).map(([key, value]) => ({ key, value }))
    : [];
}

export function FunctionEditor({ fn, onFnUpdate, onDelete, addToast }: FunctionEditorProps) {
  const [source, setSource] = useState(fn.source);
  const [entryPoint, setEntryPoint] = useState(fn.entryPoint);
  const [timeoutMs, setTimeoutMs] = useState(Math.round(fn.timeout / 1_000_000));
  const [isPublic, setIsPublic] = useState(fn.public);
  const [envVars, setEnvVars] = useState<EnvVar[]>(envVarsFromFn(fn));
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deployError, setDeployError] = useState<string | null>(null);
  const codeMirrorTheme = useCodeMirrorTheme();

  // Snapshot for dirty detection — recalculate when fn changes (after save)
  const [snapshot, setSnapshot] = useState({
    source: fn.source,
    entryPoint: fn.entryPoint,
    timeoutMs: Math.round(fn.timeout / 1_000_000),
    isPublic: fn.public,
    envVars: envVarsFromFn(fn),
  });

  // Update snapshot when fn prop changes (e.g. after save)
  useEffect(() => {
    const newSnap = {
      source: fn.source,
      entryPoint: fn.entryPoint,
      timeoutMs: Math.round(fn.timeout / 1_000_000),
      isPublic: fn.public,
      envVars: envVarsFromFn(fn),
    };
    setSnapshot(newSnap);
    setSource(fn.source);
    setEntryPoint(fn.entryPoint);
    setTimeoutMs(newSnap.timeoutMs);
    setIsPublic(fn.public);
    setEnvVars(newSnap.envVars);
  }, [fn]);

  const isDirty = useMemo(() => {
    if (source !== snapshot.source) return true;
    if (entryPoint !== snapshot.entryPoint) return true;
    if (timeoutMs !== snapshot.timeoutMs) return true;
    if (isPublic !== snapshot.isPublic) return true;
    if (envVars.length !== snapshot.envVars.length) return true;
    for (let i = 0; i < envVars.length; i++) {
      if (envVars[i].key !== snapshot.envVars[i].key) return true;
      if (envVars[i].value !== snapshot.envVars[i].value) return true;
    }
    return false;
  }, [source, entryPoint, timeoutMs, isPublic, envVars, snapshot]);

  const handleRevert = useCallback(() => {
    setSource(snapshot.source);
    setEntryPoint(snapshot.entryPoint);
    setTimeoutMs(snapshot.timeoutMs);
    setIsPublic(snapshot.isPublic);
    setEnvVars([...snapshot.envVars]);
    setDeployError(null);
  }, [snapshot]);

  const handleSave = async () => {
    if (hasEnvVarErrors(envVars)) {
      addToast("error", "Fix environment variable errors before saving");
      return;
    }
    setSaving(true);
    setDeployError(null);
    try {
      const updated = await updateEdgeFunction(fn.id, {
        source,
        entry_point: entryPoint,
        timeout_ms: timeoutMs,
        env_vars: envVarsToMap(envVars),
        public: isPublic,
      });
      onFnUpdate(updated);
      addToast("success", "Function saved");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Save failed";
      // Show compile/deploy errors inline rather than just in toast
      if (msg.toLowerCase().includes("compile") || msg.toLowerCase().includes("transpil") || msg.toLowerCase().includes("syntax")) {
        setDeployError(msg);
      }
      addToast("error", msg);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    try {
      await deleteEdgeFunction(fn.id);
      addToast("success", "Function deleted");
      onDelete();
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Delete failed");
      setDeleting(false);
      setConfirmDelete(false);
    }
  };

  return (
    <div className="space-y-4 text-gray-900 dark:text-gray-100">
      {/* Dirty indicator */}
      {isDirty && (
        <div
          className="flex items-center gap-2 px-3 py-1.5 bg-yellow-50 border border-yellow-200 rounded text-sm text-yellow-800"
          data-testid="dirty-indicator"
        >
          <span>Unsaved changes</span>
          <button
            onClick={handleRevert}
            className="flex items-center gap-1 text-xs text-yellow-700 hover:text-yellow-900 underline"
            data-testid="revert-btn"
          >
            <RotateCcw className="w-3 h-3" />
            Revert
          </button>
        </div>
      )}

      {/* Compile/deploy error banner */}
      {deployError && (
        <div
          className="flex items-start gap-2 px-3 py-2 bg-red-50 border border-red-200 rounded text-sm text-red-800"
          data-testid="deploy-error"
        >
          <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
          <div>
            <span className="font-medium">Deploy error: </span>
            <span>{deployError}</span>
          </div>
        </div>
      )}

      <div>
        <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Source</label>
        <CodeMirror
          value={source}
          onChange={(val) => {
            setSource(val);
            if (deployError) setDeployError(null);
          }}
          extensions={[javascript({ typescript: true })]}
          theme={codeMirrorTheme}
          data-testid="codemirror-editor"
          className="border border-gray-300 dark:border-gray-600 rounded overflow-hidden"
          height="300px"
        />
      </div>

      <div className="grid grid-cols-3 gap-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Entry Point</label>
          <input
            type="text"
            value={entryPoint}
            onChange={(e) => setEntryPoint(e.target.value)}
            className="w-full px-3 py-1.5 border border-gray-300 dark:border-gray-600 rounded text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
            data-testid="editor-entry-point"
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Timeout (ms)</label>
          <input
            type="number"
            value={timeoutMs}
            onChange={(e) => setTimeoutMs(Number(e.target.value))}
            className="w-full px-3 py-1.5 border border-gray-300 dark:border-gray-600 rounded text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
            data-testid="editor-timeout"
          />
        </div>
        <div className="flex items-end gap-2 pb-0.5">
          <label className="flex items-center gap-2 text-sm cursor-pointer">
            <input
              type="checkbox"
              checked={isPublic}
              onChange={(e) => setIsPublic(e.target.checked)}
              data-testid="editor-public"
            />
            Public
          </label>
        </div>
      </div>

      <EnvVarEditor envVars={envVars} onChange={setEnvVars} />

      {/* Actions */}
      <div className="flex items-center gap-3 pt-2 border-t">
        <button
          onClick={handleSave}
          disabled={saving}
          className="flex items-center gap-1.5 px-4 py-1.5 bg-gray-900 text-white rounded text-sm hover:bg-gray-800 disabled:opacity-50"
          data-testid="editor-save"
        >
          {saving && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
          Save
        </button>
        {!confirmDelete ? (
          <button
            onClick={() => setConfirmDelete(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 text-red-600 hover:bg-red-50 rounded text-sm"
            data-testid="editor-delete"
          >
            <Trash2 className="w-3.5 h-3.5" />
            Delete
          </button>
        ) : (
          <div className="flex items-center gap-2 bg-red-50 border border-red-200 rounded px-3 py-1.5">
            <span className="text-sm text-red-700">Are you sure?</span>
            <button
              onClick={handleDelete}
              disabled={deleting}
              className="px-2 py-0.5 bg-red-600 text-white rounded text-xs hover:bg-red-700 disabled:opacity-50"
              data-testid="editor-confirm-delete"
            >
              {deleting ? "Deleting..." : "Confirm"}
            </button>
            <button
              onClick={() => setConfirmDelete(false)}
              className="px-2 py-0.5 text-gray-600 dark:text-gray-300 text-xs hover:text-gray-800 dark:hover:text-gray-100"
            >
              Cancel
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
