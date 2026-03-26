import type { AppResponse } from "../types";

export function formatAppRateLimit(app: AppResponse | undefined): string {
  if (!app) {
    return "Rate: unknown";
  }
  if (app.rateLimitRps <= 0) {
    return "Rate: unlimited";
  }
  return `Rate: ${app.rateLimitRps} req/${app.rateLimitWindowSeconds}s`;
}
