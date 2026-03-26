/**
 * @module Type definitions for application layout navigation views and utilities to distinguish admin-only views from data-focused views.
 */
/**
 * String literal inventories used to derive the view unions and the runtime
 * admin-view guard from the same source of truth.
 */
type DataView = "data" | "schema" | "sql";

const ADMIN_VIEWS = [
  "webhooks",
  "storage",
  "sites",
  "users",
  "functions",
  "edge-functions",
  "apps",
  "api-keys",
  "oauth-clients",
  "api-explorer",
  "rls",
  "sql-editor",
  "schema-designer",
  "sms-health",
  "sms-messages",
  "email-templates",
  "push",
  "jobs",
  "schedules",
  "matviews",
  "auth-settings",
  "mfa-management",
  "account-linking",
  "branches",
  "realtime-inspector",
  "security-advisor",
  "performance-advisor",
  "backups",
  "analytics",
  "usage",
  "replicas",
  "ai-assistant",
  "audit-logs",
  "admin-logs",
  "secrets",
  "saml",
  "custom-domains",
  "extensions",
  "vector-indexes",
  "log-drains",
  "stats",
  "auth-hooks",
  "notifications",
  "fdw",
  "incidents",
  "support-tickets",
  "tenants",
  "organizations",
] as const;

export type View = DataView | (typeof ADMIN_VIEWS)[number];

export type AdminView = (typeof ADMIN_VIEWS)[number];

const ADMIN_VIEW_SET: ReadonlySet<AdminView> = new Set(ADMIN_VIEWS);

export function isAdminView(view: View): view is AdminView {
  return ADMIN_VIEW_SET.has(view as AdminView);
}
