import { useState } from "react";
import { Loader2, Send } from "lucide-react";
import type { AssistantResponse } from "../../types/ai";
import { streamAssistant } from "../../api_ai";

const MODES = [
  { value: "sql", label: "SQL" },
  { value: "rls", label: "RLS" },
  { value: "migration", label: "Migration" },
  { value: "general", label: "General" },
] as const;

export function AIAssistantTab() {
  const [mode, setMode] = useState("sql");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [response, setResponse] = useState<AssistantResponse | null>(null);
  const [streamText, setStreamText] = useState("");
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!query.trim()) return;
    setLoading(true);
    setError(null);
    setResponse(null);
    setStreamText("");
    try {
      let streamed = "";
      const result = await streamAssistant(
        { mode, query: query.trim() },
        (chunk) => {
          streamed += chunk;
          setStreamText(streamed);
        },
      );
      if (result) {
        setResponse(result);
      } else if (streamed.trim()) {
        setResponse({ text: streamed.trim(), explanation: streamed.trim() });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Request failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <form onSubmit={handleSubmit} className="mb-4">
        <div className="flex gap-2 mb-3">
          {MODES.map((m) => (
            <button
              key={m.value}
              type="button"
              onClick={() => setMode(m.value)}
              className={`px-3 py-1.5 text-sm rounded border ${
                mode === m.value
                  ? "bg-blue-600 text-white border-blue-600"
                  : "text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
              }`}
            >
              {m.label}
            </button>
          ))}
        </div>
        <div className="flex gap-2">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Ask a question..."
            className="flex-1 border rounded px-3 py-2 text-sm"
            aria-label="Query"
          />
          <button
            type="submit"
            disabled={loading || !query.trim()}
            aria-label="Send query"
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 inline-flex items-center gap-1.5"
          >
            {loading ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Send className="w-4 h-4" />
            )}
          </button>
        </div>
      </form>

      {error && (
        <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded text-sm text-red-600">
          {error}
        </div>
      )}

      {loading && streamText && (
        <div className="border rounded-lg p-4 mb-3">
          <h4 className="text-xs font-medium text-gray-500 uppercase mb-1">Streaming</h4>
          <pre className="bg-gray-50 dark:bg-gray-800 p-3 rounded text-sm whitespace-pre-wrap">
            <code>{streamText}</code>
          </pre>
        </div>
      )}

      {response && (
        <div className="border rounded-lg p-4">
          {response.sql && (
            <div className="mb-3">
              <h4 className="text-xs font-medium text-gray-500 uppercase mb-1">SQL</h4>
              <pre className="bg-gray-50 dark:bg-gray-800 p-3 rounded text-sm overflow-x-auto">
                <code>{response.sql}</code>
              </pre>
            </div>
          )}
          <div>
            <h4 className="text-xs font-medium text-gray-500 uppercase mb-1">Explanation</h4>
            <p className="text-sm text-gray-700 dark:text-gray-300">
              {response.explanation || response.text || "No explanation provided."}
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
