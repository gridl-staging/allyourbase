import { describe, expect, it } from "vitest";
import { loadServerSession } from "../src";
import type { SSRClientLike } from "../src";
import { AYBClient } from "../../sdk/src/client";
import { mockFetchSequence } from "../../sdk/src/test_utils/mockFetchSequence";

describe("ssr contract parity", () => {
  it("loadServerSession consumes canonical auth response via core SDK refresh", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 401,
        body: { message: "unauthorized" },
      },
      {
        status: 200,
        body: {
          token: "jwt_stage3",
          refreshToken: "refresh_stage3",
          user: {
            id: "usr_1",
            email: "dev@allyourbase.io",
            email_verified: true,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: null,
          },
        },
      },
    ]);

    const core = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const ssrClient: SSRClientLike = {
      setTokens: (token, refreshToken) => core.setTokens(token, refreshToken),
      clearTokens: () => core.clearTokens(),
      auth: {
        me: async () => core.auth.me() as Promise<Record<string, unknown>>,
        refresh: async () => core.auth.refresh() as Promise<{
          token: string;
          refreshToken: string;
          user?: Record<string, unknown>;
        }>,
      },
    };

    const result = await loadServerSession({
      cookieHeader: "ayb_token=old; ayb_refresh_token=oldr",
      client: ssrClient,
    });

    expect(result.session?.token).toBe("jwt_stage3");
    expect(result.session?.refreshToken).toBe("refresh_stage3");
    expect(result.session?.user.id).toBe("usr_1");
    expect(result.session?.user.email).toBe("dev@allyourbase.io");
    expect(result.session?.user.emailVerified).toBe(true);
    expect(result.session?.user.createdAt).toBe("2026-01-01T00:00:00Z");
    expect(result.session?.user.updatedAt).toBeUndefined();
  });
});
