/**
 * @module OAuth client types for API responses: full client records, list pagination, creation responses with secrets, and secret rotation responses.
 */
/**
 * Represents a registered OAuth client with its configuration, lifecycle state, and token usage metrics.
 */
export interface OAuthClientResponse {
  id: string;
  appId: string;
  clientId: string;
  name: string;
  redirectUris: string[];
  scopes: string[];
  clientType: string;
  createdAt: string;
  updatedAt: string;
  revokedAt: string | null;
  activeAccessTokenCount: number;
  activeRefreshTokenCount: number;
  totalGrants: number;
  lastTokenIssuedAt: string | null;
}

export interface OAuthClientListResponse {
  items: OAuthClientResponse[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export interface OAuthClientCreateResponse {
  clientSecret: string;
  client: OAuthClientResponse;
}

export interface OAuthClientRotateSecretResponse {
  clientSecret: string;
}
