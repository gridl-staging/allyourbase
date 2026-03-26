import { describe, it, expect } from "vitest";
import {
  POLICY_TEMPLATES,
  RLS_POLICY_COMMANDS,
  generatePolicySql,
} from "../rls-helpers";
import { makePolicy } from "./rls-test-fixtures";

describe("rls-helpers", () => {
  it("exports stable commands and templates", () => {
    expect(RLS_POLICY_COMMANDS).toEqual(["ALL", "SELECT", "INSERT", "UPDATE", "DELETE"]);
    expect(POLICY_TEMPLATES).toHaveLength(4);
    expect(POLICY_TEMPLATES[0].name).toBe("Owner only");
  });

  it("builds SQL with restrictive flag and optional clauses", () => {
    const sql = generatePolicySql(
      makePolicy({
        permissive: "RESTRICTIVE",
        roles: ["authenticated", "admin"],
      }),
    );

    expect(sql).toContain('CREATE POLICY "owner_access" ON "public"."posts"');
    expect(sql).toContain("AS RESTRICTIVE");
    expect(sql).toContain("FOR ALL");
    expect(sql).toContain("TO authenticated, admin");
    expect(sql).toContain("USING");
    expect(sql).toContain("WITH CHECK");
    expect(sql.endsWith(";")).toBe(true);
  });

  it("omits optional SQL clauses when policy fields are empty", () => {
    const sql = generatePolicySql(
      makePolicy({
        permissive: "PERMISSIVE",
        roles: [],
        usingExpr: null,
        withCheckExpr: null,
      }),
    );

    expect(sql).not.toContain("AS RESTRICTIVE");
    expect(sql).not.toContain(" TO ");
    expect(sql).not.toContain("USING (");
    expect(sql).not.toContain("WITH CHECK (");
  });
});
