import { useState, useCallback } from "react";
import type { SchemaFunction } from "../types";
import { callRpc } from "../api";
import { Play, ChevronDown, ChevronRight } from "lucide-react";

interface FunctionBrowserProps {
  functions: Record<string, SchemaFunction>;
}

export function FunctionBrowser({ functions }: FunctionBrowserProps) {
  const fnList = Object.values(functions).sort((a, b) =>
    `${a.schema}.${a.name}`.localeCompare(`${b.schema}.${b.name}`),
  );

  const [expanded, setExpanded] = useState<string | null>(null);
  const [argValues, setArgValues] = useState<Record<string, string>>({});
  const [result, setResult] = useState<{
    fnKey: string;
    status: number;
    data: unknown;
    error?: string;
    durationMs: number;
  } | null>(null);
  const [loading, setLoading] = useState(false);

  const toggle = useCallback(
    (key: string) => {
      setExpanded((prev) => (prev === key ? null : key));
      setArgValues({});
      setResult(null);
    },
    [],
  );

  const setArg = useCallback((paramName: string, value: string) => {
    setArgValues((prev) => ({ ...prev, [paramName]: value }));
  }, []);

  const execute = useCallback(
    async (fn: SchemaFunction) => {
      const fnKey = `${fn.schema}.${fn.name}`;
      setLoading(true);
      setResult(null);

      const args: Record<string, unknown> = {};
      for (const p of fn.parameters || []) {
        const raw = argValues[p.name];
        if (raw !== undefined && raw !== "") {
          args[p.name] = coerceValue(raw, p.type);
        }
      }

      const start = performance.now();
      try {
        const res = await callRpc(fn.name, args);
        const durationMs = Math.round(performance.now() - start);
        setResult({ fnKey, status: res.status, data: res.data, durationMs });
      } catch (err: unknown) {
        const durationMs = Math.round(performance.now() - start);
        const message = err instanceof Error ? err.message : String(err);
        setResult({ fnKey, status: 0, data: null, error: message, durationMs });
      } finally {
        setLoading(false);
      }
    },
    [argValues],
  );

  if (fnList.length === 0) {
    return (
      <div className="p-8 text-center text-gray-400 text-sm">
        No functions found in the database.
      </div>
    );
  }

  return (
    <div className="p-6 max-w-4xl">
      <h2 className="text-lg font-semibold mb-4">
        Functions ({fnList.length})
      </h2>

      <div className="space-y-1">
        {fnList.map((fn) => {
          const key = `${fn.schema}.${fn.name}`;
          const isExpanded = expanded === key;
          const params = fn.parameters || [];
          const hasUnnamedParams = params.some((p) => !p.name);

          return (
            <div key={key} className="border rounded">
              <button
                onClick={() => toggle(key)}
                className="w-full text-left px-4 py-2.5 flex items-center gap-2 hover:bg-gray-50 text-sm"
              >
                {isExpanded ? (
                  <ChevronDown className="w-4 h-4 text-gray-400 shrink-0" />
                ) : (
                  <ChevronRight className="w-4 h-4 text-gray-400 shrink-0" />
                )}
                <code className="font-mono text-sm">
                  {fn.schema !== "public" && (
                    <span className="text-gray-400">{fn.schema}.</span>
                  )}
                  {fn.name}
                </code>
                <span className="text-xs text-gray-400">
                  ({params.map((p) => p.name || `$${p.position}`).join(", ")})
                </span>
                <span className="ml-auto text-xs text-gray-400">
                  {fn.returnsSet ? `SETOF ${fn.returnType}` : fn.returnType}
                </span>
              </button>

              {isExpanded && (
                <div className="px-4 pb-4 border-t bg-gray-50">
                  {fn.comment && (
                    <p className="text-xs text-gray-500 mt-2 mb-3">
                      {fn.comment}
                    </p>
                  )}

                  {hasUnnamedParams ? (
                    <p className="text-xs text-amber-600 mt-2">
                      This function has unnamed parameters and cannot be called
                      via the REST API.
                    </p>
                  ) : (
                    <>
                      {params.length > 0 && (
                        <div className="mt-3 space-y-2">
                          <p className="text-xs font-medium text-gray-500 uppercase tracking-wider">
                            Parameters
                          </p>
                          {params.map((p) => (
                            <div
                              key={p.position}
                              className="flex items-center gap-3"
                            >
                              <label className="text-xs text-gray-600 w-32 truncate font-mono">
                                {p.name}
                                <span className="text-gray-400 ml-1">
                                  ({p.type})
                                </span>
                              </label>
                              <input
                                type="text"
                                value={argValues[p.name] || ""}
                                onChange={(e) => setArg(p.name, e.target.value)}
                                onKeyDown={(e) => {
                                  if (e.key === "Enter") execute(fn);
                                }}
                                placeholder="NULL"
                                className="flex-1 border rounded px-2 py-1 text-sm font-mono bg-white"
                              />
                            </div>
                          ))}
                        </div>
                      )}

                      <button
                        onClick={() => execute(fn)}
                        disabled={loading}
                        className="mt-3 inline-flex items-center gap-1.5 px-3 py-1.5 bg-blue-600 text-white text-xs font-medium rounded hover:bg-blue-700 disabled:opacity-50"
                      >
                        <Play className="w-3 h-3" />
                        {loading ? "Executing..." : "Execute"}
                      </button>
                    </>
                  )}

                  {result && result.fnKey === key && (
                    <div className="mt-3">
                      {result.error ? (
                        <div className="bg-red-50 border border-red-200 rounded p-3">
                          <p className="text-xs text-red-700 font-medium">
                            Error
                          </p>
                          <p className="text-xs text-red-600 mt-1">
                            {result.error}
                          </p>
                        </div>
                      ) : (
                        <div className="bg-white border rounded">
                          <div className="px-3 py-1.5 border-b bg-gray-50 flex items-center justify-between">
                            <span className="text-xs text-gray-500">
                              Result
                              {result.status === 204 && " (void)"}
                            </span>
                            <span className="text-xs text-gray-400">
                              {result.durationMs}ms
                            </span>
                          </div>
                          <pre className="p-3 text-xs font-mono overflow-auto max-h-64 whitespace-pre-wrap">
                            {result.status === 204
                              ? "(no return value)"
                              : JSON.stringify(result.data, null, 2)}
                          </pre>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function coerceValue(raw: string, pgType: string): unknown {
  if (raw === "null" || raw === "NULL") return null;
  if (raw === "true") return true;
  if (raw === "false") return false;
  if (
    pgType.includes("int") ||
    pgType === "numeric" ||
    pgType === "float" ||
    pgType === "double precision" ||
    pgType === "real"
  ) {
    const n = Number(raw);
    if (!isNaN(n)) return n;
  }
  if (pgType === "json" || pgType === "jsonb") {
    try {
      return JSON.parse(raw);
    } catch {
      return raw;
    }
  }
  return raw;
}
