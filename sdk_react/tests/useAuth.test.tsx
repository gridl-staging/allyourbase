import React from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AYBProvider, useAuth } from "../src";
import type { AYBClientLike, AuthStateListener } from "../src/types";

function createAuthClient() {
  let listener: AuthStateListener | null = null;
  const unsub = vi.fn();

  const client: AYBClientLike = {
    token: "t1",
    refreshToken: "r1",
    onAuthStateChange: (cb) => {
      listener = cb;
      return unsub;
    },
    auth: {
      login: vi.fn(async () => ({})),
      register: vi.fn(async () => ({})),
      logout: vi.fn(async () => {}),
      refresh: vi.fn(async () => ({})),
      me: vi.fn(async () => ({ id: "u1", email: "u@example.com" })),
    },
    records: {
      list: vi.fn(async () => ({ items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 })),
    },
    realtime: { subscribe: vi.fn(() => () => {}) },
  };

  return { client, emit: (event: Parameters<AuthStateListener>[0], session: Parameters<AuthStateListener>[1]) => listener?.(event, session), unsub };
}

describe("useAuth", () => {
  it("loads current user, reacts to auth events, unsubscribes on unmount", async () => {
    const { client, emit, unsub } = createAuthClient();
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <AYBProvider client={client}>{children}</AYBProvider>
    );

    const { result, unmount } = renderHook(() => useAuth(), { wrapper });

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
      expect(result.current.user?.id).toBe("u1");
    });

    await act(async () => {
      emit("SIGNED_OUT", null);
    });

    await waitFor(() => {
      expect(result.current.user).toBeNull();
    });

    unmount();
    expect(unsub).toHaveBeenCalledTimes(1);
  });
});
