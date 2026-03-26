import { describe, it, expect, vi } from "vitest";
import type { APIRequestContext, Page } from "@playwright/test";
import { promoteSessionToAAL2WithTOTP, waitForFunctionLog } from "../../browser-tests-unmocked/fixtures";

function okResponse(body: unknown) {
  return {
    ok: () => true,
    status: () => 200,
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

function errResponse(status: number, message: string) {
  return {
    ok: () => false,
    status: () => status,
    json: async () => ({ message }),
    text: async () => message,
  };
}

describe("browser-unmocked edge trigger fixture helpers", () => {
  it("waitForFunctionLog returns the matching trigger log entry", async () => {
    const logs = [
      { status: "success", requestPath: "/cron", triggerType: "http" },
      { status: "success", requestPath: "/db-event", triggerType: "db", triggerId: "t-1" },
    ];

    const request = {
      get: vi.fn(async () => okResponse(logs)),
    } as unknown as APIRequestContext;

    const matched = await waitForFunctionLog(
      request,
      "admin-token",
      "fn-123",
      (log) => log.status === "success" && log.requestPath === "/db-event" && log.triggerType === "db",
      1000,
    );

    expect(request.get).toHaveBeenCalledWith("/api/admin/functions/fn-123/logs", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(matched).toMatchObject({
      status: "success",
      requestPath: "/db-event",
      triggerType: "db",
      triggerId: "t-1",
    });
  });

  it("waitForFunctionLog errors immediately when timeout is zero", async () => {
    const request = {
      get: vi.fn(async () => okResponse([])),
    } as unknown as APIRequestContext;

    await expect(waitForFunctionLog(request, "admin-token", "fn-404", () => false, 0)).rejects.toThrow(
      "No matching log entry found for function fn-404 within 0ms",
    );
    expect(request.get).not.toHaveBeenCalled();
  });

  it("waitForFunctionLog fails fast on non-OK log API responses", async () => {
    const request = {
      get: vi.fn(async () => ({
        ok: () => false,
        status: () => 401,
        text: async () => "unauthorized",
      })),
    } as unknown as APIRequestContext;

    await expect(
      waitForFunctionLog(request, "admin-token", "fn-401", () => false, 5000),
    ).rejects.toThrow("Failed to fetch function logs for fn-401: status 401: unauthorized");
    expect(request.get).toHaveBeenCalledTimes(1);
  });

  it("waitForFunctionLog ignores stale matching logs when minCreatedAt is set", async () => {
    const beforeAction = Date.now();
    const staleLog = {
      status: "success",
      requestPath: "/db-event",
      triggerType: "db",
      triggerId: "stale-trigger",
      createdAt: new Date(beforeAction - 1000).toISOString(),
    };
    const freshLog = {
      status: "success",
      requestPath: "/db-event",
      triggerType: "db",
      triggerId: "fresh-trigger",
      createdAt: new Date(beforeAction + 1000).toISOString(),
    };

    const request = {
      get: vi
        .fn()
        .mockImplementationOnce(async () => okResponse([staleLog]))
        .mockImplementationOnce(async () => okResponse([staleLog, freshLog])),
    } as unknown as APIRequestContext;

    const matched = await waitForFunctionLog(
      request,
      "admin-token",
      "fn-stale",
      (log) => log.status === "success" && log.requestPath === "/db-event" && log.triggerType === "db",
      { timeoutMs: 2000, pollIntervalMs: 1, minCreatedAt: beforeAction },
    );

    expect(matched).toMatchObject({
      triggerId: "fresh-trigger",
      requestPath: "/db-event",
      triggerType: "db",
    });
    expect(request.get).toHaveBeenCalledTimes(2);
  });

  it("promoteSessionToAAL2WithTOTP stores upgraded token in localStorage", async () => {
    const request = {
      post: vi
        .fn()
        .mockImplementationOnce(async () => okResponse({ mfa_pending: true, mfa_token: "pending-token" }))
        .mockImplementationOnce(async () => okResponse({ challenge_id: "challenge-1" }))
        .mockImplementationOnce(async () => okResponse({
          token: "aal2-token",
          refreshToken: "refresh-1",
          user: { id: "u1", email: "mfa@example.com" },
        })),
    } as unknown as APIRequestContext;
    const page = {
      evaluate: vi.fn(async () => {}),
    } as unknown as Page;

    await promoteSessionToAAL2WithTOTP(
      request,
      page,
      "mfa@example.com",
      "password123",
      "JBSWY3DPEHPK3PXP",
    );

    expect(request.post).toHaveBeenNthCalledWith(1, "/api/auth/login", {
      data: { email: "mfa@example.com", password: "password123" },
    });
    expect(request.post).toHaveBeenNthCalledWith(2, "/api/auth/mfa/totp/challenge", {
      headers: { Authorization: "Bearer pending-token" },
    });
    expect(request.post).toHaveBeenNthCalledWith(3, "/api/auth/mfa/totp/verify", {
      headers: { Authorization: "Bearer pending-token" },
      data: expect.objectContaining({
        challenge_id: "challenge-1",
      }),
    });
    expect(page.evaluate).toHaveBeenCalledWith(expect.any(Function), "aal2-token");
  });

  it("promoteSessionToAAL2WithTOTP retries once on 401 verify and then succeeds", async () => {
    const request = {
      post: vi
        .fn()
        .mockImplementationOnce(async () => okResponse({ mfa_pending: true, mfa_token: "pending-token" }))
        .mockImplementationOnce(async () => okResponse({ challenge_id: "challenge-1" }))
        .mockImplementationOnce(async () => errResponse(401, "invalid TOTP code"))
        .mockImplementationOnce(async () => okResponse({ challenge_id: "challenge-2" }))
        .mockImplementationOnce(async () => okResponse({
          token: "aal2-token-retry",
          refreshToken: "refresh-2",
          user: { id: "u1", email: "mfa@example.com" },
        })),
    } as unknown as APIRequestContext;
    const page = {
      evaluate: vi.fn(async () => {}),
    } as unknown as Page;

    await promoteSessionToAAL2WithTOTP(
      request,
      page,
      "mfa@example.com",
      "password123",
      "JBSWY3DPEHPK3PXP",
    );

    expect(request.post).toHaveBeenCalledTimes(5);
    expect(page.evaluate).toHaveBeenCalledWith(expect.any(Function), "aal2-token-retry");
  });
});
