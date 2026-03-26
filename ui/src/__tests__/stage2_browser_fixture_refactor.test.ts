import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

function readProjectFile(relativePath: string): string {
  return readFileSync(resolve(__dirname, "..", "..", relativePath), "utf8");
}

describe("stage 2 browser fixture refactor contracts", () => {
  it("exports sqlLiteral from unmocked shared fixtures", () => {
    const fixturesSource = readProjectFile("browser-tests-unmocked/fixtures.ts");
    const coreSource = readProjectFile("browser-tests-unmocked/fixtures/core.ts");
    expect(fixturesSource).toContain('export * from "./fixtures/index"');
    expect(coreSource).toMatch(/export function sqlLiteral\(/);
  });

  it("removes duplicated sqlLiteral/createUser/createApp helpers from api-keys lifecycle spec", () => {
    const apiKeysSpec = readProjectFile("browser-tests-unmocked/full/api-keys-lifecycle.spec.ts");
    expect(apiKeysSpec).not.toMatch(/function sqlLiteral\(/);
    expect(apiKeysSpec).not.toMatch(/async function createUser\(/);
    expect(apiKeysSpec).not.toMatch(/async function createApp\(/);
    expect(apiKeysSpec).toContain("ensureUserByEmail");
    expect(apiKeysSpec).toContain("seedApiKey");
    expect(apiKeysSpec).toContain("seedAdminApp");
    expect(apiKeysSpec).not.toContain("INSERT INTO _ayb_api_keys");
  });

  it("removes duplicated sqlLiteral helper from email templates lifecycle spec", () => {
    const emailTemplatesSpec = readProjectFile("browser-tests-unmocked/full/email-templates-lifecycle.spec.ts");
    expect(emailTemplatesSpec).not.toMatch(/function sqlLiteral\(/);
  });

  it("reuses shared sqlLiteral escaping inside sms fixture helpers", () => {
    const smsFixtures = readProjectFile("browser-tests-unmocked/fixtures/sms.ts");
    expect(smsFixtures).toContain('import { execSQL, sqlLiteral } from "./core"');
    expect(smsFixtures).toContain("const safeToPhone = sqlLiteral(toPhone);");
    expect(smsFixtures).toContain("const safeBodyPrefix = sqlLiteral(bodyPrefix);");
    expect(smsFixtures).toContain("escapeLikePattern(bodyPattern)");
  });

  it("uses afterEach cleanup in the seven targeted browser specs", () => {
    const specs = [
      "browser-tests-unmocked/full/auth-mfa-lifecycle.spec.ts",
      "browser-tests-unmocked/full/edge-function-triggers.spec.ts",
      "browser-tests-unmocked/full/functions-browser.spec.ts",
      "browser-tests-unmocked/smoke/edge-functions-crud.spec.ts",
      "browser-tests-unmocked/smoke/sms-health.spec.ts",
      "browser-tests-unmocked/smoke/sms-messages.spec.ts",
      "browser-tests-unmocked/smoke/storage-upload.spec.ts",
    ];

    for (const specPath of specs) {
      const specSource = readProjectFile(specPath);
      expect(specSource).toContain("test.afterEach(");
      expect(specSource).not.toContain("finally {");
    }
  });
});
