import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

function readProjectFile(relativePath: string): string {
  return readFileSync(resolve(__dirname, "..", "..", relativePath), "utf8");
}

describe("stage 2 theme contracts", () => {
  it("keeps Tailwind dark mode configured to class strategy", () => {
    const config = readProjectFile("tailwind.config.ts");
    expect(config).toMatch(/darkMode:\s*["']class["']/);
  });

  it("defines shared light and dark design tokens in index.css", () => {
    const css = readProjectFile("src/index.css");

    expect(css).toContain(":root {");
    expect(css).toContain(".dark {");
    expect(css).toContain("--color-bg-primary:");
    expect(css).toContain("--color-text-primary:");
    expect(css).toContain("--color-border-primary:");
    expect(css).toContain("--focus-ring:");
  });

  it("theme persistence browser specs assert actual visual theme changes", () => {
    const mockedSpec = readProjectFile("browser-tests-mocked/theme-persistence.spec.ts");
    const unmockedSpec = readProjectFile(
      "browser-tests-unmocked/full/dark-mode-persistence.spec.ts",
    );

    expect(mockedSpec).toContain('page.locator("aside")');
    expect(mockedSpec).toContain('toHaveCSS("background-color", "rgb(17, 24, 39)")');
    expect(mockedSpec).toContain('toHaveCSS("background-color", "rgb(255, 255, 255)")');

    expect(unmockedSpec).toContain('page.locator("aside")');
    expect(unmockedSpec).toContain('toHaveCSS("background-color", "rgb(17, 24, 39)")');
    expect(unmockedSpec).toContain('toHaveCSS("background-color", "rgb(255, 255, 255)")');
  });
});
