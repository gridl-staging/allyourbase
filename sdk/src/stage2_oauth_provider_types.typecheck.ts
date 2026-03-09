import type {
  OAuthClient,
  OAuthClientListResponse,
  CreateOAuthClientRequest,
  CreateOAuthClientResponse,
  UpdateOAuthClientRequest,
  RotateOAuthClientSecretResponse,
  OAuthTokenResponse,
} from "./index";

const client: OAuthClient = {
  id: "00000000-0000-0000-0000-000000000001",
  appId: "00000000-0000-0000-0000-000000000002",
  clientId: "ayb_cid_abcdef1234567890abcdef1234567890abcdef1234567890",
  name: "My OAuth App",
  redirectUris: ["https://example.com/callback"],
  scopes: ["readonly"],
  clientType: "confidential",
  createdAt: "2026-02-22T00:00:00Z",
  updatedAt: "2026-02-22T00:00:00Z",
  revokedAt: null,
  activeAccessTokenCount: 5,
  activeRefreshTokenCount: 3,
  totalGrants: 10,
  lastTokenIssuedAt: "2026-02-22T12:00:00Z",
};

const clientList: OAuthClientListResponse = {
  items: [client],
  page: 1,
  perPage: 20,
  totalItems: 1,
  totalPages: 1,
};

const createReq: CreateOAuthClientRequest = {
  appId: "00000000-0000-0000-0000-000000000002",
  name: "My OAuth App",
  redirectUris: ["https://example.com/callback"],
  scopes: ["readonly"],
};

const createReqWithOptionals: CreateOAuthClientRequest = {
  appId: "00000000-0000-0000-0000-000000000002",
  name: "My OAuth App",
  redirectUris: ["https://example.com/callback"],
  scopes: ["readwrite"],
  clientType: "public",
};

const createRes: CreateOAuthClientResponse = {
  clientSecret: "ayb_cs_abcdef1234567890",
  client: client,
};

const createResPublic: CreateOAuthClientResponse = {
  client: client,
};

const updateReq: UpdateOAuthClientRequest = {
  name: "Updated Name",
  redirectUris: ["https://example.com/new-callback"],
  scopes: ["readwrite"],
};

const rotateRes: RotateOAuthClientSecretResponse = {
  clientSecret: "ayb_cs_newsecrethex",
};

const tokenRes: OAuthTokenResponse = {
  access_token: "ayb_at_abc123",
  token_type: "Bearer",
  expires_in: 3600,
  scope: "readonly",
};

const tokenResWithRefresh: OAuthTokenResponse = {
  access_token: "ayb_at_abc123",
  token_type: "Bearer",
  expires_in: 3600,
  refresh_token: "ayb_rt_xyz456",
  scope: "readwrite",
};

void clientList;
void createReq;
void createReqWithOptionals;
void createRes;
void createResPublic;
void updateReq;
void rotateRes;
void tokenRes;
void tokenResWithRefresh;
