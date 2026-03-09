/** List response envelope returned by collection endpoints. */
export interface ListResponse<T = Record<string, unknown>> {
  items: T[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

/** Parameters for listing records. */
export interface ListParams {
  page?: number;
  perPage?: number;
  sort?: string;
  filter?: string;
  search?: string;
  fields?: string;
  expand?: string;
  skipTotal?: boolean;
}

/** Parameters for reading a single record. */
export interface GetParams {
  fields?: string;
  expand?: string;
}

/** Auth tokens returned by login/register. */
export interface AuthResponse {
  token: string;
  refreshToken: string;
  user: User;
}

/** User record from the auth system. */
export interface User {
  id: string;
  email: string;
  emailVerified?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

/** Registered app (matches admin apps API response). */
export interface App {
  id: string;
  name: string;
  description: string;
  ownerUserId: string;
  rateLimitRps: number;
  rateLimitWindowSeconds: number;
  createdAt: string;
  updatedAt: string;
}

/** Paginated app list envelope returned by admin apps API. */
export interface AppListResponse {
  items: App[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

/** Admin API key record (matches admin api-keys API response). */
export interface AdminAPIKey {
  id: string;
  userId: string;
  name: string;
  keyPrefix: string;
  scope: string;
  allowedTables: string[] | null;
  appId: string | null;
  lastUsedAt: string | null;
  expiresAt: string | null;
  createdAt: string;
  revokedAt: string | null;
}

/** Paginated admin API key list envelope. */
export interface AdminAPIKeyListResponse {
  items: AdminAPIKey[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

/** Request body for creating an admin API key. */
export interface CreateAdminAPIKeyRequest {
  userId: string;
  name: string;
  scope?: string;
  allowedTables?: string[];
  appId?: string;
}

/** Response body when an admin API key is created. */
export interface CreateAdminAPIKeyResponse {
  key: string;
  apiKey: AdminAPIKey;
}

/** Realtime event from SSE stream. */
export interface RealtimeEvent {
  action: "create" | "update" | "delete";
  table: string;
  record: Record<string, unknown>;
}

/** Stored file metadata returned by storage endpoints. */
export interface StorageObject {
  id: string;
  bucket: string;
  name: string;
  size: number;
  contentType: string;
  userId?: string;
  createdAt: string;
  updatedAt: string;
}

/** A single operation within a batch request. */
export interface BatchOperation {
  method: "create" | "update" | "delete";
  id?: string;
  body?: Record<string, unknown>;
}

/** Result of a single operation within a batch response. */
export interface BatchResult<T = Record<string, unknown>> {
  index: number;
  status: number;
  body?: T;
}

/** Client configuration options. */
export interface ClientOptions {
  /** Custom fetch implementation (e.g. for Node.js < 18). */
  fetch?: typeof globalThis.fetch;
}

/** Registered OAuth client (matches admin OAuth clients API response). */
export interface OAuthClient {
  id: string;
  appId: string;
  clientId: string;
  name: string;
  redirectUris: string[];
  scopes: string[];
  clientType: "confidential" | "public";
  createdAt: string;
  updatedAt: string;
  revokedAt: string | null;
  activeAccessTokenCount: number;
  activeRefreshTokenCount: number;
  totalGrants: number;
  lastTokenIssuedAt: string | null;
}

/** Paginated OAuth client list envelope. */
export interface OAuthClientListResponse {
  items: OAuthClient[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

/** Request body for creating an OAuth client. */
export interface CreateOAuthClientRequest {
  appId: string;
  name: string;
  redirectUris: string[];
  scopes: string[];
  clientType?: "confidential" | "public";
}

/** Response body when an OAuth client is created. */
export interface CreateOAuthClientResponse {
  clientSecret?: string;
  client: OAuthClient;
}

/** Request body for updating an OAuth client. */
export interface UpdateOAuthClientRequest {
  name: string;
  redirectUris: string[];
  scopes: string[];
}

/** Response body when an OAuth client secret is rotated. */
export interface RotateOAuthClientSecretResponse {
  clientSecret: string;
}

/** RFC 6749 §5.1 OAuth token response from the token endpoint. */
export interface OAuthTokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token?: string;
  scope: string;
}

/** Supported OAuth providers. */
export type OAuthProvider = "google" | "github";

/** Options for the `signInWithOAuth()` method. */
export interface OAuthOptions {
  /** Additional scopes to request from the OAuth provider. */
  scopes?: string[];
  /**
   * Custom URL handler for redirect-based flow (instead of popup).
   * When provided, no popup is opened — the SDK calls this with the
   * authorization URL so the app can redirect.
   * Use this for iOS PWAs or when popups are blocked.
   */
  urlCallback?: (url: string) => void | Promise<void>;
}

/** Auth state change events emitted by `onAuthStateChange`. */
export type AuthStateEvent = "SIGNED_IN" | "SIGNED_OUT" | "TOKEN_REFRESHED";

/** Callback for auth state change events. */
export type AuthStateListener = (
  event: AuthStateEvent,
  session: { token: string; refreshToken: string } | null,
) => void;
