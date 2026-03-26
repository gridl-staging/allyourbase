import { test as base } from "@playwright/test";

export * from "./core";
export * from "./email-templates";
export * from "./push";
export * from "./apps";
export * from "./edge-functions";
export * from "./auth";
export * from "./api-keys";
export * from "./oauth-clients";
export * from "./webhooks";
export * from "./storage";
export * from "./secrets";
export * from "./fdw";
export * from "./sql-editor";
export * from "./notifications";
export * from "./support-tickets";
export * from "./incidents";
export * from "./admin-logs";
export * from "./usage-metering";
export * from "./storage-cdn";
export { mockTenantAdminApis, type TenantAdminMockState } from "../fixtures-tenants";

export const test = base;
export { expect } from "@playwright/test";
