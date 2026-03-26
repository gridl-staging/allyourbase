/**
 * @module Utilities for generating Row-Level Security (RLS) policy SQL statements and providing predefined policy templates for common access control patterns.
 */
import type { RlsPolicy } from "../types";

export const RLS_POLICY_COMMANDS = ["ALL", "SELECT", "INSERT", "UPDATE", "DELETE"] as const;

export interface PolicyTemplate {
  name: string;
  description: string;
  command: string;
  using: string;
  withCheck: string;
}

export const POLICY_TEMPLATES: PolicyTemplate[] = [
  {
    name: "Owner only",
    description: "Users can only access their own rows",
    command: "ALL",
    using: "(user_id = current_setting('ayb.user_id', true)::uuid)",
    withCheck: "(user_id = current_setting('ayb.user_id', true)::uuid)",
  },
  {
    name: "Public read, owner write",
    description: "Anyone can read, only owner can modify",
    command: "SELECT",
    using: "true",
    withCheck: "",
  },
  {
    name: "Role-based access",
    description: "Only authenticated role can access",
    command: "ALL",
    using: "(current_setting('ayb.user_role', true) = 'authenticated')",
    withCheck: "(current_setting('ayb.user_role', true) = 'authenticated')",
  },
  {
    name: "Tenant isolation",
    description: "Rows filtered by tenant_id session variable",
    command: "ALL",
    using: "(tenant_id = current_setting('ayb.tenant_id', true)::uuid)",
    withCheck: "(tenant_id = current_setting('ayb.tenant_id', true)::uuid)",
  },
];

/**
 * Generates a PostgreSQL CREATE POLICY SQL statement from an RLS policy configuration. Constructs a complete policy definition string with optional clauses for roles, USING conditions, and WITH CHECK conditions.
 * @param policy - Configuration object containing policy name, table schema/name, command, permissive mode, roles, and expressions
 * @returns A valid PostgreSQL CREATE POLICY statement ready for execution
 */
export function generatePolicySql(policy: RlsPolicy): string {
  let sql = `CREATE POLICY "${policy.policyName}" ON "${policy.tableSchema}"."${policy.tableName}"`;
  if (policy.permissive === "RESTRICTIVE") {
    sql += "\n  AS RESTRICTIVE";
  }

  sql += `\n  FOR ${policy.command}`;

  if (policy.roles.length > 0) {
    sql += `\n  TO ${policy.roles.join(", ")}`;
  }

  if (policy.usingExpr) {
    sql += `\n  USING (${policy.usingExpr})`;
  }

  if (policy.withCheckExpr) {
    sql += `\n  WITH CHECK (${policy.withCheckExpr})`;
  }

  return sql + ";";
}
