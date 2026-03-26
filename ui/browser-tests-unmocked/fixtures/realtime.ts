/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_6_test_verification_and_lint/allyourbase_dev/ui/browser-tests-unmocked/fixtures/realtime.ts.
 */
import type { Page } from "@playwright/test";

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
