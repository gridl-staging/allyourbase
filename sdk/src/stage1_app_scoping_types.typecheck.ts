import type {
  AdminAPIKey,
  AdminAPIKeyListResponse,
  App,
  AppListResponse,
  CreateAdminAPIKeyRequest,
  CreateAdminAPIKeyResponse,
} from "./index";

const app: App = {
  id: "00000000-0000-0000-0000-000000000001",
  name: "sigil-web",
  description: "Sigil web client",
  ownerUserId: "00000000-0000-0000-0000-000000000002",
  rateLimitRps: 30,
  rateLimitWindowSeconds: 60,
  createdAt: "2026-02-22T00:00:00Z",
  updatedAt: "2026-02-22T00:00:00Z",
};

const apps: AppListResponse = {
  items: [app],
  page: 1,
  perPage: 20,
  totalItems: 1,
  totalPages: 1,
};

const key: AdminAPIKey = {
  id: "00000000-0000-0000-0000-000000000003",
  userId: "00000000-0000-0000-0000-000000000002",
  name: "sigil-ingestor",
  keyPrefix: "ayb_abc123",
  scope: "readonly",
  allowedTables: ["workouts"],
  appId: app.id,
  lastUsedAt: null,
  expiresAt: null,
  createdAt: "2026-02-22T00:00:00Z",
  revokedAt: null,
};

const keys: AdminAPIKeyListResponse = {
  items: [key],
  page: 1,
  perPage: 20,
  totalItems: 1,
  totalPages: 1,
};

const createReq: CreateAdminAPIKeyRequest = {
  userId: app.ownerUserId,
  name: "sigil-ingestor",
  scope: "readonly",
  allowedTables: ["workouts"],
  appId: app.id,
};

const createRes: CreateAdminAPIKeyResponse = {
  key: "ayb_secret_key",
  apiKey: key,
};

void apps;
void keys;
void createReq;
void createRes;
