import React, { Suspense } from "react";
import { render, renderHook, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AYBProvider, useQuery } from "../src";
import type { AYBClientLike } from "../src/types";

function createClient() {
  const list = vi.fn(async () => ({
    items: [{ id: "p1" }],
    page: 1,
    perPage: 20,
    totalItems: 1,
    totalPages: 1,
  }));

  const client: AYBClientLike = {
    token: null,
    refreshToken: null,
    onAuthStateChange: () => () => {},
    auth: {
      login: async () => ({}),
      register: async () => ({}),
      logout: async () => {},
      refresh: async () => ({}),
      me: async () => ({ id: "u1", email: "u@example.com" }),
    },
    records: { list },
    realtime: { subscribe: () => () => {} },
  };

  return { client, list };
}

describe("useQuery", () => {
  it("handles loading and success", async () => {
    const { client } = createClient();
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AYBProvider client={client}>{children}</AYBProvider>
    );

    const { result } = renderHook(() => useQuery<{ id: string }>("posts"), { wrapper });

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
      expect(result.current.data?.items[0]?.id).toBe("p1");
      expect(result.current.error).toBeNull();
    });
  });

  it("handles error", async () => {
    const { client, list } = createClient();
    list.mockRejectedValueOnce(new Error("boom"));

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AYBProvider client={client}>{children}</AYBProvider>
    );

    const { result } = renderHook(() => useQuery<{ id: string }>("posts"), { wrapper });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
      expect(result.current.error?.message).toBe("boom");
    });
  });

  it("supports suspense mode", async () => {
    const { client, list } = createClient();

    function Demo() {
      const { data } = useQuery<{ id: string }>("posts", undefined, { suspense: true });
      return <div>count:{data?.items.length ?? 0}</div>;
    }

    render(
      <AYBProvider client={client}>
        <Suspense fallback={<div>loading...</div>}>
          <Demo />
        </Suspense>
      </AYBProvider>,
    );

    expect(screen.getByText("loading...")).toBeTruthy();
    await waitFor(() => {
      expect(list).toHaveBeenCalledTimes(1);
    });
  });
});
