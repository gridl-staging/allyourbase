export interface APIKeyResponse {
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

export interface APIKeyListResponse {
  items: APIKeyResponse[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export interface APIKeyCreateResponse {
  key: string;
  apiKey: APIKeyResponse;
}
