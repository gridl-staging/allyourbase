import type { FullConfig } from "@playwright/test";
import { getBrowserUnmockedSkipReason, runBrowserUnmockedPreflight } from "./browser-preflight";

export default async function globalSetup(_config: FullConfig): Promise<void> {
  await runBrowserUnmockedPreflight();

  const skipReason = getBrowserUnmockedSkipReason();
  if (skipReason) {
    // Keep output concise and explicit when the environment cannot launch browsers.
    console.warn(`[browser-tests-unmocked] ${skipReason}`);
  }
}
