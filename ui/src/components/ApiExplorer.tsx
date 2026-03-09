import { useState, useCallback, useEffect } from "react";
import { executeApiExplorer, ApiError } from "../api";
import type {
  SchemaCache,
  Table,
  ApiExplorerResponse,
  ApiExplorerHistoryEntry,
} from "../types";
import {
  Play,
  Clock,
  Copy,
  Check,
  ChevronDown,
  ChevronRight,
  Trash2,
  AlertCircle,
} from "lucide-react";
import { cn } from "../lib/utils";

const METHODS = ["GET", "POST", "PATCH", "DELETE"] as const;
type Method = (typeof METHODS)[number];

const HISTORY_KEY = "ayb_api_explorer_history";
const MAX_HISTORY = 20;

const METHOD_COLORS: Record<Method, string> = {
  GET: "bg-green-100 text-green-700",
  POST: "bg-blue-100 text-blue-700",
  PATCH: "bg-yellow-100 text-yellow-700",
  DELETE: "bg-red-100 text-red-700",
};

interface ApiExplorerProps {
  schema: SchemaCache;
}

function loadHistory(): ApiExplorerHistoryEntry[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function saveHistory(history: ApiExplorerHistoryEntry[]) {
  localStorage.setItem(HISTORY_KEY, JSON.stringify(history.slice(0, MAX_HISTORY)));
}

function formatJson(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

function generateCurl(method: string, fullUrl: string, body?: string): string {
  let cmd = `curl -X ${method}`;
  cmd += ` \\\n  -H "Authorization: Bearer <TOKEN>"`;
  if (body && (method === "POST" || method === "PATCH")) {
    cmd += ` \\\n  -H "Content-Type: application/json"`;
    cmd += ` \\\n  -d '${body}'`;
  }
  cmd += ` \\\n  "${fullUrl}"`;
  return cmd;
}

function generateJsSdk(
  method: string,
  path: string,
  body?: string,
): string {
  // Parse the path to detect collection CRUD
  const collMatch = path.match(/^\/api\/collections\/([^/?]+)(?:\/([^/?]+))?/);
  if (collMatch) {
    const table = collMatch[1];
    const id = collMatch[2];
    const qs = path.includes("?") ? path.split("?")[1] : "";
    const params = new URLSearchParams(qs);

    if (method === "GET" && !id) {
      const opts: string[] = [];
      for (const [k, v] of params) opts.push(`  ${k}: "${v}"`);
      const optsStr = opts.length > 0 ? `, {\n${opts.join(",\n")}\n}` : "";
      return `const { items } = await ayb.records.list("${table}"${optsStr});`;
    }
    if (method === "GET" && id) {
      return `const record = await ayb.records.get("${table}", "${id}");`;
    }
    if (method === "POST") {
      const parsed = body ? JSON.parse(body) : {};
      return `const record = await ayb.records.create("${table}", ${JSON.stringify(parsed, null, 2)});`;
    }
    if (method === "PATCH" && id) {
      const parsed = body ? JSON.parse(body) : {};
      return `const record = await ayb.records.update("${table}", "${id}", ${JSON.stringify(parsed, null, 2)});`;
    }
    if (method === "DELETE" && id) {
      return `await ayb.records.delete("${table}", "${id}");`;
    }
  }

  // RPC
  const rpcMatch = path.match(/^\/api\/rpc\/([^/?]+)/);
  if (rpcMatch && method === "POST") {
    const fn = rpcMatch[1];
    const parsed = body ? JSON.parse(body) : {};
    return `const result = await ayb.rpc("${fn}", ${JSON.stringify(parsed, null, 2)});`;
  }

  // Fallback to fetch
  let code = `const res = await fetch("${path}", {\n  method: "${method}",\n  headers: {\n    "Authorization": "Bearer <TOKEN>"`;
  if (body && (method === "POST" || method === "PATCH")) {
    code += `,\n    "Content-Type": "application/json"`;
  }
  code += `\n  }`;
  if (body && (method === "POST" || method === "PATCH")) {
    code += `,\n  body: JSON.stringify(${body})`;
  }
  code += `\n});\nconst data = await res.json();`;
  return code;
}

function statusColor(status: number): string {
  if (status >= 200 && status < 300) return "text-green-600";
  if (status >= 300 && status < 400) return "text-yellow-600";
  if (status >= 400 && status < 500) return "text-orange-600";
  return "text-red-600";
}

export function ApiExplorer({ schema }: ApiExplorerProps) {
  const tables = Object.values(schema.tables).sort((a, b) =>
    `${a.schema}.${a.name}`.localeCompare(`${b.schema}.${b.name}`),
  );

  const [method, setMethod] = useState<Method>("GET");
  const [path, setPath] = useState("/api/collections/");
  const [body, setBody] = useState("");
  const [response, setResponse] = useState<ApiExplorerResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [history, setHistory] = useState<ApiExplorerHistoryEntry[]>(loadHistory);
  const [showHistory, setShowHistory] = useState(false);
  const [snippetTab, setSnippetTab] = useState<"curl" | "js">("curl");
  const [copied, setCopied] = useState(false);
  const [showParams, setShowParams] = useState(false);

  // Query param state
  const [filter, setFilter] = useState("");
  const [sort, setSort] = useState("");
  const [page, setPage] = useState("");
  const [perPage, setPerPage] = useState("");
  const [fields, setFields] = useState("");
  const [expand, setExpand] = useState("");
  const [search, setSearch] = useState("");

  // Build full path with query params
  const buildFullPath = useCallback(() => {
    const qs = new URLSearchParams();
    if (filter) qs.set("filter", filter);
    if (sort) qs.set("sort", sort);
    if (page) qs.set("page", page);
    if (perPage) qs.set("perPage", perPage);
    if (fields) qs.set("fields", fields);
    if (expand) qs.set("expand", expand);
    if (search) qs.set("search", search);
    const suffix = qs.toString() ? `?${qs}` : "";
    return `${path}${suffix}`;
  }, [path, filter, sort, page, perPage, fields, expand, search]);

  const execute = useCallback(async () => {
    const fullPath = buildFullPath();
    if (!fullPath) return;

    setLoading(true);
    setError(null);
    setResponse(null);

    try {
      const res = await executeApiExplorer(
        method,
        fullPath,
        body || undefined,
      );
      setResponse(res);

      const entry: ApiExplorerHistoryEntry = {
        method,
        path: fullPath,
        body: body || undefined,
        status: res.status,
        durationMs: res.durationMs,
        timestamp: new Date().toISOString(),
      };
      const updated = [entry, ...history.filter(
        (h) => !(h.method === method && h.path === fullPath),
      )].slice(0, MAX_HISTORY);
      setHistory(updated);
      saveHistory(updated);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setLoading(false);
    }
  }, [method, body, history, buildFullPath]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
        e.preventDefault();
        execute();
      }
    },
    [execute],
  );

  const handleCollectionSelect = useCallback(
    (t: Table) => {
      const tableName =
        t.schema !== "public" ? `${t.schema}.${t.name}` : t.name;
      setPath(`/api/collections/${tableName}`);
    },
    [],
  );

  const handleHistorySelect = useCallback(
    (entry: ApiExplorerHistoryEntry) => {
      const [pathPart, queryPart] = entry.path.split("?");
      setMethod(entry.method as Method);
      setPath(pathPart);
      if (entry.body) setBody(entry.body);
      // Parse query params
      if (queryPart) {
        const params = new URLSearchParams(queryPart);
        setFilter(params.get("filter") || "");
        setSort(params.get("sort") || "");
        setPage(params.get("page") || "");
        setPerPage(params.get("perPage") || "");
        setFields(params.get("fields") || "");
        setExpand(params.get("expand") || "");
        setSearch(params.get("search") || "");
        setShowParams(true);
      }
      setShowHistory(false);
    },
    [],
  );

  const clearHistory = useCallback(() => {
    setHistory([]);
    saveHistory([]);
  }, []);

  const copySnippet = useCallback(
    (text: string) => {
      navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    },
    [],
  );

  // Detect expandable FKs from selected table for hints
  const [selectedTable, setSelectedTable] = useState<Table | null>(null);
  useEffect(() => {
    const match = path.match(/^\/api\/collections\/([^/?]+)/);
    if (match) {
      const name = match[1];
      const found = tables.find(
        (t) =>
          t.name === name ||
          `${t.schema}.${t.name}` === name,
      );
      setSelectedTable(found || null);
    } else {
      setSelectedTable(null);
    }
  }, [path, tables]);

  const fullPath = buildFullPath();
  const fullUrl = `${globalThis.location?.origin || "http://localhost:8090"}${fullPath}`;

  const showBodyEditor = method === "POST" || method === "PATCH";

  return (
    <div className="flex flex-col h-full" onKeyDown={handleKeyDown}>
      <div className="border-b px-6 py-4">
        <div className="flex items-center gap-3 mb-3">
          <h1 className="font-semibold text-lg">API Explorer</h1>
          <button
            onClick={() => setShowHistory(!showHistory)}
            className="ml-auto text-xs text-gray-500 hover:text-gray-700 flex items-center gap-1"
          >
            <Clock className="w-3 h-3" />
            History ({history.length})
          </button>
        </div>

        {/* Method + Path */}
        <div className="flex gap-2 items-stretch">
          <select
            aria-label="HTTP method"
            value={method}
            onChange={(e) => setMethod(e.target.value as Method)}
            className={cn(
              "px-3 py-2 rounded-lg font-mono text-sm font-bold border",
              METHOD_COLORS[method],
            )}
          >
            {METHODS.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>

          <input
            aria-label="Request path"
            type="text"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            className="flex-1 px-3 py-2 border rounded-lg font-mono text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="/api/collections/table_name"
          />

          <button
            onClick={execute}
            disabled={loading || !path.trim()}
            className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1.5"
          >
            <Play className="w-3.5 h-3.5" />
            {loading ? "Sending..." : "Send"}
          </button>
        </div>

        {/* Collection quick-select */}
        <div className="mt-2 flex flex-wrap gap-1">
          {tables.slice(0, 12).map((t) => {
            const label =
              t.schema !== "public" ? `${t.schema}.${t.name}` : t.name;
            return (
              <button
                key={`${t.schema}.${t.name}`}
                onClick={() => handleCollectionSelect(t)}
                className="px-2 py-0.5 text-xs bg-gray-100 hover:bg-gray-200 rounded text-gray-600"
              >
                {label}
              </button>
            );
          })}
          {tables.length > 12 && (
            <span className="px-2 py-0.5 text-xs text-gray-400">
              +{tables.length - 12} more
            </span>
          )}
        </div>

        {/* Query Params Toggle */}
        <button
          onClick={() => setShowParams(!showParams)}
          className="mt-2 text-xs text-gray-500 hover:text-gray-700 flex items-center gap-1"
        >
          {showParams ? (
            <ChevronDown className="w-3 h-3" />
          ) : (
            <ChevronRight className="w-3 h-3" />
          )}
          Query Parameters
        </button>

        {showParams && (
          <div className="mt-2 grid grid-cols-2 gap-2">
            <div>
              <label className="text-xs text-gray-500 block mb-0.5">
                filter
              </label>
              <input
                aria-label="filter"
                type="text"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                className="w-full px-2 py-1 text-xs border rounded font-mono"
                placeholder="status='active' AND age>21"
              />
            </div>
            <div>
              <label className="text-xs text-gray-500 block mb-0.5">sort</label>
              <input
                aria-label="sort"
                type="text"
                value={sort}
                onChange={(e) => setSort(e.target.value)}
                className="w-full px-2 py-1 text-xs border rounded font-mono"
                placeholder="-created_at,+title"
              />
            </div>
            <div>
              <label className="text-xs text-gray-500 block mb-0.5">page</label>
              <input
                aria-label="page"
                type="text"
                value={page}
                onChange={(e) => setPage(e.target.value)}
                className="w-full px-2 py-1 text-xs border rounded font-mono"
                placeholder="1"
              />
            </div>
            <div>
              <label className="text-xs text-gray-500 block mb-0.5">
                perPage
              </label>
              <input
                aria-label="perPage"
                type="text"
                value={perPage}
                onChange={(e) => setPerPage(e.target.value)}
                className="w-full px-2 py-1 text-xs border rounded font-mono"
                placeholder="20"
              />
            </div>
            <div>
              <label className="text-xs text-gray-500 block mb-0.5">
                fields
              </label>
              <input
                aria-label="fields"
                type="text"
                value={fields}
                onChange={(e) => setFields(e.target.value)}
                className="w-full px-2 py-1 text-xs border rounded font-mono"
                placeholder="id,name,email"
              />
            </div>
            <div>
              <label className="text-xs text-gray-500 block mb-0.5">
                expand
              </label>
              <input
                aria-label="expand"
                type="text"
                value={expand}
                onChange={(e) => setExpand(e.target.value)}
                className="w-full px-2 py-1 text-xs border rounded font-mono"
                placeholder={
                  selectedTable?.foreignKeys?.map((fk) => fk.referencedTable).join(",") ||
                  "author,category"
                }
              />
            </div>
            <div className="col-span-2">
              <label className="text-xs text-gray-500 block mb-0.5">
                search
              </label>
              <input
                aria-label="search"
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="w-full px-2 py-1 text-xs border rounded font-mono"
                placeholder="full text search query"
              />
            </div>
          </div>
        )}

        {/* Request Body */}
        {showBodyEditor && (
          <div className="mt-3">
            <label className="text-xs text-gray-500 block mb-0.5">
              Request Body (JSON)
            </label>
            <textarea
              aria-label="Request body"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              className="w-full h-24 px-3 py-2 font-mono text-xs border rounded-lg resize-y bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder='{"key": "value"}'
              spellCheck={false}
            />
          </div>
        )}

        <div className="mt-2 text-xs text-gray-400">
          {navigator.platform?.includes("Mac") ? "\u2318" : "Ctrl"}+Enter to
          send
        </div>
      </div>

      {/* History Panel */}
      {showHistory && history.length > 0 && (
        <div className="border-b bg-gray-50 px-6 py-3 max-h-48 overflow-y-auto">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-medium text-gray-500">
              Recent Requests
            </span>
            <button
              onClick={clearHistory}
              className="text-xs text-gray-400 hover:text-red-500 flex items-center gap-1"
            >
              <Trash2 className="w-3 h-3" />
              Clear
            </button>
          </div>
          {history.map((entry, i) => (
            <button
              key={i}
              onClick={() => handleHistorySelect(entry)}
              className="w-full text-left px-2 py-1 text-xs hover:bg-gray-100 rounded flex items-center gap-2 font-mono"
            >
              <span
                className={cn(
                  "px-1.5 py-0.5 rounded text-[10px] font-bold",
                  METHOD_COLORS[entry.method as Method] || "bg-gray-100",
                )}
              >
                {entry.method}
              </span>
              <span className="truncate flex-1">{entry.path}</span>
              <span className={cn("shrink-0", statusColor(entry.status))}>
                {entry.status}
              </span>
              <span className="text-gray-400 shrink-0">
                {entry.durationMs}ms
              </span>
            </button>
          ))}
        </div>
      )}

      {/* Response Area */}
      <div className="flex-1 overflow-auto">
        {error && (
          <div className="m-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
            <AlertCircle className="w-4 h-4 text-red-500 mt-0.5 shrink-0" />
            <pre className="text-sm text-red-700 whitespace-pre-wrap font-mono">
              {error}
            </pre>
          </div>
        )}

        {response && (
          <div className="p-4">
            {/* Status bar */}
            <div className="flex items-center gap-3 mb-3">
              <span
                className={cn(
                  "text-sm font-bold",
                  statusColor(response.status),
                )}
              >
                {response.status} {response.statusText}
              </span>
              <span className="text-xs text-gray-500 flex items-center gap-1">
                <Clock className="w-3 h-3" />
                {response.durationMs}ms
              </span>
              <span className="text-xs text-gray-400">
                {new TextEncoder().encode(response.body).length} bytes
              </span>
            </div>

            {/* Code Snippets */}
            <div className="mb-3 border rounded-lg overflow-hidden">
              <div className="flex bg-gray-50 border-b">
                <button
                  onClick={() => setSnippetTab("curl")}
                  className={cn(
                    "px-3 py-1.5 text-xs font-medium",
                    snippetTab === "curl"
                      ? "bg-white border-b-2 border-blue-500 text-blue-600"
                      : "text-gray-500 hover:text-gray-700",
                  )}
                >
                  cURL
                </button>
                <button
                  onClick={() => setSnippetTab("js")}
                  className={cn(
                    "px-3 py-1.5 text-xs font-medium",
                    snippetTab === "js"
                      ? "bg-white border-b-2 border-blue-500 text-blue-600"
                      : "text-gray-500 hover:text-gray-700",
                  )}
                >
                  JS SDK
                </button>
                <button
                  onClick={() =>
                    copySnippet(
                      snippetTab === "curl"
                        ? generateCurl(method, fullUrl, body || undefined)
                        : generateJsSdk(method, fullPath, body || undefined),
                    )
                  }
                  className="ml-auto px-3 py-1.5 text-xs text-gray-400 hover:text-gray-600 flex items-center gap-1"
                >
                  {copied ? (
                    <>
                      <Check className="w-3 h-3" /> Copied
                    </>
                  ) : (
                    <>
                      <Copy className="w-3 h-3" /> Copy
                    </>
                  )}
                </button>
              </div>
              <pre className="p-3 text-xs font-mono bg-gray-900 text-gray-100 overflow-x-auto max-h-32">
                {snippetTab === "curl"
                  ? generateCurl(method, fullUrl, body || undefined)
                  : generateJsSdk(method, fullPath, body || undefined)}
              </pre>
            </div>

            {/* Response Body */}
            <div className="border rounded-lg overflow-hidden">
              <div className="px-3 py-1.5 bg-gray-50 border-b text-xs font-medium text-gray-500">
                Response Body
              </div>
              <pre className="p-3 text-xs font-mono overflow-x-auto max-h-96 bg-white whitespace-pre-wrap">
                {formatJson(response.body)}
              </pre>
            </div>
          </div>
        )}

        {!response && !error && (
          <div className="flex-1 flex items-center justify-center text-gray-400 text-sm h-48">
            Send a request to see the response
          </div>
        )}
      </div>
    </div>
  );
}
