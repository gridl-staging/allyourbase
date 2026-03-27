/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar26_pm_1_managed_pg_release_and_staging_promotion/allyourbase_dev/ui/browser-tests-unmocked/fixtures/realtime.ts.
 */
import type { Page } from "@playwright/test";

const REALTIME_WS_PATH = "/api/realtime/ws";

export interface SSECaptureHandle {
  getEvents: () => Promise<Array<Record<string, unknown>>>;
  close: () => Promise<void>;
}

interface SSEFrame {
  data: string;
  event: string;
}

function emitSSEFrame(
  pendingEvent: { dataLines: string[]; event: string | null },
  onFrame: (frame: SSEFrame) => void,
): void {
  if (!pendingEvent.event && pendingEvent.dataLines.length === 0) {
    return;
  }
  onFrame({
    data: pendingEvent.dataLines.join("\n"),
    event: pendingEvent.event ?? "message",
  });
  pendingEvent.event = null;
  pendingEvent.dataLines = [];
}

/**
 * TODO: Document consumeSSEStream.
 */
async function consumeSSEStream(
  stream: ReadableStream<Uint8Array>,
  onFrame: (frame: SSEFrame) => void,
): Promise<void> {
  const decoder = new TextDecoder();
  const reader = stream.getReader();
  const pendingEvent = { dataLines: [] as string[], event: null as string | null };
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      buffer += decoder.decode(value ?? new Uint8Array(), { stream: !done });
      if (done && buffer.length > 0 && !buffer.endsWith("\n")) {
        buffer += "\n";
      }

      let newlineIndex = buffer.indexOf("\n");
      while (newlineIndex !== -1) {
        let line = buffer.slice(0, newlineIndex);
        buffer = buffer.slice(newlineIndex + 1);
        if (line.endsWith("\r")) {
          line = line.slice(0, -1);
        }

        if (line.length === 0) {
          emitSSEFrame(pendingEvent, onFrame);
          newlineIndex = buffer.indexOf("\n");
          continue;
        }

        if (!line.startsWith(":")) {
          const separatorIndex = line.indexOf(":");
          const field = separatorIndex === -1 ? line : line.slice(0, separatorIndex);
          let valueText = separatorIndex === -1 ? "" : line.slice(separatorIndex + 1);
          if (valueText.startsWith(" ")) {
            valueText = valueText.slice(1);
          }
          if (field === "event") {
            pendingEvent.event = valueText;
          } else if (field === "data") {
            pendingEvent.dataLines.push(valueText);
          }
        }
        newlineIndex = buffer.indexOf("\n");
      }

      if (done) {
        emitSSEFrame(pendingEvent, onFrame);
        return;
      }
    }
  } finally {
    reader.releaseLock();
  }
}

/**
 * TODO: Document startSSECapture.
 */
export async function startSSECapture(
  page: Page,
  baseURL: string,
  token: string,
  tables: string[],
): Promise<SSECaptureHandle> {
  void page;

  const endpoint = new URL("/api/realtime", baseURL);
  endpoint.searchParams.set("tables", tables.join(","));

  const abortController = new AbortController();
  const events: Array<Record<string, unknown>> = [];

  let connected = false;
  let resolveConnected: (() => void) | null = null;
  let rejectConnected: ((error: Error) => void) | null = null;
  const connectedPromise = new Promise<void>((resolve, reject) => {
    resolveConnected = resolve;
    rejectConnected = reject;
  });
  const timeoutHandle = setTimeout(() => {
    if (connected) {
      return;
    }
    abortController.abort();
    rejectConnected?.(new Error("Timed out waiting for SSE connected event"));
  }, 10_000);

  const streamTask = (async () => {
    const response = await fetch(endpoint, {
      headers: {
        Accept: "text/event-stream",
        Authorization: `Bearer ${token}`,
      },
      signal: abortController.signal,
    });
    if (!response.ok) {
      throw new Error(`Realtime SSE request failed with status ${response.status}`);
    }
    if (!response.body) {
      throw new Error("Realtime SSE response body was empty");
    }

    await consumeSSEStream(response.body, ({ event, data }) => {
      if (event === "connected") {
        if (!connected) {
          connected = true;
          clearTimeout(timeoutHandle);
          resolveConnected?.();
        }
        return;
      }

      if (event !== "message" || data.length === 0) {
        return;
      }

      try {
        events.push(JSON.parse(data) as Record<string, unknown>);
      } catch {
        // Ignore malformed data events; tests consume successfully parsed records only.
      }
    });

    if (!connected) {
      throw new Error("Realtime SSE stream closed before connected event");
    }
  })().catch((error: unknown) => {
    clearTimeout(timeoutHandle);
    if (!connected && !abortController.signal.aborted) {
      rejectConnected?.(error instanceof Error ? error : new Error(String(error)));
    }
    if (abortController.signal.aborted) {
      return;
    }
    throw error;
  });

  await connectedPromise;

  return {
    getEvents: async () => events.slice(),
    close: async () => {
      clearTimeout(timeoutHandle);
      abortController.abort();
      await streamTask;
    },
  };
}

function buildRealtimeWsUrl(currentPageUrl: string, token: string): string {
  const currentURL = new URL(currentPageUrl);
  const wsProtocol = currentURL.protocol === "https:" ? "wss:" : "ws:";
  const wsURL = new URL(REALTIME_WS_PATH, `${wsProtocol}//${currentURL.host}`);
  wsURL.searchParams.set("token", token);
  return wsURL.toString();
}

/**
 * TODO: Document openRealtimeWsSubscription.
 */
async function openRealtimeWsSubscription(
  page: Page,
  currentPageUrl: string,
  token: string,
  table: string,
): Promise<string> {
  const handle = `__aybRealtimeSmokeWs${Date.now()}${Math.random().toString(36).slice(2)}`;
  const wsURL = buildRealtimeWsUrl(currentPageUrl, token);
  await page.evaluate(
    async ({ wsURL: evaluateWsUrl, table: evaluateTable, handle: evaluateHandle }) => {
      const registry = globalThis as typeof globalThis & Record<string, WebSocket | undefined>;
      await new Promise<void>((resolve, reject) => {
        const ws = new WebSocket(evaluateWsUrl);
        const timeout = setTimeout(() => {
          cleanup();
          reject(new Error("Timed out waiting for WebSocket to open"));
        }, 5000);
        const onOpen = () => {
          try {
            ws.send(
              JSON.stringify({ type: "subscribe", ref: "inspect-users", tables: [evaluateTable] }),
            );
            registry[evaluateHandle] = ws;
            cleanup();
            resolve();
          } catch (error) {
            cleanup();
            reject(error);
          }
        };
        const onError = () => {
          cleanup();
          reject(new Error("WebSocket failed to open"));
        };
        const onClose = () => {
          cleanup();
          reject(new Error("WebSocket closed before opening"));
        };
        const cleanup = () => {
          clearTimeout(timeout);
          ws.removeEventListener("open", onOpen);
          ws.removeEventListener("error", onError);
          ws.removeEventListener("close", onClose);
        };
        ws.addEventListener("open", onOpen);
        ws.addEventListener("error", onError);
        ws.addEventListener("close", onClose);
      });
    },
    { wsURL, table, handle },
  );
  return handle;
}

/**
 * TODO: Document closeRealtimeWsSubscription.
 */
async function closeRealtimeWsSubscription(page: Page, handle: string): Promise<void> {
  await page.evaluate(async (evaluateHandle) => {
    const registry = globalThis as typeof globalThis & Record<string, WebSocket | undefined>;
    const ws = registry[evaluateHandle];
    if (!ws || ws.readyState === ws.CLOSED) {
      delete registry[evaluateHandle];
      return;
    }

    await new Promise<void>((resolve, reject) => {
      const timeout = setTimeout(() => {
        cleanup();
        reject(new Error("Timed out waiting for WebSocket to close"));
      }, 5000);
      const onClose = () => {
        cleanup();
        resolve();
      };
      const onError = () => {
        cleanup();
        reject(new Error("WebSocket failed while closing"));
      };
      const cleanup = () => {
        clearTimeout(timeout);
        ws.removeEventListener("close", onClose);
        ws.removeEventListener("error", onError);
        delete registry[evaluateHandle];
      };
      ws.addEventListener("close", onClose);
      ws.addEventListener("error", onError);
      if (ws.readyState !== ws.CLOSING) {
        ws.close();
      }
      if (ws.readyState === ws.CLOSED) {
        cleanup();
        resolve();
      }
    });
  }, handle);
}

/**
 * TODO: Document withRealtimeWsSubscription.
 */
export async function withRealtimeWsSubscription<T>(
  page: Page,
  currentPageUrl: string,
  token: string,
  table: string,
  run: () => Promise<T>,
): Promise<T> {
  const wsHandle = await openRealtimeWsSubscription(page, currentPageUrl, token, table);
  let runSucceeded = false;
  try {
    const result = await run();
    runSucceeded = true;
    return result;
  } finally {
    if (runSucceeded) {
      // Run body passed — await cleanup so a cleanup-only failure surfaces clearly.
      await closeRealtimeWsSubscription(page, wsHandle);
    } else {
      // Run body already failed — close fire-and-forget so the primary error
      // propagates immediately without risking a cleanup timeout that masks it.
      closeRealtimeWsSubscription(page, wsHandle).catch(() => {});
    }
  }
}

// Fixture helper: create an API key for a user via the admin API.
// Extracted from spec files to comply with eslint no-restricted-syntax rule
// that bans request.* calls in spec files.
/**
 * TODO: Document createApiKeyForUser.
 */
export async function createApiKeyForUser(
  request: import("@playwright/test").APIRequestContext,
  adminToken: string,
  userId: string,
  keyName: string,
  scope: string = "*",
): Promise<{ key: string }> {
  const res = await request.post("/api/admin/api-keys", {
    headers: {
      Authorization: `Bearer ${adminToken}`,
      "Content-Type": "application/json",
    },
    data: {
      userId,
      name: keyName,
      scope,
    },
  });
  if (!res.ok()) {
    throw new Error(`Failed to create API key ${keyName}: ${res.status()} ${res.statusText()}`);
  }
  const body = (await res.json()) as { key?: unknown };
  if (typeof body.key !== "string" || body.key.length === 0) {
    throw new Error(`Expected API key plaintext in response for ${keyName}`);
  }
  return { key: body.key as string };
}
