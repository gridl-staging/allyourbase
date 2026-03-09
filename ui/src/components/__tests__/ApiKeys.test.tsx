import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiKeys } from "../ApiKeys";
import {
  listApiKeys,
  createApiKey,
  revokeApiKey,
  listUsers,
  listApps,
} from "../../api";
import type {
  APIKeyResponse,
  APIKeyListResponse,
  APIKeyCreateResponse,
  UserListResponse,
  AppResponse,
  AppListResponse,
} from "../../types";

vi.mock("../../api", () => ({
  listApiKeys: vi.fn(),
  createApiKey: vi.fn(),
  revokeApiKey: vi.fn(),
  listUsers: vi.fn(),
  listApps: vi.fn(),
  ApiError: class extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
}));

const mockListApiKeys = vi.mocked(listApiKeys);
const mockCreateApiKey = vi.mocked(createApiKey);
const mockRevokeApiKey = vi.mocked(revokeApiKey);
const mockListUsers = vi.mocked(listUsers);
const mockListApps = vi.mocked(listApps);

function makeKey(overrides: Partial<APIKeyResponse> = {}): APIKeyResponse {
  return {
    id: "k1",
    userId: "u1",
    name: "CI/CD",
    keyPrefix: "ayb_abc12345",
    scope: "*",
    allowedTables: null,
    appId: null,
    lastUsedAt: null,
    expiresAt: null,
    createdAt: "2026-02-10T12:00:00Z",
    revokedAt: null,
    ...overrides,
  };
}

function makeResponse(
  keys: APIKeyResponse[] = [],
  overrides: Partial<APIKeyListResponse> = {},
): APIKeyListResponse {
  return {
    items: keys,
    page: 1,
    perPage: 20,
    totalItems: keys.length,
    totalPages: keys.length > 0 ? 1 : 0,
    ...overrides,
  };
}

function makeUserResponse(): UserListResponse {
  return {
    items: [
      {
        id: "u1",
        email: "alice@example.com",
        emailVerified: true,
        createdAt: "2026-02-10T12:00:00Z",
        updatedAt: "2026-02-10T12:00:00Z",
      },
    ],
    page: 1,
    perPage: 100,
    totalItems: 1,
    totalPages: 1,
  };
}

function makeApp(overrides: Partial<AppResponse> = {}): AppResponse {
  return {
    id: "a1",
    name: "Orders Service",
    description: "Processes orders",
    ownerUserId: "u1",
    rateLimitRps: 120,
    rateLimitWindowSeconds: 60,
    createdAt: "2026-02-21T00:00:00Z",
    updatedAt: "2026-02-21T00:00:00Z",
    ...overrides,
  };
}

function makeAppResponse(
  apps: AppResponse[] = [],
  overrides: Partial<AppListResponse> = {},
): AppListResponse {
  return {
    items: apps,
    page: 1,
    perPage: 100,
    totalItems: apps.length,
    totalPages: 1,
    ...overrides,
  };
}

describe("ApiKeys", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListUsers.mockResolvedValue(makeUserResponse());
    mockListApps.mockResolvedValue(makeAppResponse());
  });

  it("shows loading state", () => {
    mockListApiKeys.mockReturnValue(new Promise(() => {}));
    render(<ApiKeys />);
    expect(screen.getByText("Loading API keys...")).toBeInTheDocument();
  });

  it("renders API keys list", async () => {
    const keys = [
      makeKey({ id: "k1", name: "CI/CD" }),
      makeKey({ id: "k2", name: "Backend Service", keyPrefix: "ayb_def67890" }),
    ];
    mockListApiKeys.mockResolvedValueOnce(makeResponse(keys));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
      expect(screen.getByText("Backend Service")).toBeInTheDocument();
    });
  });

  it("shows empty state when no keys", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });
  });

  it("shows error state with retry", async () => {
    mockListApiKeys.mockRejectedValueOnce(new Error("connection refused"));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("connection refused")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });
  });

  it("shows total key count", async () => {
    const keys = [makeKey()];
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse(keys, { totalItems: 1 }),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("1 key")).toBeInTheDocument();
    });
  });

  it("shows plural key count", async () => {
    const keys = [
      makeKey({ id: "k1", name: "CI/CD" }),
      makeKey({ id: "k2", name: "Backend" }),
    ];
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse(keys, { totalItems: 2 }),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("2 keys")).toBeInTheDocument();
    });
  });

  it("shows key prefix with ellipsis", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ keyPrefix: "ayb_abc12345" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("ayb_abc12345...")).toBeInTheDocument();
    });
  });

  it("shows Active status for non-revoked keys", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ revokedAt: null })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Active")).toBeInTheDocument();
    });
  });

  it("shows Revoked status for revoked keys", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ revokedAt: "2026-02-10T13:00:00Z" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Revoked")).toBeInTheDocument();
    });
  });

  it("shows Never for keys that haven't been used", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ lastUsedAt: null })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Never")).toBeInTheDocument();
    });
  });

  it("create button opens create modal", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));

    expect(screen.getByText("Create API Key")).toBeInTheDocument();
    expect(screen.getByLabelText("Key name")).toBeInTheDocument();
  });

  it("create flow calls createApiKey and shows key", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    const createdResponse: APIKeyCreateResponse = {
      key: "ayb_fullkeyshownonce1234567890abcdef0123456789ab",
      apiKey: makeKey({ id: "k-new", name: "Deploy Key" }),
    };
    mockCreateApiKey.mockResolvedValueOnce(createdResponse);
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));

    // Fill in the form
    await user.type(screen.getByLabelText("Key name"), "Deploy Key");
    // Select user from dropdown
    await user.selectOptions(screen.getByLabelText("User"), "u1");

    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateApiKey).toHaveBeenCalledWith({
        userId: "u1",
        name: "Deploy Key",
        scope: "*",
      });
    });

    // Should show the created key
    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
      expect(
        screen.getByText(
          "ayb_fullkeyshownonce1234567890abcdef0123456789ab",
        ),
      ).toBeInTheDocument();
      expect(
        screen.getByText("Copy this key now. It will not be shown again."),
      ).toBeInTheDocument();
    });
  });

  it("revoke button opens confirmation dialog", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke key"));

    expect(screen.getByText("Revoke API Key")).toBeInTheDocument();
    expect(
      screen.getByText(
        /This will permanently revoke the API key/,
      ),
    ).toBeInTheDocument();
  });

  it("confirming revoke calls revokeApiKey and refreshes", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([makeKey()]));
    mockRevokeApiKey.mockResolvedValueOnce(undefined);
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke key"));

    const dialog = screen
      .getByText("Revoke API Key")
      .closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", { name: "Revoke" });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockRevokeApiKey).toHaveBeenCalledWith("k1");
    });
  });

  it("cancel on revoke dialog closes it", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke key"));
    expect(screen.getByText("Revoke API Key")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Revoke API Key")).not.toBeInTheDocument();
  });

  it("shows user email for known users", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ userId: "u1" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });
  });

  it("revoked keys don't show revoke button", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ revokedAt: "2026-02-10T13:00:00Z" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Revoked")).toBeInTheDocument();
    });

    expect(screen.queryByTitle("Revoke key")).not.toBeInTheDocument();
  });

  it("displays page info for multi-page results", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey()], {
        totalItems: 45,
        totalPages: 3,
        page: 1,
      }),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("45 keys")).toBeInTheDocument();
      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });
  });

  it("retry button refetches keys after error", async () => {
    mockListApiKeys.mockRejectedValueOnce(new Error("network down"));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("network down")).toBeInTheDocument();
    });

    // Retry should fetch again
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    const user = userEvent.setup();
    await user.click(screen.getByText("Retry"));

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });
    // Should have been called twice: initial + retry
    expect(mockListApiKeys).toHaveBeenCalledTimes(2);
  });

  it("create button disabled when name is empty", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));

    // Select user but leave name empty
    await user.selectOptions(screen.getByLabelText("User"), "u1");

    const createBtn = screen.getByRole("button", { name: "Create" });
    expect(createBtn).toBeDisabled();
  });

  it("create button disabled when user is not selected", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));

    // Type name but don't select user
    await user.type(screen.getByLabelText("Key name"), "Test Key");

    const createBtn = screen.getByRole("button", { name: "Create" });
    expect(createBtn).toBeDisabled();
  });

  it("cancel on create modal closes it", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));
    expect(screen.getByText("Create API Key")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Create API Key")).not.toBeInTheDocument();
  });

  it("done button on created modal closes it", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_fullkeyshownonce1234567890abcdef0123456789ab",
      apiKey: makeKey({ id: "k-new", name: "Test Key" }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Test Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Done"));
    expect(screen.queryByText("API Key Created")).not.toBeInTheDocument();
  });

  it("create refreshes key list after success", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_fullkeyshownonce1234567890abcdef0123456789ab",
      apiKey: makeKey({ id: "k-new", name: "Test Key" }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Test Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
    });

    // listApiKeys should have been called twice: initial load + refresh after create
    expect(mockListApiKeys).toHaveBeenCalledTimes(2);
  });

  it("revoke refreshes key list after success", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([makeKey()]));
    mockRevokeApiKey.mockResolvedValueOnce(undefined);
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke key"));

    const dialog = screen
      .getByText("Revoke API Key")
      .closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", { name: "Revoke" });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockRevokeApiKey).toHaveBeenCalledWith("k1");
    });
    // listApiKeys should have been called multiple times (initial + refresh after revoke)
    expect(mockListApiKeys.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("shows last used date when available", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ lastUsedAt: "2026-02-09T08:00:00Z" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      // "Never" should not appear since lastUsedAt is set
      expect(screen.queryByText("Never")).not.toBeInTheDocument();
      // The actual formatted date should appear
      const expectedDate = new Date("2026-02-09T08:00:00Z").toLocaleDateString();
      expect(screen.getByText(expectedDate)).toBeInTheDocument();
    });
  });

  it("shows user ID when email not available", async () => {
    mockListUsers.mockResolvedValueOnce({
      items: [],
      page: 1,
      perPage: 100,
      totalItems: 0,
      totalPages: 0,
    });
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ userId: "unknown-user-id" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("unknown-user-id")).toBeInTheDocument();
    });
  });

  it("shows full access badge for * scope", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ scope: "*" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("full access")).toBeInTheDocument();
    });
  });

  it("shows readonly badge for readonly scope", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ scope: "readonly" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("readonly")).toBeInTheDocument();
    });
  });

  it("shows readwrite badge for readwrite scope", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ scope: "readwrite" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("readwrite")).toBeInTheDocument();
    });
  });

  it("shows allowed tables when restricted", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ allowedTables: ["posts", "comments"] })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("posts, comments")).toBeInTheDocument();
    });
  });

  it("does not show tables when allowedTables is null", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ allowedTables: null })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("full access")).toBeInTheDocument();
    });
    expect(screen.queryByText("posts")).not.toBeInTheDocument();
  });

  it("scope column header is visible", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Scope")).toBeInTheDocument();
    });
  });

  it("create form has scope selector defaulting to full access", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));

    const scopeSelect = screen.getByLabelText("Scope") as HTMLSelectElement;
    expect(scopeSelect.value).toBe("*");
  });

  it("create form sends readonly scope when selected", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_testkey",
      apiKey: makeKey({ id: "k-new", name: "Readonly Key", scope: "readonly" }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Readonly Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.selectOptions(screen.getByLabelText("Scope"), "readonly");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateApiKey).toHaveBeenCalledWith({
        userId: "u1",
        name: "Readonly Key",
        scope: "readonly",
      });
    });
  });

  it("create form sends allowed tables when specified", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_testkey",
      apiKey: makeKey({ id: "k-new", name: "Limited Key", scope: "readwrite", allowedTables: ["posts"] }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Limited Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.selectOptions(screen.getByLabelText("Scope"), "readwrite");
    await user.type(screen.getByLabelText("Allowed tables"), "posts");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateApiKey).toHaveBeenCalledWith({
        userId: "u1",
        name: "Limited Key",
        scope: "readwrite",
        allowedTables: ["posts"],
      });
    });
  });

  it("create form includes optional app selector", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    mockListApps.mockResolvedValueOnce(
      makeAppResponse([
        makeApp({ id: "a-orders", name: "Orders Service" }),
        makeApp({ id: "a-analytics", name: "Analytics" }),
      ]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));

    const appSelect = screen.getByLabelText("App Scope") as HTMLSelectElement;
    expect(appSelect.value).toBe("");
    expect(screen.getByRole("option", { name: "User-scoped (no app)" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Orders Service" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Analytics" })).toBeInTheDocument();
  });

  it("create form sends appId when app selected", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockListApps.mockResolvedValueOnce(
      makeAppResponse([makeApp({ id: "a-orders", name: "Orders Service" })]),
    );
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_testkey",
      apiKey: makeKey({ id: "k-new", name: "Orders Key", appId: "a-orders" }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Orders Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.selectOptions(screen.getByLabelText("App Scope"), "a-orders");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateApiKey).toHaveBeenCalledWith({
        userId: "u1",
        name: "Orders Key",
        scope: "*",
        appId: "a-orders",
      });
    });
  });

  it("shows app association on key rows", async () => {
    mockListApps.mockResolvedValueOnce(
      makeAppResponse([makeApp({ id: "a-orders", name: "Orders Service" })]),
    );
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ appId: "a-orders" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Orders Service")).toBeInTheDocument();
    });
  });

  it("shows configured app rate limit stats for app-scoped key", async () => {
    mockListApps.mockResolvedValueOnce(
      makeAppResponse([
        makeApp({ id: "a-orders", rateLimitRps: 120, rateLimitWindowSeconds: 60 }),
      ]),
    );
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ appId: "a-orders" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Rate: 120 req/60s")).toBeInTheDocument();
    });
  });

  it("shows User-scoped label for keys without app association", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ appId: null })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("User-scoped")).toBeInTheDocument();
    });
  });

  it("shows Rate: unlimited for app-scoped key with no rate limit", async () => {
    mockListApps.mockResolvedValueOnce(
      makeAppResponse([
        makeApp({ id: "a-free", name: "Free Tier", rateLimitRps: 0, rateLimitWindowSeconds: 0 }),
      ]),
    );
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ appId: "a-free" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Rate: unlimited")).toBeInTheDocument();
    });
  });

  it("shows app ID when app metadata is unavailable", async () => {
    mockListApps.mockResolvedValueOnce(makeAppResponse([]));
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ appId: "a-unknown" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("a-unknown")).toBeInTheDocument();
      // Should show "Rate: unknown" when app metadata is missing
      expect(screen.getByText("Rate: unknown")).toBeInTheDocument();
    });
  });

  it("created modal shows app name and rate limit for app-scoped key", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockListApps.mockResolvedValueOnce(
      makeAppResponse([
        makeApp({ id: "a-orders", name: "Orders Service", rateLimitRps: 120, rateLimitWindowSeconds: 60 }),
      ]),
    );
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_testkey",
      apiKey: makeKey({ id: "k-new", name: "App Key", appId: "a-orders" }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "App Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
      // The created modal should show the app name
      expect(screen.getByText("Orders Service")).toBeInTheDocument();
      // The created modal should show the rate limit (without the "Rate: " prefix per formatAppRateLimit stripping)
      expect(screen.getByText("120 req/60s")).toBeInTheDocument();
    });
  });

  it("created modal shows scope info", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_testkey",
      apiKey: makeKey({ id: "k-new", name: "Test Key", scope: "readonly" }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Test Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.selectOptions(screen.getByLabelText("Scope"), "readonly");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
      expect(screen.getByText(/readonly/)).toBeInTheDocument();
    });
  });

  it("created modal shows allowed tables when restricted", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_testkey",
      apiKey: makeKey({
        id: "k-new",
        name: "Table Key",
        scope: "readwrite",
        allowedTables: ["orders", "products"],
      }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Table Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
      expect(screen.getByText("orders, products")).toBeInTheDocument();
    });
  });
});
