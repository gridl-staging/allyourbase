/**
 * @module Helper utilities for the API explorer including localStorage-based request history, JSON formatting, and code generation in curl and JavaScript SDK formats.
 */
import type { ApiExplorerHistoryEntry } from "../types";

export const METHODS = ["GET", "POST", "PATCH", "DELETE"] as const;
export type Method = (typeof METHODS)[number];

const HISTORY_KEY = "ayb_api_explorer_history";
const MAX_HISTORY = 20;

export const METHOD_COLORS: Record<Method, string> = {
  GET: "bg-green-100 text-green-700",
  POST: "bg-blue-100 text-blue-700",
  PATCH: "bg-yellow-100 text-yellow-700",
  DELETE: "bg-red-100 text-red-700",
};

export function loadHistory(): ApiExplorerHistoryEntry[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

export function saveHistory(history: ApiExplorerHistoryEntry[]) {
  localStorage.setItem(HISTORY_KEY, JSON.stringify(history.slice(0, MAX_HISTORY)));
}

export function formatJson(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

export function generateCurl(method: string, fullUrl: string, body?: string): string {
  let cmd = `curl -X ${method}`;
  cmd += ` \\\n  -H "Authorization: Bearer <TOKEN>"`;
  if (body && (method === "POST" || method === "PATCH")) {
    cmd += ` \\\n  -H "Content-Type: application/json"`;
    cmd += ` \\\n  -d '${body}'`;
  }
  cmd += ` \\\n  "${fullUrl}"`;
  return cmd;
}

/**
 * Generates JavaScript SDK code for API requests, converting known collection and RPC paths to idiomatic SDK calls or falling back to raw fetch code.
 * @param method - HTTP method
 * @param path - API endpoint path
 * @param body - Optional JSON request body as string
 * @returns JavaScript code snippet for the API call
 */
export function generateJsSdk(method: string, path: string, body?: string): string {
  const collMatch = path.match(/^\/api\/collections\/([^/?]+)(?:\/([^/?]+))?/);
  if (collMatch) {
    const table = collMatch[1];
    const id = collMatch[2];
    const qs = path.includes("?") ? path.split("?")[1] : "";
    const params = new URLSearchParams(qs);

    if (method === "GET" && !id) {
      const opts: string[] = [];
      for (const [key, value] of params) opts.push(`  ${key}: "${value}"`);
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

  const rpcMatch = path.match(/^\/api\/rpc\/([^/?]+)/);
  if (rpcMatch && method === "POST") {
    const fn = rpcMatch[1];
    const parsed = body ? JSON.parse(body) : {};
    return `const result = await ayb.rpc("${fn}", ${JSON.stringify(parsed, null, 2)});`;
  }

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

export function statusColor(status: number): string {
  if (status >= 200 && status < 300) return "text-green-600";
  if (status >= 300 && status < 400) return "text-yellow-600";
  if (status >= 400 && status < 500) return "text-orange-600";
  return "text-red-600";
}
