// Barrel re-export — public API surface only.
// Internal client helpers (request, requestNoBody, requestAuth, getAdminToken,
// emitUnauthorized) are intentionally NOT re-exported here; they were never
// part of the public API.  Domain files import them directly from ./api_client.

export {
  setToken,
  clearToken,
  getAuthToken,
  setAuthToken,
  clearAuthToken,
  ApiError,
} from "./api_client";

export * from "./api_auth";
export * from "./api_schema";
export * from "./api_database";
export * from "./api_webhooks";
export * from "./api_storage";
export * from "./api_edge_functions";
export * from "./api_push";
export * from "./api_email";
export * from "./api_admin";
