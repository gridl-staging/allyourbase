import { useState, useCallback, useEffect } from "react";
import type { KeyboardEvent } from "react";
import { executeApiExplorer, ApiError } from "../api";
import type {
  SchemaCache,
  Table,
  ApiExplorerResponse as ApiExplorerResponseData,
  ApiExplorerHistoryEntry,
} from "../types";
import {
  loadHistory,
  saveHistory,
  type Method,
} from "./api-explorer-helpers";
import { ApiExplorerRequest } from "./ApiExplorerRequest";
import { ApiExplorerHistory } from "./ApiExplorerHistory";
import { ApiExplorerResponse } from "./ApiExplorerResponse";

interface ApiExplorerProps {
  schema: SchemaCache;
}

export function ApiExplorer({ schema }: ApiExplorerProps) {
  const tables = Object.values(schema.tables).sort((a, b) =>
    `${a.schema}.${a.name}`.localeCompare(`${b.schema}.${b.name}`),
  );

  const [method, setMethod] = useState<Method>("GET");
  const [path, setPath] = useState("/api/collections/");
  const [body, setBody] = useState("");
  const [response, setResponse] = useState<ApiExplorerResponseData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [history, setHistory] = useState<ApiExplorerHistoryEntry[]>(loadHistory);
  const [showHistory, setShowHistory] = useState(false);
  const [snippetTab, setSnippetTab] = useState<"curl" | "js">("curl");
  const [copied, setCopied] = useState(false);
  const [showParams, setShowParams] = useState(false);

  const [filter, setFilter] = useState("");
  const [sort, setSort] = useState("");
  const [page, setPage] = useState("");
  const [perPage, setPerPage] = useState("");
  const [fields, setFields] = useState("");
  const [expand, setExpand] = useState("");
  const [search, setSearch] = useState("");

  const buildFullPath = useCallback(() => {
    const params = new URLSearchParams();
    if (filter) params.set("filter", filter);
    if (sort) params.set("sort", sort);
    if (page) params.set("page", page);
    if (perPage) params.set("perPage", perPage);
    if (fields) params.set("fields", fields);
    if (expand) params.set("expand", expand);
    if (search) params.set("search", search);
    const suffix = params.toString() ? `?${params}` : "";
    return `${path}${suffix}`;
  }, [path, filter, sort, page, perPage, fields, expand, search]);

  const execute = useCallback(async () => {
    const fullPath = buildFullPath();
    if (!fullPath) return;

    setLoading(true);
    setError(null);
    setResponse(null);

    try {
      const res = await executeApiExplorer(method, fullPath, body || undefined);
      setResponse(res);

      const entry: ApiExplorerHistoryEntry = {
        method,
        path: fullPath,
        body: body || undefined,
        status: res.status,
        durationMs: res.durationMs,
        timestamp: new Date().toISOString(),
      };

      setHistory((current) => {
        const updated = [
          entry,
          ...current.filter((item) => !(item.method === method && item.path === fullPath)),
        ].slice(0, 20);
        saveHistory(updated);
        return updated;
      });
    } catch (requestError) {
      if (requestError instanceof ApiError) {
        setError(requestError.message);
      } else {
        setError(requestError instanceof Error ? requestError.message : String(requestError));
      }
    } finally {
      setLoading(false);
    }
  }, [method, body, buildFullPath]);

  const handleKeyDown = useCallback(
    (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
        event.preventDefault();
        execute();
      }
    },
    [execute],
  );

  const handleCollectionSelect = useCallback((table: Table) => {
    const tableName = table.schema !== "public" ? `${table.schema}.${table.name}` : table.name;
    setPath(`/api/collections/${tableName}`);
  }, []);

  const handleHistorySelect = useCallback((entry: ApiExplorerHistoryEntry) => {
    const [pathPart, queryPart] = entry.path.split("?");
    setMethod(entry.method as Method);
    setPath(pathPart);
    setBody(entry.body || "");

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
    } else {
      setFilter("");
      setSort("");
      setPage("");
      setPerPage("");
      setFields("");
      setExpand("");
      setSearch("");
      setShowParams(false);
    }
    setShowHistory(false);
  }, []);

  const clearHistory = useCallback(() => {
    setHistory([]);
    saveHistory([]);
  }, []);

  const copySnippet = useCallback((text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, []);

  const [selectedTable, setSelectedTable] = useState<Table | null>(null);

  useEffect(() => {
    const match = path.match(/^\/api\/collections\/([^/?]+)/);
    if (!match) {
      setSelectedTable(null);
      return;
    }

    const tableName = match[1];
    const found = tables.find(
      (table) => table.name === tableName || `${table.schema}.${table.name}` === tableName,
    );
    setSelectedTable(found || null);
  }, [path, tables]);

  const fullPath = buildFullPath();
  const fullUrl = `${globalThis.location?.origin || "http://localhost:8090"}${fullPath}`;
  const showBodyEditor = method === "POST" || method === "PATCH";

  return (
    <div className="flex flex-col h-full" onKeyDown={handleKeyDown}>
      <ApiExplorerRequest
        tables={tables}
        method={method}
        path={path}
        body={body}
        loading={loading}
        historyCount={history.length}
        showParams={showParams}
        filter={filter}
        sort={sort}
        page={page}
        perPage={perPage}
        fields={fields}
        expand={expand}
        search={search}
        showBodyEditor={showBodyEditor}
        selectedTable={selectedTable}
        onMethodChange={setMethod}
        onPathChange={setPath}
        onBodyChange={setBody}
        onExecute={execute}
        onToggleHistory={() => setShowHistory(!showHistory)}
        onToggleParams={() => setShowParams(!showParams)}
        onCollectionSelect={handleCollectionSelect}
        onFilterChange={setFilter}
        onSortChange={setSort}
        onPageChange={setPage}
        onPerPageChange={setPerPage}
        onFieldsChange={setFields}
        onExpandChange={setExpand}
        onSearchChange={setSearch}
      />

      {showHistory && (
        <ApiExplorerHistory
          history={history}
          onClear={clearHistory}
          onSelect={handleHistorySelect}
        />
      )}

      <ApiExplorerResponse
        response={response}
        error={error}
        method={method}
        body={body}
        fullPath={fullPath}
        fullUrl={fullUrl}
        snippetTab={snippetTab}
        copied={copied}
        onSnippetTabChange={setSnippetTab}
        onCopySnippet={copySnippet}
      />
    </div>
  );
}
