import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { OAuthClients } from "../OAuthClients";
import {
  listOAuthClients,
  createOAuthClient,
  revokeOAuthClient,
  rotateOAuthClientSecret,
  listApps,
} from "../../api";
import type {
  OAuthClientResponse,
  OAuthClientListResponse,
  OAuthClientCreateResponse,
  AppResponse,
  AppListResponse,
} from "../../types";

vi.mock("../../api", () => ({
  listOAuthClients: vi.fn(),
  createOAuthClient: vi.fn(),
  revokeOAuthClient: vi.fn(),
  rotateOAuthClientSecret: vi.fn(),
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

const mockListOAuthClients = vi.mocked(listOAuthClients);
const mockCreateOAuthClient = vi.mocked(createOAuthClient);
const mockRevokeOAuthClient = vi.mocked(revokeOAuthClient);
const mockRotateOAuthClientSecret = vi.mocked(rotateOAuthClientSecret);
const mockListApps = vi.mocked(listApps);

function makeClient(
  overrides: Partial<OAuthClientResponse> = {},
): OAuthClientResponse {
  return {
    id: "c1-uuid",
    appId: "a1",
    clientId: "ayb_cid_abc123def456abc123def456abc123def456abc123def456",
    name: "My OAuth Client",
    redirectUris: ["https://example.com/callback"],
    scopes: ["readonly"],
    clientType: "confidential",
    createdAt: "2026-02-21T00:00:00Z",
    updatedAt: "2026-02-21T00:00:00Z",
    revokedAt: null,
    activeAccessTokenCount: 0,
    activeRefreshTokenCount: 0,
    totalGrants: 0,
    lastTokenIssuedAt: null,
    ...overrides,
  };
}

function makeListResponse(
  clients: OAuthClientResponse[] = [],
  overrides: Partial<OAuthClientListResponse> = {},
): OAuthClientListResponse {
  return {
    items: clients,
    page: 1,
    perPage: 20,
    totalItems: clients.length,
    totalPages: clients.length > 0 ? 1 : 0,
    ...overrides,
  };
}

function makeApp(overrides: Partial<AppResponse> = {}): AppResponse {
  return {
    id: "a1",
    name: "Frontend App",
    description: "The frontend",
    ownerUserId: "u1",
    rateLimitRps: 100,
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

describe("OAuthClients", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListApps.mockResolvedValue(makeAppResponse([makeApp()]));
  });

  // --- Loading / Error / Empty States ---

  it("shows loading state", () => {
    mockListApps.mockReturnValue(new Promise(() => {}));
    mockListOAuthClients.mockReturnValue(new Promise(() => {}));
    render(<OAuthClients />);
    expect(screen.getByText("Loading OAuth clients...")).toBeInTheDocument();
  });

  it("shows error state with retry", async () => {
    mockListOAuthClients.mockRejectedValueOnce(new Error("connection refused"));
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("connection refused")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });
  });

  it("shows empty state when no clients", async () => {
    mockListOAuthClients.mockResolvedValueOnce(makeListResponse([]));
    render(<OAuthClients />);

    await waitFor(() => {
      expect(
        screen.getByText("No OAuth clients registered yet"),
      ).toBeInTheDocument();
    });
  });

  it("retry button refetches after error", async () => {
    mockListOAuthClients.mockRejectedValueOnce(new Error("network down"));
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("network down")).toBeInTheDocument();
    });

    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient()]),
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Retry"));

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });
    expect(mockListOAuthClients).toHaveBeenCalledTimes(2);
  });

  // --- List Rendering ---

  it("renders clients list with name and client_id", async () => {
    const clients = [
      makeClient({ name: "Web App Client" }),
      makeClient({
        id: "c2-uuid",
        name: "Mobile Client",
        clientId: "ayb_cid_999888777666555444333222111000aabbccddeeff0011",
      }),
    ];
    mockListOAuthClients.mockResolvedValueOnce(makeListResponse(clients));
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("Web App Client")).toBeInTheDocument();
      expect(screen.getByText("Mobile Client")).toBeInTheDocument();
    });
  });

  it("shows client type badge", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient({ clientType: "confidential" })]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("confidential")).toBeInTheDocument();
    });
  });

  it("shows public client type badge", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient({ clientType: "public" })]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("public")).toBeInTheDocument();
    });
  });

  it("shows Active status for non-revoked clients", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient({ revokedAt: null })]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("Active")).toBeInTheDocument();
    });
  });

  it("shows Revoked status for revoked clients", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([
        makeClient({ revokedAt: "2026-02-22T00:00:00Z" }),
      ]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("Revoked")).toBeInTheDocument();
    });
  });

  it("shows linked app name", async () => {
    mockListApps.mockResolvedValueOnce(
      makeAppResponse([makeApp({ id: "a1", name: "Frontend App" })]),
    );
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient({ appId: "a1" })]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("Frontend App")).toBeInTheDocument();
    });
  });

  it("shows scopes in the list", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient({ scopes: ["readonly", "readwrite"] })]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("readonly, readwrite")).toBeInTheDocument();
    });
  });

  it("shows OAuth token stats per client", async () => {
    const client = {
      ...makeClient(),
      activeAccessTokenCount: 3,
      activeRefreshTokenCount: 1,
      totalGrants: 2,
      lastTokenIssuedAt: "2026-02-22T05:30:00Z",
    } as OAuthClientResponse & {
      activeAccessTokenCount: number;
      activeRefreshTokenCount: number;
      totalGrants: number;
      lastTokenIssuedAt: string | null;
    };

    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([client as unknown as OAuthClientResponse]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(
        screen.getByText("Access 3 / Refresh 1 / Grants 2"),
      ).toBeInTheDocument();
      expect(
        screen.getByText(
          `Last issued ${new Date("2026-02-22T05:30:00Z").toLocaleString()}`,
        ),
      ).toBeInTheDocument();
    });
  });

  it("shows total client count", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient()], { totalItems: 1 }),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("1 client")).toBeInTheDocument();
    });
  });

  it("shows plural client count", async () => {
    const clients = [
      makeClient({ id: "c1", name: "Client 1" }),
      makeClient({ id: "c2", name: "Client 2" }),
    ];
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse(clients, { totalItems: 2 }),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("2 clients")).toBeInTheDocument();
    });
  });

  it("displays page info for multi-page results", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient()], {
        totalItems: 45,
        totalPages: 3,
        page: 1,
      }),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("45 clients")).toBeInTheDocument();
      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });
  });

  it("shows redirect URIs in client row", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([
        makeClient({
          redirectUris: [
            "https://example.com/callback",
            "https://example.com/auth",
          ],
        }),
      ]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(
        screen.getByText("https://example.com/callback"),
      ).toBeInTheDocument();
    });
  });

  // --- Create Flow ---

  it("create button opens create modal", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient()]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Register Client"));

    expect(screen.getByText("Register OAuth Client")).toBeInTheDocument();
    expect(screen.getByLabelText("Client name")).toBeInTheDocument();
  });

  it("create flow calls createOAuthClient and shows secret", async () => {
    mockListOAuthClients.mockResolvedValue(makeListResponse([]));
    const created: OAuthClientCreateResponse = {
      clientSecret: "ayb_cs_secretvalue123456789",
      client: makeClient({ name: "New Client" }),
    };
    mockCreateOAuthClient.mockResolvedValueOnce(created);
    render(<OAuthClients />);

    await waitFor(() => {
      expect(
        screen.getByText("No OAuth clients registered yet"),
      ).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Register your first client"));

    await user.type(screen.getByLabelText("Client name"), "New Client");
    await user.selectOptions(screen.getByLabelText("App"), "a1");
    await user.type(
      screen.getByLabelText("Redirect URIs"),
      "https://example.com/callback",
    );

    await user.click(screen.getByRole("button", { name: "Register" }));

    await waitFor(() => {
      expect(mockCreateOAuthClient).toHaveBeenCalledWith({
        appId: "a1",
        name: "New Client",
        clientType: "confidential",
        redirectUris: ["https://example.com/callback"],
        scopes: ["readonly"],
      });
    });

    // Should show the secret modal
    await waitFor(() => {
      expect(screen.getByText("OAuth Client Registered")).toBeInTheDocument();
      expect(
        screen.getByText("ayb_cs_secretvalue123456789"),
      ).toBeInTheDocument();
      expect(
        screen.getByText(
          /Copy this secret now. It will not be shown again./,
        ),
      ).toBeInTheDocument();
    });
  });

  it("create button disabled when name is empty", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient()]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Register Client"));

    // Select app but leave name empty
    await user.selectOptions(screen.getByLabelText("App"), "a1");

    const registerBtn = screen.getByRole("button", { name: "Register" });
    expect(registerBtn).toBeDisabled();
  });

  it("cancel on create modal closes it", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient()]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Register Client"));
    expect(screen.getByText("Register OAuth Client")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(
      screen.queryByText("Register OAuth Client"),
    ).not.toBeInTheDocument();
  });

  it("done button on created modal closes it", async () => {
    mockListOAuthClients.mockResolvedValue(makeListResponse([]));
    mockCreateOAuthClient.mockResolvedValueOnce({
      clientSecret: "ayb_cs_test",
      client: makeClient({ name: "Test" }),
    });
    render(<OAuthClients />);

    await waitFor(() => {
      expect(
        screen.getByText("No OAuth clients registered yet"),
      ).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Register your first client"));
    await user.type(screen.getByLabelText("Client name"), "Test");
    await user.selectOptions(screen.getByLabelText("App"), "a1");
    await user.type(
      screen.getByLabelText("Redirect URIs"),
      "https://example.com/cb",
    );
    await user.click(screen.getByRole("button", { name: "Register" }));

    await waitFor(() => {
      expect(screen.getByText("OAuth Client Registered")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Done"));
    expect(
      screen.queryByText("OAuth Client Registered"),
    ).not.toBeInTheDocument();
  });

  it("create form has client type selector defaulting to confidential", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient()]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Register Client"));

    const typeSelect = screen.getByLabelText("Client type") as HTMLSelectElement;
    expect(typeSelect.value).toBe("confidential");
  });

  it("create form sends public client type when selected", async () => {
    mockListOAuthClients.mockResolvedValue(makeListResponse([]));
    mockCreateOAuthClient.mockResolvedValueOnce({
      clientSecret: "",
      client: makeClient({ clientType: "public" }),
    });
    render(<OAuthClients />);

    await waitFor(() => {
      expect(
        screen.getByText("No OAuth clients registered yet"),
      ).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Register your first client"));
    await user.type(screen.getByLabelText("Client name"), "SPA Client");
    await user.selectOptions(screen.getByLabelText("App"), "a1");
    await user.selectOptions(screen.getByLabelText("Client type"), "public");
    await user.type(
      screen.getByLabelText("Redirect URIs"),
      "https://spa.example.com/callback",
    );

    await user.click(screen.getByRole("button", { name: "Register" }));

    await waitFor(() => {
      expect(mockCreateOAuthClient).toHaveBeenCalledWith(
        expect.objectContaining({ clientType: "public" }),
      );
    });
  });

  it("create form has scope selector defaulting to readonly", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient()]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Register Client"));

    const scopeSelect = screen.getByLabelText("Scopes") as HTMLSelectElement;
    expect(scopeSelect.value).toBe("readonly");
  });

  // --- Revoke Flow ---

  it("revoke button opens confirmation dialog", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient()]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke client"));

    expect(screen.getByText("Revoke OAuth Client")).toBeInTheDocument();
    expect(
      screen.getByText(
        /This will revoke the OAuth client and invalidate all tokens/,
      ),
    ).toBeInTheDocument();
  });

  it("confirming revoke calls revokeOAuthClient and refreshes", async () => {
    mockListOAuthClients.mockResolvedValue(
      makeListResponse([makeClient()]),
    );
    mockRevokeOAuthClient.mockResolvedValueOnce(undefined);
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke client"));

    const dialog = screen
      .getByText("Revoke OAuth Client")
      .closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", { name: "Revoke" });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockRevokeOAuthClient).toHaveBeenCalledWith(
        "ayb_cid_abc123def456abc123def456abc123def456abc123def456",
      );
    });

    expect(mockListOAuthClients.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("cancel on revoke dialog closes it", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient()]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke client"));
    expect(screen.getByText("Revoke OAuth Client")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(
      screen.queryByText("Revoke OAuth Client"),
    ).not.toBeInTheDocument();
  });

  it("revoked clients don't show revoke or rotate buttons", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([
        makeClient({ revokedAt: "2026-02-22T00:00:00Z" }),
      ]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("Revoked")).toBeInTheDocument();
    });

    expect(screen.queryByTitle("Revoke client")).not.toBeInTheDocument();
    expect(screen.queryByTitle("Rotate secret")).not.toBeInTheDocument();
  });

  // --- Rotate Secret Flow ---

  it("rotate secret button opens confirmation and shows new secret", async () => {
    mockListOAuthClients.mockResolvedValue(
      makeListResponse([makeClient({ clientType: "confidential" })]),
    );
    mockRotateOAuthClientSecret.mockResolvedValueOnce({
      clientSecret: "ayb_cs_newsecret999",
    });
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Rotate secret"));

    // Confirmation dialog
    expect(screen.getByText("Rotate Client Secret")).toBeInTheDocument();

    const dialog = screen
      .getByText("Rotate Client Secret")
      .closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", {
      name: "Rotate",
    });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockRotateOAuthClientSecret).toHaveBeenCalledWith(
        "ayb_cid_abc123def456abc123def456abc123def456abc123def456",
      );
    });

    // Should show the new secret
    await waitFor(() => {
      expect(screen.getByText("New Client Secret")).toBeInTheDocument();
      expect(screen.getByText("ayb_cs_newsecret999")).toBeInTheDocument();
    });
  });

  it("public clients don't show rotate secret button", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient({ clientType: "public" })]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("My OAuth Client")).toBeInTheDocument();
    });

    expect(screen.queryByTitle("Rotate secret")).not.toBeInTheDocument();
  });

  // --- Client Details ---

  it("shows client_id in the row", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([
        makeClient({
          clientId:
            "ayb_cid_abc123def456abc123def456abc123def456abc123def456",
        }),
      ]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(
        screen.getByText(
          /ayb_cid_abc123def456/,
        ),
      ).toBeInTheDocument();
    });
  });

  it("shows created date", async () => {
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient({ createdAt: "2026-02-21T00:00:00Z" })]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      const expectedDate = new Date(
        "2026-02-21T00:00:00Z",
      ).toLocaleDateString();
      expect(screen.getByText(expectedDate)).toBeInTheDocument();
    });
  });

  it("shows app ID when app metadata is unavailable", async () => {
    mockListApps.mockResolvedValueOnce(makeAppResponse([]));
    mockListOAuthClients.mockResolvedValueOnce(
      makeListResponse([makeClient({ appId: "unknown-app-id" })]),
    );
    render(<OAuthClients />);

    await waitFor(() => {
      expect(screen.getByText("unknown-app-id")).toBeInTheDocument();
    });
  });

  // --- Created modal details ---

  it("created modal shows client_id", async () => {
    mockListOAuthClients.mockResolvedValue(makeListResponse([]));
    mockCreateOAuthClient.mockResolvedValueOnce({
      clientSecret: "ayb_cs_test",
      client: makeClient({
        clientId: "ayb_cid_newclientid123456789012345678901234567890ab",
      }),
    });
    render(<OAuthClients />);

    await waitFor(() => {
      expect(
        screen.getByText("No OAuth clients registered yet"),
      ).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Register your first client"));
    await user.type(screen.getByLabelText("Client name"), "Test");
    await user.selectOptions(screen.getByLabelText("App"), "a1");
    await user.type(
      screen.getByLabelText("Redirect URIs"),
      "https://example.com/cb",
    );
    await user.click(screen.getByRole("button", { name: "Register" }));

    await waitFor(() => {
      expect(screen.getByText("OAuth Client Registered")).toBeInTheDocument();
      expect(
        screen.getByText(
          "ayb_cid_newclientid123456789012345678901234567890ab",
        ),
      ).toBeInTheDocument();
    });
  });

  it("no secret shown for public client creation", async () => {
    mockListOAuthClients.mockResolvedValue(makeListResponse([]));
    mockCreateOAuthClient.mockResolvedValueOnce({
      clientSecret: "",
      client: makeClient({ clientType: "public" }),
    });
    render(<OAuthClients />);

    await waitFor(() => {
      expect(
        screen.getByText("No OAuth clients registered yet"),
      ).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Register your first client"));
    await user.type(screen.getByLabelText("Client name"), "SPA");
    await user.selectOptions(screen.getByLabelText("App"), "a1");
    await user.selectOptions(screen.getByLabelText("Client type"), "public");
    await user.type(
      screen.getByLabelText("Redirect URIs"),
      "https://spa.example.com/cb",
    );
    await user.click(screen.getByRole("button", { name: "Register" }));

    await waitFor(() => {
      expect(screen.getByText("OAuth Client Registered")).toBeInTheDocument();
    });

    // Secret section should not appear for public clients
    expect(
      screen.queryByText(/Copy this secret now/),
    ).not.toBeInTheDocument();
  });
});
