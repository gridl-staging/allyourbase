import React from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AYBProvider, useAuth, useQuery } from "../src";
import type { AYBClientLike } from "../src/types";
import { AYBClient } from "../../sdk/src/client";
import type { AuthResponse, ListResponse } from "../../sdk/src/types";
import { mockFetchSequence } from "../../sdk/src/test_utils/mockFetchSequence";

describe("react contract parity", () => {
  it("useAuth consumes canonical auth fixture shape parsed by core SDK", async () => {
    const fetchFn = mockFetchSequence([
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
      {
        status: 200,
        body: {
          id: "usr_1",
          email: "dev@allyourbase.io",
          emailVerified: true,
          createdAt: "2026-01-01T00:00:00Z",
          updatedAt: null,
        },
      },
    ]);

    const core = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const auth: AuthResponse = await core.auth.login("dev@allyourbase.io", "secret");
    expect(auth.user.emailVerified).toBe(true);

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AYBProvider client={core as unknown as AYBClientLike}>{children}</AYBProvider>
    );

    const { result } = renderHook(() => useAuth(), { wrapper });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
      expect(result.current.user?.id).toBe("usr_1");
      expect(result.current.user?.email).toBe("dev@allyourbase.io");
    });
  });

  it("useQuery consumes canonical list fixture shape parsed by core SDK", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          items: [
            { id: "rec_1", title: "First" },
            { id: "rec_2", title: "Second" },
          ],
          page: 1,
          perPage: 2,
          totalItems: 2,
          totalPages: 1,
        },
      },
      {
        status: 200,
        body: {
          items: [
            { id: "rec_1", title: "First" },
            { id: "rec_2", title: "Second" },
          ],
          page: 1,
          perPage: 2,
          totalItems: 2,
          totalPages: 1,
        },
      },
    ]);

    const core = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const list: ListResponse<Record<string, unknown>> = await core.records.list("posts");
    expect(list.totalItems).toBe(2);

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AYBProvider client={core as unknown as AYBClientLike}>{children}</AYBProvider>
    );

    const { result } = renderHook(() => useQuery<Record<string, unknown>>("posts"), { wrapper });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
      expect(result.current.data?.items[0]?.title).toBe("First");
      expect(result.current.data?.items[1]?.title).toBe("Second");
    });
  });
});
