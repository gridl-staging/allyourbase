import { describe, expect, it, vi } from "vitest";
import type { APIRequestContext, Page } from "@playwright/test";
import { listRecords, startSSECapture } from "../../browser-tests-unmocked/fixtures";

function okResponse(body: unknown) {
  return {
    ok: () => true,
    status: () => 200,
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

function sseResponse(...chunks: string[]) {
  return {
    body: new ReadableStream<Uint8Array>({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(new TextEncoder().encode(chunk));
        }
        controller.close();
      },
    }),
    ok: true,
    status: 200,
  };
}

describe("browser-unmocked realtime fixture helpers", () => {
  it("listRecords returns only the items array from list envelope", async () => {
    const request = {
      get: vi.fn(async () => okResponse({
        page: 1,
        perPage: 20,
        totalItems: 1,
        totalPages: 1,
        items: [{ id: "row-1", owner: "user-a" }],
      })),
    } as unknown as APIRequestContext;

    const items = await listRecords(request, "jwt-token", "notes");

    expect(request.get).toHaveBeenCalledWith("/api/collections/notes", {
      headers: { Authorization: "Bearer jwt-token" },
    });
    expect(items).toEqual([{ id: "row-1", owner: "user-a" }]);
  });

  it("startSSECapture uses authorization header and exposes getEvents/close", async () => {
    const fetchMock = vi.fn(async () =>
      sseResponse(
        "event: connected\n\n",
        'data: {"action":"create","table":"notes","record":{"id":"row-1"}}\n\n',
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    try {
      const page = {} as Page;

      const capture = await startSSECapture(page, "http://localhost:8090", "jwt-token", ["notes"]);
      const events = await capture.getEvents();
      await capture.close();

      expect(events).toEqual([{ action: "create", table: "notes", record: { id: "row-1" } }]);
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect((fetchMock.mock.calls[0] ?? [])[0]?.toString()).toBe(
        "http://localhost:8090/api/realtime?tables=notes",
      );
      expect(fetchMock).toHaveBeenCalledWith(expect.any(URL), {
        headers: {
          Accept: "text/event-stream",
          Authorization: "Bearer jwt-token",
        },
        signal: expect.any(AbortSignal),
      });
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
