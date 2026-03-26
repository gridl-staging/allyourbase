import { describe, expect, it, vi } from "vitest";
import { loadServerSession } from "../src";
import type { SSRClientLike } from "../src";

function makeClient() {
  const client: SSRClientLike = {
    setTokens: vi.fn(),
    clearTokens: vi.fn(),
    auth: {
      me: vi.fn(async () => {
        throw Object.assign(new Error("unauthorized"), { status: 401 });
      }),
      refresh: vi.fn(async () => ({
        token: "new-token",
        refreshToken: "new-refresh",
        user: { id: "u1", email: "u@example.com" },
      })),
    },
  };
  return client;
}

describe("loadServerSession", () => {
  it("refreshes expired session and rotates cookies", async () => {
    const client = makeClient();
    const result = await loadServerSession({
      cookieHeader: "ayb_token=old; ayb_refresh_token=oldr",
      client,
    });

    expect(client.auth.refresh).toHaveBeenCalledTimes(1);
    expect(result.session?.token).toBe("new-token");
    expect(result.setCookieHeaders.length).toBe(2);
  });

  it("clears cookies when refresh fails", async () => {
    const client = makeClient();
    (client.auth.refresh as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error("refresh failed"),
    );

    const result = await loadServerSession({
      cookieHeader: "ayb_token=old; ayb_refresh_token=oldr",
      client,
    });

    expect(result.session).toBeNull();
    expect(result.setCookieHeaders.length).toBe(2);
    expect(result.setCookieHeaders[0]).toContain("Max-Age=0");
  });
});
