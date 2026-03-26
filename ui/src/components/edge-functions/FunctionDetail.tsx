import { useState, useEffect, useCallback } from "react";
import { ArrowLeft, Loader2 } from "lucide-react";
import { cn } from "../../lib/utils";
import { getEdgeFunction, listEdgeFunctionLogs } from "../../api";
import type { EdgeFunctionResponse, EdgeFunctionLogEntry } from "../../types";
import { FunctionEditor } from "./FunctionEditor";
import { FunctionLogs } from "./FunctionLogs";
import { InvokeTester } from "./InvokeTester";
import { EdgeFunctionTriggers } from "../EdgeFunctionTriggers";

interface FunctionDetailProps {
  id: string;
  onBack: () => void;
  addToast: (type: "success" | "error", message: string) => void;
}

type Tab = "editor" | "logs" | "tester" | "triggers";

export function FunctionDetail({ id, onBack, addToast }: FunctionDetailProps) {
  const [fn, setFn] = useState<EdgeFunctionResponse | null>(null);
  const [logs, setLogs] = useState<EdgeFunctionLogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useState<Tab>("editor");

  useEffect(() => {
    (async () => {
      try {
        const [fnData, logData] = await Promise.all([
          getEdgeFunction(id),
          listEdgeFunctionLogs(id),
        ]);
        setFn(fnData);
        setLogs(logData);
      } catch {
        addToast("error", "Failed to load function details");
        onBack();
      } finally {
        setLoading(false);
      }
    })();
  }, [id, addToast, onBack]);

  const handleFnUpdate = useCallback((updated: EdgeFunctionResponse) => {
    setFn(updated);
  }, []);

  const handleLogsUpdate = useCallback((newLogs: EdgeFunctionLogEntry[]) => {
    setLogs(newLogs);
  }, []);

  const handleDelete = useCallback(() => {
    onBack();
  }, [onBack]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12 text-gray-400 dark:text-gray-500">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading function...
      </div>
    );
  }

  if (!fn) return null;

  const TABS: { key: Tab; label: string }[] = [
    { key: "editor", label: "Editor" },
    { key: "logs", label: "Logs" },
    { key: "tester", label: "Invoke" },
    { key: "triggers", label: "Triggers" },
  ];

  return (
    <div className="p-6">
      <div className="flex items-center gap-3 mb-6">
        <button
          onClick={onBack}
          className="p-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
          aria-label="Back"
        >
          <ArrowLeft className="w-4 h-4" />
        </button>
        <h1 className="text-xl font-semibold">{fn.name}</h1>
        <span
          className={cn(
            "px-2 py-0.5 rounded text-xs font-medium",
            fn.public
              ? "bg-green-100 text-green-700"
              : "bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300",
          )}
        >
          {fn.public ? "Public" : "Private"}
        </span>
      </div>

      <div className="flex gap-1 bg-gray-100 dark:bg-gray-700 rounded p-0.5 mb-6 w-fit">
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={cn(
              "px-3 py-1 text-xs rounded font-medium",
              tab === t.key
                ? "bg-white dark:bg-gray-800 shadow-sm text-gray-900 dark:text-gray-100"
                : "text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 dark:text-gray-200",
            )}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "editor" && (
        <FunctionEditor
          fn={fn}
          onFnUpdate={handleFnUpdate}
          onDelete={handleDelete}
          addToast={addToast}
        />
      )}

      {tab === "logs" && (
        <FunctionLogs
          functionId={fn.id}
          logs={logs}
          onLogsUpdate={handleLogsUpdate}
          addToast={addToast}
        />
      )}

      {tab === "tester" && (
        <InvokeTester
          fn={fn}
          onLogsUpdate={handleLogsUpdate}
          addToast={addToast}
        />
      )}

      {tab === "triggers" && (
        <EdgeFunctionTriggers functionId={fn.id} addToast={addToast} />
      )}
    </div>
  );
}
