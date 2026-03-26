import { afterEach, beforeEach, describe, it, expect, vi } from "vitest";
import { chmodSync, existsSync, mkdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { chromium, type Browser } from "@playwright/test";
import {
  BROWSER_UNMOCKED_SKIP_REASON_ENV,
  BROWSER_UNMOCKED_SKIP_REASON_FILE,
  formatBrowserLaunchFailure,
  getBrowserUnmockedSkipReason,
  runBrowserUnmockedPreflight,
} from "../../browser-tests-unmocked/browser-preflight";

describe("browser-unmocked preflight helpers", () => {
  beforeEach(() => {
    rmSync(BROWSER_UNMOCKED_SKIP_REASON_FILE, { force: true, recursive: true });
  });

  afterEach(() => {
    rmSync(BROWSER_UNMOCKED_SKIP_REASON_FILE, { force: true, recursive: true });
    vi.restoreAllMocks();
  });

  it("returns null when skip reason env var is missing", () => {
    const env: NodeJS.ProcessEnv = {};
    expect(getBrowserUnmockedSkipReason(env)).toBeNull();
  });

  it("returns trimmed skip reason when env var is present", () => {
    const env: NodeJS.ProcessEnv = {
      [BROWSER_UNMOCKED_SKIP_REASON_ENV]: "  browser unavailable  ",
    };
    expect(getBrowserUnmockedSkipReason(env)).toBe("browser unavailable");
  });

  it("falls back to persisted skip reason file when env var is missing", () => {
    mkdirSync(dirname(BROWSER_UNMOCKED_SKIP_REASON_FILE), { recursive: true });
    writeFileSync(BROWSER_UNMOCKED_SKIP_REASON_FILE, "  persisted reason  \n", "utf8");
    expect(getBrowserUnmockedSkipReason({})).toBe("persisted reason");
  });

  it("normalizes known macOS sandbox bootstrap errors", () => {
    const error = new Error(
      "browserType.launch failed: bootstrap_check_in org.chromium.Chromium.MachPortRendezvousServer.4993: Permission denied (1100)",
    );
    expect(formatBrowserLaunchFailure(error)).toContain("macOS sandbox");
    expect(formatBrowserLaunchFailure(error)).toContain("Permission denied (1100)");
  });

  it("falls back to generic message for unknown errors", () => {
    expect(formatBrowserLaunchFailure("bad launch")).toBe("Browser launch preflight failed: bad launch");
  });

  it("retries browser launch when only a persisted skip reason exists", async () => {
    mkdirSync(dirname(BROWSER_UNMOCKED_SKIP_REASON_FILE), { recursive: true });
    writeFileSync(BROWSER_UNMOCKED_SKIP_REASON_FILE, "stale reason\n", "utf8");

    const close = vi.fn<() => Promise<void>>().mockResolvedValue(undefined);
    const launchSpy = vi
      .spyOn(chromium, "launch")
      .mockResolvedValue({ close } as unknown as Browser);

    const env: NodeJS.ProcessEnv = {};
    await runBrowserUnmockedPreflight(env);

    expect(launchSpy).toHaveBeenCalledTimes(1);
    expect(close).toHaveBeenCalledTimes(1);
    expect(existsSync(BROWSER_UNMOCKED_SKIP_REASON_FILE)).toBe(false);
    expect(getBrowserUnmockedSkipReason(env)).toBeNull();
  });

  it("does not throw when stale skip reason path is a directory", async () => {
    mkdirSync(BROWSER_UNMOCKED_SKIP_REASON_FILE, { recursive: true });
    const close = vi.fn<() => Promise<void>>().mockResolvedValue(undefined);
    const launchSpy = vi
      .spyOn(chromium, "launch")
      .mockResolvedValue({ close } as unknown as Browser);

    const env: NodeJS.ProcessEnv = {};
    await expect(runBrowserUnmockedPreflight(env)).resolves.toBeUndefined();

    expect(launchSpy).toHaveBeenCalledTimes(1);
    expect(close).toHaveBeenCalledTimes(1);
    expect(existsSync(BROWSER_UNMOCKED_SKIP_REASON_FILE)).toBe(false);
    expect(getBrowserUnmockedSkipReason(env)).toBeNull();
  });

  it("does not throw when persisting skip reason fails", async () => {
    const skipReasonDir = dirname(BROWSER_UNMOCKED_SKIP_REASON_FILE);
    mkdirSync(skipReasonDir, { recursive: true });
    const originalMode = statSync(skipReasonDir).mode & 0o777;
    chmodSync(skipReasonDir, 0o555);

    const launchSpy = vi
      .spyOn(chromium, "launch")
      .mockRejectedValue(new Error("browser launch failed"));
    const env: NodeJS.ProcessEnv = {};

    try {
      await expect(runBrowserUnmockedPreflight(env)).resolves.toBeUndefined();
      expect(launchSpy).toHaveBeenCalledTimes(1);
      expect(env[BROWSER_UNMOCKED_SKIP_REASON_ENV]).toContain("browser launch failed");
      expect(existsSync(BROWSER_UNMOCKED_SKIP_REASON_FILE)).toBe(false);
    } finally {
      chmodSync(skipReasonDir, originalMode);
    }
  });
});
