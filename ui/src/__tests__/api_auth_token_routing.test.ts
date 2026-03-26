import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAnonymousSession, linkEmail } from "../api";

describe("auth API token routing", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.stubGlobal("fetch", fetchMock);
  });

  it("does not send admin token to user auth endpoints", async () => {
    localStorage.setItem("ayb_admin_token", "admin-token");
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          token: "anon-token",
          refreshToken: "anon-refresh",
          user: {
            id: "anon-1",
            email: "",
            is_anonymous: true,
            createdAt: "2026-01-01T00:00:00Z",
            updatedAt: "2026-01-01T00:00:00Z",
          },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await createAnonymousSession();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][1]?.headers).toEqual({});
  });

  it("uses persisted user auth token for account-link requests", async () => {
    fetchMock
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            token: "anon-token",
            refreshToken: "anon-refresh",
            user: {
              id: "anon-1",
              email: "",
              is_anonymous: true,
              createdAt: "2026-01-01T00:00:00Z",
              updatedAt: "2026-01-01T00:00:00Z",
            },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            token: "linked-token",
            refreshToken: "linked-refresh",
            user: {
              id: "anon-1",
              email: "linked@example.com",
              is_anonymous: false,
              createdAt: "2026-01-01T00:00:00Z",
              updatedAt: "2026-01-01T00:00:00Z",
            },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
      );

    await createAnonymousSession();
    await linkEmail("linked@example.com", "password123");

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[1][0]).toBe("/api/auth/link/email");
    expect(fetchMock.mock.calls[1][1]?.headers).toEqual({
      "Content-Type": "application/json",
      Authorization: "Bearer anon-token",
    });
  });
});
