import { useState } from "react";
import { Loader2, Send, X } from "lucide-react";
import { cn } from "../../lib/utils";
import { invokeEdgeFunction, listEdgeFunctionLogs } from "../../api";
import type {
  EdgeFunctionResponse,
  EdgeFunctionInvokeResponse,
  EdgeFunctionLogEntry,
} from "../../types";

interface InvokeTesterProps {
  fn: EdgeFunctionResponse;
  onLogsUpdate: (logs: EdgeFunctionLogEntry[]) => void;
  addToast: (type: "success" | "error", message: string) => void;
}

const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"] as const;
const CONTENT_TYPES = [
  { label: "JSON", value: "application/json" },
  { label: "Text", value: "text/plain" },
  { label: "Form", value: "application/x-www-form-urlencoded" },
] as const;

export function InvokeTester({ fn, onLogsUpdate, addToast }: InvokeTesterProps) {
  const [method, setMethod] = useState("GET");
  const [path, setPath] = useState("/" + fn.name);
  const [headers, setHeaders] = useState<{ key: string; value: string }[]>([]);
  const [body, setBody] = useState("");
  const [contentType, setContentType] = useState("application/json");
  const [response, setResponse] = useState<EdgeFunctionInvokeResponse | null>(null);
  const [invoking, setInvoking] = useState(false);
  const [duration, setDuration] = useState<number | null>(null);

  const addHeader = () => setHeaders([...headers, { key: "", value: "" }]);
  const removeHeader = (idx: number) => setHeaders(headers.filter((_, i) => i !== idx));
  const updateHeader = (idx: number, field: "key" | "value", val: string) => {
    setHeaders(headers.map((h, i) => (i === idx ? { ...h, [field]: val } : h)));
  };

  const handleInvoke = async () => {
    setInvoking(true);
    setResponse(null);
    setDuration(null);
    const start = performance.now();
    try {
      const headerMap: Record<string, string[]> = {};
      for (const { key, value } of headers) {
        if (key.trim()) {
          const k = key.trim();
          headerMap[k] = headerMap[k] ? [...headerMap[k], value] : [value];
        }
      }
      if (["POST", "PUT", "PATCH"].includes(method) && body) {
        headerMap["Content-Type"] = [contentType];
      }
      const resp = await invokeEdgeFunction(fn.id, {
        method,
        path: path || "/" + fn.name,
        headers: Object.keys(headerMap).length > 0 ? headerMap : undefined,
        body: body || undefined,
      });
      setDuration(Math.round(performance.now() - start));
      setResponse(resp);
      try {
        const newLogs = await listEdgeFunctionLogs(fn.id);
        onLogsUpdate(newLogs);
      } catch {
        // Log refresh is non-critical; don't confuse user with invoke failure message
      }
    } catch (e) {
      setDuration(Math.round(performance.now() - start));
      addToast("error", e instanceof Error ? e.message : "Invocation failed");
    } finally {
      setInvoking(false);
    }
  };

  const showBody = ["POST", "PUT", "PATCH"].includes(method);

  return (
    <div className="space-y-4" data-testid="invoke-tester">
      <div className="flex gap-2">
        <select
          value={method}
          onChange={(e) => setMethod(e.target.value)}
          className="px-3 py-1.5 border rounded text-sm bg-white dark:bg-gray-800"
          aria-label="HTTP Method"
          data-testid="invoke-method"
        >
          {METHODS.map((m) => (
            <option key={m} value={m}>{m}</option>
          ))}
        </select>
        <input
          type="text"
          value={path}
          onChange={(e) => setPath(e.target.value)}
          placeholder={`/${fn.name}`}
          className="flex-1 px-3 py-1.5 border rounded text-sm font-mono"
          aria-label="Request Path"
          data-testid="invoke-path"
        />
        <button
          onClick={handleInvoke}
          disabled={invoking}
          className="flex items-center gap-1.5 px-4 py-1.5 bg-blue-600 text-white rounded text-sm hover:bg-blue-700 disabled:opacity-50"
          data-testid="invoke-send"
        >
          {invoking ? (
            <Loader2 className="w-3.5 h-3.5 animate-spin" />
          ) : (
            <Send className="w-3.5 h-3.5" />
          )}
          Send
        </button>
      </div>

      {/* Headers */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <label className="text-sm font-medium text-gray-700 dark:text-gray-200">Headers</label>
          <button
            onClick={addHeader}
            className="text-xs text-blue-600 hover:text-blue-700"
            data-testid="invoke-add-header"
          >
            + Add Header
          </button>
        </div>
        {headers.map((h, idx) => (
          <div key={idx} className="flex gap-2 mb-2">
            <input
              type="text"
              placeholder="Header-Name"
              value={h.key}
              onChange={(e) => updateHeader(idx, "key", e.target.value)}
              className="flex-1 px-3 py-1.5 border rounded text-sm font-mono"
              data-testid={`invoke-header-key-${idx}`}
            />
            <input
              type="text"
              placeholder="value"
              value={h.value}
              onChange={(e) => updateHeader(idx, "value", e.target.value)}
              className="flex-1 px-3 py-1.5 border rounded text-sm font-mono"
              data-testid={`invoke-header-value-${idx}`}
            />
            <button
              onClick={() => removeHeader(idx)}
              className="p-1.5 text-gray-400 dark:text-gray-500 hover:text-red-500"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        ))}
      </div>

      {showBody && (
        <div>
          <div className="flex items-center gap-3 mb-1">
            <label className="text-sm font-medium text-gray-700 dark:text-gray-200">Request Body</label>
            <select
              value={contentType}
              onChange={(e) => setContentType(e.target.value)}
              className="text-xs px-2 py-0.5 border rounded bg-white dark:bg-gray-800 text-gray-600 dark:text-gray-300"
              aria-label="Content Type"
              data-testid="invoke-content-type"
            >
              {CONTENT_TYPES.map((ct) => (
                <option key={ct.value} value={ct.value}>{ct.label}</option>
              ))}
            </select>
          </div>
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            className="w-full px-3 py-2 border rounded text-sm font-mono h-24"
            placeholder='{"key": "value"}'
            data-testid="invoke-body"
          />
        </div>
      )}

      {response && (
        <div className="border rounded-lg p-4 bg-gray-50 dark:bg-gray-800" data-testid="invoke-response">
          <div className="flex items-center gap-2 mb-3">
            <span className="text-sm font-medium">Response</span>
            <span
              className={cn(
                "px-2 py-0.5 rounded text-xs font-mono font-medium",
                response.statusCode >= 200 && response.statusCode < 300
                  ? "bg-green-100 text-green-700"
                  : response.statusCode >= 400
                    ? "bg-red-100 text-red-700"
                    : "bg-yellow-100 text-yellow-700",
              )}
              data-testid="invoke-status-code"
            >
              {response.statusCode}
            </span>
            {duration !== null && (
              <span className="text-xs text-gray-500 dark:text-gray-400" data-testid="invoke-duration">
                {duration}ms
              </span>
            )}
          </div>
          {response.headers && Object.keys(response.headers).length > 0 && (
            <div className="mb-3" data-testid="invoke-response-headers">
              <span className="text-xs font-medium text-gray-500 dark:text-gray-400">Headers</span>
              <div className="text-xs font-mono bg-white dark:bg-gray-800 border rounded p-2 mt-1">
                {Object.entries(response.headers).map(([k, v]) => (
                  <div key={k}>
                    <span className="text-gray-600 dark:text-gray-300">{k}:</span> {v.join(", ")}
                  </div>
                ))}
              </div>
            </div>
          )}
          {response.body && (
            <pre className="text-xs font-mono bg-white dark:bg-gray-800 border rounded p-3 overflow-auto max-h-64 whitespace-pre-wrap" data-testid="invoke-response-body">
              {response.body}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}
