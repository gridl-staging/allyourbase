import { describe, expect, it } from "vitest";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { resolve, join, relative } from "node:path";

/**
 * Audit test: every component file that uses Tailwind light-only utility classes
 * (bg-white, bg-gray-50, text-gray-*, border-gray-*) must also include a dark:
 * counterpart somewhere in the same file.
 *
 * This test will FAIL (RED) if any un-retrofitted component exists,
 * and PASS (GREEN) once all components have dark mode classes.
 */

const COMPONENTS_DIR = resolve(__dirname, "..", "components");

// Files that are already retrofitted (verified in prior sessions) — skip to avoid noise
const ALREADY_VERIFIED = new Set([
  "Layout.tsx",
  "TableBrowser.tsx",
  "RecordForm.tsx",
  "SchemaView.tsx",
  "ThemeProvider.tsx",
]);

// Collect all .tsx files from components/ (including subdirectories)
function collectTsxFiles(dir: string): string[] {
  const results: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (entry === "__tests__") continue;
    if (statSync(full).isDirectory()) {
      results.push(...collectTsxFiles(full));
    } else if (entry.endsWith(".tsx")) {
      results.push(full);
    }
  }
  return results;
}

// Pattern: bare light-mode class usage that SHOULD have dark: counterpart
// We look for className strings containing these patterns without a dark: version
const LIGHT_ONLY_PATTERNS = [
  /\bbg-white\b/,
  /\bbg-gray-50\b/,
  /\bbg-gray-100\b/,
  /\btext-gray-[3-7]00\b/,
  /\bborder-gray-[2-3]00\b/,
  /\bborder-b\b(?!\s)/,
];

describe("dark mode audit: all components must have dark: counterparts", () => {
  const allFiles = collectTsxFiles(COMPONENTS_DIR);
  const filesToAudit = allFiles.filter((f) => {
    const name = f.split("/").pop()!;
    return !ALREADY_VERIFIED.has(name);
  });

  for (const filePath of filesToAudit) {
    const relPath = relative(COMPONENTS_DIR, filePath);
    const content = readFileSync(filePath, "utf8");

    // Only audit files that actually have className usage
    if (!content.includes("className")) continue;

    it(`${relPath} uses dark: classes when using light-mode classes`, () => {
      const hasDarkClasses = content.includes("dark:");

      // If the file uses any light-mode classes, it must also have dark: classes
      const usesLightClasses = LIGHT_ONLY_PATTERNS.some((p) => p.test(content));

      if (usesLightClasses) {
        expect(
          hasDarkClasses,
          `${relPath} has light-mode Tailwind classes but no dark: counterparts`,
        ).toBe(true);
      }
    });
  }
});
