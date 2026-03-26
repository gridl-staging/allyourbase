/**
 * @module Utilities for validating browser readiness before running unmocked browser tests, including error normalization and preflight validation.
 */
import { chromium, type Browser } from "@playwright/test";
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";

export const BROWSER_UNMOCKED_SKIP_REASON_ENV = "AYB_BROWSER_UNMOCKED_SKIP_REASON";
export const BROWSER_UNMOCKED_SKIP_REASON_FILE = "browser-tests-unmocked/.skip-reason";

function readPersistedSkipReason(): string | null {
  try {
    const value = readFileSync(BROWSER_UNMOCKED_SKIP_REASON_FILE, "utf8").trim();
    return value.length > 0 ? value : null;
  } catch {
    return null;
  }
}

function writePersistedSkipReason(reason: string): void {
  mkdirSync(dirname(BROWSER_UNMOCKED_SKIP_REASON_FILE), { recursive: true });
  writeFileSync(BROWSER_UNMOCKED_SKIP_REASON_FILE, `${reason}\n`, "utf8");
}

function clearPersistedSkipReason(): void {
  try {
    rmSync(BROWSER_UNMOCKED_SKIP_REASON_FILE, { force: true, recursive: true });
  } catch {
    // Best effort: preflight must not crash on skip-reason cleanup.
  }
}

function persistSkipReason(reason: string): void {
  try {
    writePersistedSkipReason(reason);
  } catch {
    // Best effort: env skip reason is enough for the current test run.
  }
}

/**
 * Extracts a meaningful string representation from any error type. Attempts Error.message extraction, string coercion, JSON serialization, and finally String() conversion as fallback. @param error - the error object to normalize. @returns string representation of the error.
 */
function normalizeErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    if (error.message && error.message.trim().length > 0) {
      return error.message.trim();
    }
  }
  if (typeof error === "string" && error.trim().length > 0) {
    return error.trim();
  }
  try {
    const json = JSON.stringify(error);
    if (json && json !== "{}") {
      return json;
    }
  } catch {
    // Ignore serialization issues and fall back to String().
  }
  return String(error);
}

export function getBrowserUnmockedSkipReason(env: NodeJS.ProcessEnv = process.env): string | null {
  const value = env[BROWSER_UNMOCKED_SKIP_REASON_ENV];
  if (typeof value !== "string") {
    return readPersistedSkipReason();
  }
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : readPersistedSkipReason();
}

export function formatBrowserLaunchFailure(error: unknown): string {
  const message = normalizeErrorMessage(error);
  const isMacSandboxBootstrapFailure =
    /bootstrap_check_in/i.test(message) && /permission denied\s*\(1100\)/i.test(message);

  if (isMacSandboxBootstrapFailure) {
    return "Playwright browser launch blocked by macOS sandbox (bootstrap_check_in Permission denied (1100)); run browser-unmocked tests outside sandbox.";
  }

  return `Browser launch preflight failed: ${message}`;
}

/**
 * Verifies browser launch capability by attempting to launch Chromium headlessly. Clears prior skip reasons and sets a persistent skip reason if launch fails, allowing tests to be conditionally skipped. @param env - optional environment object for checking and setting skip reason; defaults to process.env. @returns promise that resolves when preflight check completes.
 */
export async function runBrowserUnmockedPreflight(
  env: NodeJS.ProcessEnv = process.env,
): Promise<void> {
  const envReason = env[BROWSER_UNMOCKED_SKIP_REASON_ENV];
  if (typeof envReason === "string" && envReason.trim().length > 0) {
    return;
  }

  clearPersistedSkipReason();
  let browser: Browser | null = null;
  try {
    browser = await chromium.launch({ headless: true });
  } catch (error) {
    const reason = formatBrowserLaunchFailure(error);
    env[BROWSER_UNMOCKED_SKIP_REASON_ENV] = reason;
    persistSkipReason(reason);
  } finally {
    if (browser) {
      await browser.close().catch(() => {});
    }
  }
}
