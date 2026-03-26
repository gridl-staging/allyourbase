import { request } from "./api_client";
import type { AuthHooksConfig } from "./types/auth_hooks";

export function getAuthHooks(): Promise<AuthHooksConfig> {
  return request<AuthHooksConfig>("/api/admin/auth/hooks");
}
