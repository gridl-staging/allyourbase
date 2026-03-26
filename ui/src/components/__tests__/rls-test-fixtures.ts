import type { RlsPolicy } from "../../types";

export function makePolicy(overrides: Partial<RlsPolicy> = {}): RlsPolicy {
  return {
    tableSchema: "public",
    tableName: "posts",
    policyName: "owner_access",
    command: "ALL",
    permissive: "PERMISSIVE",
    roles: ["authenticated"],
    usingExpr: "(user_id = current_setting('ayb.user_id', true)::uuid)",
    withCheckExpr: "(user_id = current_setting('ayb.user_id', true)::uuid)",
    ...overrides,
  };
}
