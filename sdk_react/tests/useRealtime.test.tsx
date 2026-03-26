import React from "react";
import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AYBProvider, useRealtime } from "../src";
import type { AYBClientLike } from "../src/types";

describe("useRealtime", () => {
  it("subscribes and unsubscribes on lifecycle changes", () => {
    const unsub = vi.fn();
    const subscribe = vi.fn(() => unsub);

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
      records: {
        list: async () => ({ items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 }),
      },
      realtime: { subscribe },
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AYBProvider client={client}>{children}</AYBProvider>
    );

    const cb1 = vi.fn();
    const { rerender, unmount } = renderHook(
      ({ cb, tables }) => {
        useRealtime(tables, cb);
      },
      { wrapper, initialProps: { cb: cb1, tables: ["posts"] } },
    );

    expect(subscribe).toHaveBeenCalledTimes(1);

    const cb2 = vi.fn();
    rerender({ cb: cb2, tables: ["posts", "comments"] });

    expect(unsub).toHaveBeenCalledTimes(1);
    expect(subscribe).toHaveBeenCalledTimes(2);

    unmount();
    expect(unsub).toHaveBeenCalledTimes(2);
  });
});
