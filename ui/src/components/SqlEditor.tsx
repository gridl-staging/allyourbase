import { useState, useCallback, useMemo } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { sql, PostgreSQL } from "@codemirror/lang-sql";
import { keymap, EditorView } from "@codemirror/view";
import { executeSQL, ApiError } from "../api";
import type { SqlResult } from "../types";
import {
  Play,
  AlertCircle,
  Clock,
  CheckCircle2,
  FileJson,
  FileSpreadsheet,
} from "lucide-react";

function classifyQuery(q: string): "select" | "dml" | "ddl" | "other" {
  const first = q.trim().split(/\s+/)[0]?.toUpperCase();
  if (first === "SELECT" || first === "WITH" || first === "TABLE" || first === "VALUES")
    return "select";
  if (["INSERT", "UPDATE", "DELETE", "MERGE"].includes(first)) return "dml";
  if (
    ["CREATE", "ALTER", "DROP", "TRUNCATE", "GRANT", "REVOKE", "COMMENT"].includes(
      first,
    )
  )
    return "ddl";
  return "other";
}

export function resultToCSV(result: SqlResult): string {
  const escape = (v: unknown) => {
    if (v === null) return "";
    const s = typeof v === "object" ? JSON.stringify(v) : String(v);
    return s.includes(",") || s.includes('"') || s.includes("\n")
      ? `"${s.replace(/"/g, '""')}"`
      : s;
  };
  const header = result.columns.map(escape).join(",");
  const rows = result.rows.map((row) => row.map(escape).join(","));
  return [header, ...rows].join("\n");
}

export function resultToJSON(result: SqlResult): string {
  const objects = result.rows.map((row) => {
    const obj: Record<string, unknown> = {};
    result.columns.forEach((col, i) => {
      obj[col] = row[i];
    });
    return obj;
  });
  return JSON.stringify(objects, null, 2);
}

interface SqlEditorProps {
  onSchemaChange?: () => void;
}

export function SqlEditor({ onSchemaChange }: SqlEditorProps = {}) {
  const [query, setQuery] = useState(
    () => localStorage.getItem("ayb_sql_query") || "SELECT 1 AS hello;",
  );
  const [result, setResult] = useState<SqlResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [lastQuery, setLastQuery] = useState<string>("");
  const [copyFeedback, setCopyFeedback] = useState<string | null>(null);

  const execute = useCallback(async () => {
    const trimmed = query.trim();
    if (!trimmed) return;

    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const res = await executeSQL(trimmed);
      setResult(res);
      setLastQuery(trimmed);
      localStorage.setItem("ayb_sql_query", trimmed);
      // Auto-refresh schema after DDL (CREATE, ALTER, DROP, etc.)
      if (classifyQuery(trimmed) === "ddl" && onSchemaChange) {
        onSchemaChange();
      }
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError(String(err));
      }
    } finally {
      setLoading(false);
    }
  }, [query, onSchemaChange]);

  // Stable reference for Cmd+Enter keymap — reads current query via closure
  // but the keymap extension itself is only created once.
  const executeKeymapExt = useMemo(() => {
    // We use a Prec-less keymap; Mod-Enter is unlikely to collide.
    return keymap.of([
      {
        key: "Mod-Enter",
        run: () => {
          execute();
          return true;
        },
      },
    ]);
    // execute is stable (useCallback) so this is fine
  }, [execute]);

  const extensions = useMemo(
    () => [sql({ dialect: PostgreSQL }), executeKeymapExt, EditorView.contentAttributes.of({ "aria-label": "SQL query" })],
    [executeKeymapExt],
  );

  const copyToClipboard = useCallback(
    async (format: "csv" | "json") => {
      if (!result) return;
      const text = format === "csv" ? resultToCSV(result) : resultToJSON(result);
      try {
        await navigator.clipboard.writeText(text);
      } catch {
        // Fallback for environments without clipboard API
      }
      setCopyFeedback(format === "csv" ? "CSV copied!" : "JSON copied!");
      setTimeout(() => setCopyFeedback(null), 2000);
    },
    [result],
  );

  const feedbackMessage = useMemo(() => {
    if (!result) return null;
    const qtype = classifyQuery(lastQuery);
    const dur = `${result.durationMs}ms`;

    if (result.columns.length > 0) {
      // SELECT-style query that returned rows
      return {
        icon: "clock" as const,
        text: `${result.rowCount} row${result.rowCount !== 1 ? "s" : ""} in ${dur}`,
      };
    }

    // No columns — DDL/DML
    if (qtype === "ddl") {
      return {
        icon: "check" as const,
        text: `Statement executed successfully in ${dur}`,
      };
    }
    if (qtype === "dml") {
      return {
        icon: "check" as const,
        text: `${result.rowCount} row${result.rowCount !== 1 ? "s" : ""} affected in ${dur}`,
      };
    }
    // fallback
    return {
      icon: "check" as const,
      text: `Query OK — ${result.rowCount} row${result.rowCount !== 1 ? "s" : ""} affected in ${dur}`,
    };
  }, [result, lastQuery]);

  return (
    <div className="flex flex-col h-full">
      {/* Editor area */}
      <div className="border-b p-4">
        <div className="border rounded-lg overflow-hidden">
          <CodeMirror
            value={query}
            onChange={setQuery}
            extensions={extensions}
            height="160px"
            minHeight="80px"
            maxHeight="400px"
            placeholder="Enter SQL query..."
            basicSetup={{
              lineNumbers: true,
              foldGutter: false,
              highlightActiveLine: true,
              bracketMatching: true,
              autocompletion: true,
            }}
          />
        </div>
        <div className="mt-2 flex items-center gap-3">
          <button
            onClick={execute}
            disabled={loading || !query.trim()}
            className="px-4 py-1.5 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
          >
            <Play className="w-3.5 h-3.5" />
            {loading ? "Running..." : "Execute"}
          </button>
          <span className="text-xs text-gray-400">
            {navigator.platform.includes("Mac") ? "\u2318" : "Ctrl"}+Enter to run
          </span>
          {feedbackMessage && feedbackMessage.icon === "clock" && (
            <span className="ml-auto text-xs text-gray-500 flex items-center gap-1">
              <Clock className="w-3 h-3" />
              {feedbackMessage.text}
            </span>
          )}
        </div>
      </div>

      {/* Results area */}
      <div className="flex-1 overflow-auto">
        {error && (
          <div className="m-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
            <AlertCircle className="w-4 h-4 text-red-500 mt-0.5 shrink-0" />
            <pre className="text-sm text-red-700 whitespace-pre-wrap font-mono">
              {error}
            </pre>
          </div>
        )}

        {result && result.columns.length > 0 && (
          <div className="relative">
            {/* Copy buttons */}
            <div className="absolute top-2 right-2 flex items-center gap-1 z-10">
              {copyFeedback && (
                <span className="text-xs text-green-600 mr-1">{copyFeedback}</span>
              )}
              <button
                onClick={() => copyToClipboard("csv")}
                title="Copy as CSV"
                className="p-1.5 text-gray-400 hover:text-gray-600 hover:bg-gray-100 rounded transition-colors"
              >
                <FileSpreadsheet className="w-4 h-4" />
              </button>
              <button
                onClick={() => copyToClipboard("json")}
                title="Copy as JSON"
                className="p-1.5 text-gray-400 hover:text-gray-600 hover:bg-gray-100 rounded transition-colors"
              >
                <FileJson className="w-4 h-4" />
              </button>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-gray-50">
                    {result.columns.map((col) => (
                      <th
                        key={col}
                        className="px-4 py-2 text-left font-medium text-gray-600 whitespace-nowrap"
                      >
                        {col}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {result.rows.map((row, i) => (
                    <tr key={i} className="border-b hover:bg-gray-50">
                      {row.map((cell, j) => (
                        <td
                          key={j}
                          className="px-4 py-2 whitespace-nowrap font-mono text-xs"
                        >
                          {cell === null ? (
                            <span className="text-gray-300 italic">null</span>
                          ) : typeof cell === "object" ? (
                            JSON.stringify(cell)
                          ) : (
                            String(cell)
                          )}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {result && result.columns.length === 0 && feedbackMessage && (
          <div className="m-4 p-3 bg-green-50 border border-green-200 rounded-lg flex items-center gap-2">
            <CheckCircle2 className="w-4 h-4 text-green-600 shrink-0" />
            <span className="text-sm text-green-700">{feedbackMessage.text}</span>
          </div>
        )}

        {!result && !error && (
          <div className="flex-1 flex items-center justify-center text-gray-400 text-sm h-48">
            Run a query to see results
          </div>
        )}
      </div>
    </div>
  );
}
