import { describe, expect, it } from "vitest";
import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import * as unmockedFixtures from "../../browser-tests-unmocked/fixtures";

function projectPath(relativePath: string): string {
  return resolve(__dirname, "..", "..", relativePath);
}

function readProjectFile(relativePath: string): string {
  return readFileSync(projectPath(relativePath), "utf8");
}

function projectFileLineCount(relativePath: string): number {
  return readProjectFile(relativePath).split("\n").length;
}

describe("stage 2 unmocked fixture module split contracts", () => {
  const expectedFixtureModules = [
    "browser-tests-unmocked/fixtures/index.ts",
    "browser-tests-unmocked/fixtures/core.ts",
    "browser-tests-unmocked/fixtures/auth.ts",
    "browser-tests-unmocked/fixtures/sms.ts",
    "browser-tests-unmocked/fixtures/push.ts",
    "browser-tests-unmocked/fixtures/admin.ts",
    "browser-tests-unmocked/fixtures/jobs.ts",
    "browser-tests-unmocked/fixtures/infra.ts",
    "browser-tests-unmocked/fixtures/edge-functions.ts",
    "browser-tests-unmocked/fixtures/storage.ts",
    "browser-tests-unmocked/fixtures/usage.ts",
    "browser-tests-unmocked/fixtures/tenants.ts",
    "browser-tests-unmocked/fixtures/orgs.ts",
  ];

  it("creates the required unmocked fixture module files", () => {
    for (const fixtureModulePath of expectedFixtureModules) {
      expect(existsSync(projectPath(fixtureModulePath))).toBe(true);
    }
  });

  it("keeps top-level unmocked fixtures entrypoint as a thin barrel", () => {
    const fixturesEntrypoint = readProjectFile("browser-tests-unmocked/fixtures.ts");
    expect(fixturesEntrypoint).toContain('export * from "./fixtures/index"');
    expect(fixturesEntrypoint).not.toContain("base.extend<");
    expect(projectFileLineCount("browser-tests-unmocked/fixtures.ts")).toBeLessThanOrEqual(20);
  });

  it("defines test fixture extension and expect re-export in fixtures/index.ts", () => {
    const fixturesIndex = readProjectFile("browser-tests-unmocked/fixtures/index.ts");
    expect(fixturesIndex).toContain("export const test = base.extend<");
    expect(fixturesIndex).toContain('export { expect } from "@playwright/test"');
  });

  it("re-exports newly ported unmocked fixture helpers through the top-level barrel", () => {
    expect(typeof unmockedFixtures.triggerAdminStatsRequest).toBe("function");
    expect(typeof unmockedFixtures.seedUsageMeteringTenantDailyRows).toBe("function");
    expect(typeof unmockedFixtures.cleanupUsageMeteringTenant).toBe("function");
    expect(typeof unmockedFixtures.seedTenantDashboardSmokeTenant).toBe("function");
    expect(typeof unmockedFixtures.cleanupTenantDashboardSmokeTenant).toBe("function");
    expect(typeof unmockedFixtures.seedOrganizationDashboardSmokeOrg).toBe("function");
    expect(typeof unmockedFixtures.cleanupOrganizationDashboardSmokeOrg).toBe("function");
  });

  it("keeps each unmocked fixture module under 800 lines", () => {
    for (const fixtureModulePath of expectedFixtureModules) {
      expect(projectFileLineCount(fixtureModulePath)).toBeLessThan(800);
    }
  });

  it("exports core fixture APIs from the split module surface", () => {
    const coreModule = readProjectFile("browser-tests-unmocked/fixtures/core.ts");
    expect(coreModule).toContain("export async function checkAuthEnabled(");
    expect(coreModule).toContain("export async function execSQL(");
    expect(coreModule).toContain("export async function probeEndpoint(");
    expect(coreModule).toContain("export function sqlLiteral(");
  });
});
