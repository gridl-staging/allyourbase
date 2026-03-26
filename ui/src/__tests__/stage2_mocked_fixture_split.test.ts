import { describe, expect, it } from "vitest";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import ts from "typescript";
import * as mockedFixtures from "../../browser-tests-mocked/fixtures";

function projectPath(relativePath: string): string {
  return resolve(__dirname, "..", "..", relativePath);
}

function readProjectFile(relativePath: string): string {
  return readFileSync(projectPath(relativePath), "utf8");
}

function projectFileLineCount(relativePath: string): number {
  return readProjectFile(relativePath).split("\n").length;
}

function exportedFunctionLineCount(relativePath: string, functionName: string): number {
  const sourceText = readProjectFile(relativePath);
  const sourceFile = ts.createSourceFile(
    projectPath(relativePath),
    sourceText,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );

  let lineCount = -1;
  sourceFile.forEachChild((node) => {
    if (!ts.isFunctionDeclaration(node) || node.name?.text !== functionName) {
      return;
    }

    const startLine = sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile)).line;
    const endLine = sourceFile.getLineAndCharacterOfPosition(node.end).line;
    lineCount = endLine - startLine + 1;
  });

  if (lineCount < 0) {
    throw new Error(`Could not find exported function ${functionName} in ${relativePath}`);
  }

  return lineCount;
}

describe("stage 2 mocked fixture module split contracts", () => {
  const expectedFixtureModules = [
    "browser-tests-mocked/fixtures/index.ts",
    "browser-tests-mocked/fixtures/core.ts",
    "browser-tests-mocked/fixtures/email-templates.ts",
    "browser-tests-mocked/fixtures/push.ts",
    "browser-tests-mocked/fixtures/apps.ts",
    "browser-tests-mocked/fixtures/edge-functions.ts",
    "browser-tests-mocked/fixtures/auth.ts",
    "browser-tests-mocked/fixtures/admin-logs.ts",
    "browser-tests-mocked/fixtures/usage-metering.ts",
    "browser-tests-mocked/fixtures/storage-cdn.ts",
  ];

  const extractedRouteSpecs = [
    "browser-tests-mocked/performance-advisor.spec.ts",
    "browser-tests-mocked/security-advisor.spec.ts",
    "browser-tests-mocked/schema-designer.spec.ts",
    "browser-tests-mocked/theme-persistence.spec.ts",
  ];

  it("creates the required mocked fixture module files", () => {
    for (const fixtureModulePath of expectedFixtureModules) {
      expect(existsSync(projectPath(fixtureModulePath))).toBe(true);
    }
  });

  it("keeps top-level mocked fixtures entrypoint as a thin barrel", () => {
    const fixturesEntrypoint = readProjectFile("browser-tests-mocked/fixtures.ts");
    expect(fixturesEntrypoint).toContain('export * from "./fixtures/index"');
    expect(fixturesEntrypoint).not.toContain("page.route(");
    expect(projectFileLineCount("browser-tests-mocked/fixtures.ts")).toBeLessThanOrEqual(20);
  });

  it("re-exports mocked fixture helpers through the top-level barrel at runtime", () => {
    expect(typeof mockedFixtures.test).toBe("function");
    expect(typeof mockedFixtures.bootstrapMockedAdminApp).toBe("function");
    expect(typeof mockedFixtures.mockAdminEdgeFunctionApis).toBe("function");
    expect(typeof mockedFixtures.mockAdminLogsApis).toBe("function");
    expect(typeof mockedFixtures.mockUsageMeteringApis).toBe("function");
    expect(typeof mockedFixtures.mockStorageCDNApis).toBe("function");
    expect(typeof mockedFixtures.mockTenantAdminApis).toBe("function");
  });

  it("keeps each mocked fixture module under 800 lines", () => {
    for (const fixtureModulePath of expectedFixtureModules) {
      expect(projectFileLineCount(fixtureModulePath)).toBeLessThan(800);
    }
  });

  it("extracts inline page.route handlers from targeted mocked specs", () => {
    for (const specPath of extractedRouteSpecs) {
      const specSource = readProjectFile(specPath);
      expect(specSource).not.toContain('page.route("**/api/**"');
    }
  });

  it("exposes extracted API mock builders for each targeted mocked spec", () => {
    const coreModule = readProjectFile("browser-tests-mocked/fixtures/core.ts");

    expect(coreModule).toContain("export async function mockRealtimeInspectorApis(");
    expect(coreModule).toContain("export async function mockPerformanceAdvisorApis(");
    expect(coreModule).toContain("export async function mockSecurityAdvisorApis(");
    expect(coreModule).toContain("export async function mockSchemaDesignerApis(");
    expect(coreModule).toContain("export async function mockThemePersistenceApis(");
  });

  it("keeps extracted mocked helpers fail-fast on unexpected api routes", () => {
    const coreModule = readProjectFile("browser-tests-mocked/fixtures/core.ts");
    expect(coreModule).toContain("Unhandled mocked API route");
    expect(coreModule).not.toContain("return json(route, 200, {});");
  });

  it("keeps the oversized mocked helpers under the 100-line hard limit", () => {
    expect(exportedFunctionLineCount("browser-tests-mocked/fixtures/auth.ts", "mockMFAApis")).toBeLessThanOrEqual(100);
    expect(exportedFunctionLineCount("browser-tests-mocked/fixtures/auth.ts", "mockAuthProviderApis")).toBeLessThanOrEqual(100);
    expect(exportedFunctionLineCount("browser-tests-mocked/fixtures/edge-functions.ts", "mockAdminEdgeFunctionApis")).toBeLessThanOrEqual(100);
  });

  it("uses exact logs-tab locators in mocked edge function specs", () => {
    const edgeFunctionsSpec = readProjectFile("browser-tests-mocked/edge-functions.spec.ts");
    expect(edgeFunctionsSpec).not.toContain('getByRole("button", { name: /Logs/i })');
    expect(edgeFunctionsSpec).toContain('getByRole("button", { name: "Logs", exact: true })');
  });

  it("uses an exact back-button locator in mocked edge function specs", () => {
    const edgeFunctionsSpec = readProjectFile("browser-tests-mocked/edge-functions.spec.ts");
    expect(edgeFunctionsSpec).not.toContain('getByRole("button", { name: /Back/i })');
    expect(edgeFunctionsSpec).toContain('getByRole("button", { name: "Back", exact: true })');
  });
});
