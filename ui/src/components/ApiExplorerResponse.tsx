import { Clock, Copy, Check, AlertCircle } from "lucide-react";
import { cn } from "../lib/utils";
import type { ApiExplorerResponse as ApiExplorerResponseData } from "../types";
import {
  formatJson,
  generateCurl,
  generateJsSdk,
  statusColor,
  type Method,
} from "./api-explorer-helpers";

interface ApiExplorerResponseProps {
  response: ApiExplorerResponseData | null;
  error: string | null;
  method: Method;
  body: string;
  fullPath: string;
  fullUrl: string;
  snippetTab: "curl" | "js";
  copied: boolean;
  onSnippetTabChange: (tab: "curl" | "js") => void;
  onCopySnippet: (text: string) => void;
}

export function ApiExplorerResponse({
  response,
  error,
  method,
  body,
  fullPath,
  fullUrl,
  snippetTab,
  copied,
  onSnippetTabChange,
  onCopySnippet,
}: ApiExplorerResponseProps) {
  return (
    <div className="flex-1 overflow-auto">
      {error && (
        <div className="m-4 p-3 bg-red-50 border border-red-200 rounded-lg flex items-start gap-2">
          <AlertCircle className="w-4 h-4 text-red-500 mt-0.5 shrink-0" />
          <pre className="text-sm text-red-700 whitespace-pre-wrap font-mono">{error}</pre>
        </div>
      )}

      {response && (
        <div className="p-4">
          <div className="flex items-center gap-3 mb-3">
            <span className={cn("text-sm font-bold", statusColor(response.status))}>
              {response.status} {response.statusText}
            </span>
            <span className="text-xs text-gray-500 dark:text-gray-400 flex items-center gap-1">
              <Clock className="w-3 h-3" />
              {response.durationMs}ms
            </span>
            <span className="text-xs text-gray-400 dark:text-gray-500">
              {new TextEncoder().encode(response.body).length} bytes
            </span>
          </div>

          <div className="mb-3 border rounded-lg overflow-hidden">
            <div className="flex bg-gray-50 dark:bg-gray-800 border-b">
              <button
                onClick={() => onSnippetTabChange("curl")}
                className={cn(
                  "px-3 py-1.5 text-xs font-medium",
                  snippetTab === "curl"
                    ? "bg-white dark:bg-gray-800 border-b-2 border-blue-500 text-blue-600"
                    : "text-gray-500 dark:text-gray-200 hover:text-gray-700 dark:hover:text-gray-200",
                )}
              >
                cURL
              </button>
              <button
                onClick={() => onSnippetTabChange("js")}
                className={cn(
                  "px-3 py-1.5 text-xs font-medium",
                  snippetTab === "js"
                    ? "bg-white dark:bg-gray-800 border-b-2 border-blue-500 text-blue-600"
                    : "text-gray-500 dark:text-gray-200 hover:text-gray-700 dark:hover:text-gray-200",
                )}
              >
                JS SDK
              </button>
              <button
                onClick={() =>
                  onCopySnippet(
                    snippetTab === "curl"
                      ? generateCurl(method, fullUrl, body || undefined)
                      : generateJsSdk(method, fullPath, body || undefined),
                  )
                }
                className="ml-auto px-3 py-1.5 text-xs text-gray-400 dark:text-gray-300 hover:text-gray-600 dark:hover:text-gray-300 flex items-center gap-1"
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

          <div className="border rounded-lg overflow-hidden">
            <div className="px-3 py-1.5 bg-gray-50 dark:bg-gray-800 border-b text-xs font-medium text-gray-500 dark:text-gray-400">
              Response Body
            </div>
            <pre className="p-3 text-xs font-mono overflow-x-auto max-h-96 bg-white dark:bg-gray-800 whitespace-pre-wrap">
              {formatJson(response.body)}
            </pre>
          </div>
        </div>
      )}

      {!response && !error && (
        <div className="flex-1 flex items-center justify-center text-gray-400 dark:text-gray-500 text-sm h-48">
          Send a request to see the response
        </div>
      )}
    </div>
  );
}
