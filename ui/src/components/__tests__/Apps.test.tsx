import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Apps } from "../Apps";
import { listApps, createApp, deleteApp, listUsers } from "../../api";
import type {
  AppResponse,
  AppListResponse,
  UserListResponse,
} from "../../types";

vi.mock("../../api", () => ({
  listApps: vi.fn(),
  createApp: vi.fn(),
  deleteApp: vi.fn(),
  listUsers: vi.fn(),
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

const mockListApps = vi.mocked(listApps);
const mockCreateApp = vi.mocked(createApp);
const mockDeleteApp = vi.mocked(deleteApp);
const mockListUsers = vi.mocked(listUsers);

function makeApp(overrides: Partial<AppResponse> = {}): AppResponse {
  return {
    id: "a1",
    name: "My App",
    description: "Test app",
    ownerUserId: "u1",
    rateLimitRps: 0,
    rateLimitWindowSeconds: 0,
    createdAt: "2026-02-21T00:00:00Z",
    updatedAt: "2026-02-21T00:00:00Z",
    ...overrides,
  };
}

function makeListResponse(
  apps: AppResponse[] = [],
  overrides: Partial<AppListResponse> = {},
): AppListResponse {
  return {
    items: apps,
    page: 1,
    perPage: 20,
    totalItems: apps.length,
    totalPages: apps.length > 0 ? 1 : 0,
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

describe("Apps", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListUsers.mockResolvedValue(makeUserResponse());
  });

  it("shows loading state", () => {
    mockListApps.mockReturnValue(new Promise(() => {}));
    render(<Apps />);
    expect(screen.getByText("Loading apps...")).toBeInTheDocument();
  });

  it("renders apps list", async () => {
    const apps = [
      makeApp({ id: "a1", name: "Frontend" }),
      makeApp({ id: "a2", name: "Backend API" }),
    ];
    mockListApps.mockResolvedValueOnce(makeListResponse(apps));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("Frontend")).toBeInTheDocument();
      expect(screen.getByText("Backend API")).toBeInTheDocument();
    });
  });

  it("shows empty state when no apps", async () => {
    mockListApps.mockResolvedValueOnce(makeListResponse([]));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("No apps registered yet")).toBeInTheDocument();
    });
  });

  it("shows error state with retry", async () => {
    mockListApps.mockRejectedValueOnce(new Error("connection refused"));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("connection refused")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });
  });

  it("shows total app count", async () => {
    const apps = [makeApp()];
    mockListApps.mockResolvedValueOnce(
      makeListResponse(apps, { totalItems: 1 }),
    );
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("1 app")).toBeInTheDocument();
    });
  });

  it("shows plural app count", async () => {
    const apps = [
      makeApp({ id: "a1", name: "App 1" }),
      makeApp({ id: "a2", name: "App 2" }),
    ];
    mockListApps.mockResolvedValueOnce(
      makeListResponse(apps, { totalItems: 2 }),
    );
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("2 apps")).toBeInTheDocument();
    });
  });

  it("shows rate limit when configured", async () => {
    mockListApps.mockResolvedValueOnce(
      makeListResponse([
        makeApp({ rateLimitRps: 100, rateLimitWindowSeconds: 60 }),
      ]),
    );
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("100 req/60s")).toBeInTheDocument();
    });
  });

  it("shows 'none' for zero rate limit", async () => {
    mockListApps.mockResolvedValueOnce(
      makeListResponse([makeApp({ rateLimitRps: 0 })]),
    );
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("none")).toBeInTheDocument();
    });
  });

  it("shows owner email for known users", async () => {
    mockListApps.mockResolvedValueOnce(
      makeListResponse([makeApp({ ownerUserId: "u1" })]),
    );
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });
  });

  it("create button opens create modal", async () => {
    mockListApps.mockResolvedValueOnce(makeListResponse([makeApp()]));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("My App")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create App"));

    expect(screen.getByText("Create Application")).toBeInTheDocument();
    expect(screen.getByLabelText("App name")).toBeInTheDocument();
  });

  it("create flow calls createApp and refreshes list", async () => {
    mockListApps.mockResolvedValue(makeListResponse([]));
    mockCreateApp.mockResolvedValueOnce(makeApp({ id: "a-new", name: "New App" }));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("No apps registered yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first app"));

    await user.type(screen.getByLabelText("App name"), "New App");
    await user.selectOptions(screen.getByLabelText("Owner"), "u1");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateApp).toHaveBeenCalledWith({
        name: "New App",
        description: "",
        ownerUserId: "u1",
      });
    });

    expect(mockListApps).toHaveBeenCalledTimes(2);
  });

  it("create button disabled when name is empty", async () => {
    mockListApps.mockResolvedValueOnce(makeListResponse([makeApp()]));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("My App")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create App"));

    await user.selectOptions(screen.getByLabelText("Owner"), "u1");
    const createBtn = screen.getByRole("button", { name: "Create" });
    expect(createBtn).toBeDisabled();
  });

  it("cancel on create modal closes it", async () => {
    mockListApps.mockResolvedValueOnce(makeListResponse([makeApp()]));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("My App")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create App"));
    expect(screen.getByText("Create Application")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Create Application")).not.toBeInTheDocument();
  });

  it("delete button opens confirmation dialog", async () => {
    mockListApps.mockResolvedValueOnce(makeListResponse([makeApp()]));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("My App")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete app"));

    expect(screen.getByText("Delete Application")).toBeInTheDocument();
    expect(
      screen.getByText(/This will permanently delete the application/),
    ).toBeInTheDocument();
  });

  it("confirming delete calls deleteApp and refreshes", async () => {
    mockListApps.mockResolvedValue(makeListResponse([makeApp()]));
    mockDeleteApp.mockResolvedValueOnce(undefined);
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("My App")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete app"));

    const dialog = screen
      .getByText("Delete Application")
      .closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", { name: "Delete" });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockDeleteApp).toHaveBeenCalledWith("a1");
    });

    expect(mockListApps.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("cancel on delete dialog closes it", async () => {
    mockListApps.mockResolvedValueOnce(makeListResponse([makeApp()]));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("My App")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete app"));
    expect(screen.getByText("Delete Application")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Delete Application")).not.toBeInTheDocument();
  });

  it("shows app description", async () => {
    mockListApps.mockResolvedValueOnce(
      makeListResponse([makeApp({ description: "Production frontend" })]),
    );
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("Production frontend")).toBeInTheDocument();
    });
  });

  it("shows app ID in the row", async () => {
    mockListApps.mockResolvedValueOnce(
      makeListResponse([makeApp({ id: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" })]),
    );
    render(<Apps />);

    await waitFor(() => {
      expect(
        screen.getByText("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
      ).toBeInTheDocument();
    });
  });

  it("retry button refetches apps after error", async () => {
    mockListApps.mockRejectedValueOnce(new Error("network down"));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("network down")).toBeInTheDocument();
    });

    mockListApps.mockResolvedValueOnce(makeListResponse([makeApp()]));
    const user = userEvent.setup();
    await user.click(screen.getByText("Retry"));

    await waitFor(() => {
      expect(screen.getByText("My App")).toBeInTheDocument();
    });
    expect(mockListApps).toHaveBeenCalledTimes(2);
  });

  it("displays page info for multi-page results", async () => {
    mockListApps.mockResolvedValueOnce(
      makeListResponse([makeApp()], {
        totalItems: 45,
        totalPages: 3,
        page: 1,
      }),
    );
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("45 apps")).toBeInTheDocument();
      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });
  });

  it("create form sends description when provided", async () => {
    mockListApps.mockResolvedValue(makeListResponse([]));
    mockCreateApp.mockResolvedValueOnce(makeApp({ name: "Desc App" }));
    render(<Apps />);

    await waitFor(() => {
      expect(screen.getByText("No apps registered yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first app"));
    await user.type(screen.getByLabelText("App name"), "Desc App");
    await user.type(screen.getByLabelText("Description"), "A great app");
    await user.selectOptions(screen.getByLabelText("Owner"), "u1");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateApp).toHaveBeenCalledWith({
        name: "Desc App",
        description: "A great app",
        ownerUserId: "u1",
      });
    });
  });
});
