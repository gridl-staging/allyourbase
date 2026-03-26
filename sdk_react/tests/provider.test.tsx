import React from "react";
import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AYBProvider, useAYBClient } from "../src";
import type { AYBClientLike } from "../src";

function makeClient(): AYBClientLike {
  return {
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
    realtime: { subscribe: () => () => {} },
  };
}

describe("AYBProvider/useAYBClient", () => {
  it("returns client inside provider", () => {
    const client = makeClient();
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AYBProvider client={client}>{children}</AYBProvider>
    );

    const { result } = renderHook(() => useAYBClient(), { wrapper });
    expect(result.current).toBe(client);
  });

  it("throws outside provider", () => {
    expect(() => renderHook(() => useAYBClient())).toThrow(
      "useAYBClient must be used within AYBProvider",
    );
  });
});
