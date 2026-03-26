/**
 * @module Test utility for creating a vitest mock fetch function that returns a sequence of predefined responses.
 */
import { vi } from "vitest";

/**
 * Creates a mock fetch function that returns responses in sequence from a queue. Each invocation shifts and returns the next response; throws if called more times than responses provided. Useful for testing code that makes multiple fetch requests with predictable responses.
 */
export function mockFetchSequence(
  responses: Array<{ status: number; body: unknown }>,
): typeof globalThis.fetch {
  const queue = [...responses];
  return vi.fn().mockImplementation(() => {
    const next = queue.shift();
    if (!next) {
      throw new Error("unexpected fetch call");
    }
    return Promise.resolve({
      ok: next.status >= 200 && next.status < 300,
      status: next.status,
      statusText:
        next.status === 401 ? "Unauthorized" : next.status === 403 ? "Forbidden" : "OK",
      headers: new Headers(),
      json: () => Promise.resolve(next.body),
    });
  }) as unknown as typeof globalThis.fetch;
}
