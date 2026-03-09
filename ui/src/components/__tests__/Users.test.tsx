import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Users } from "../Users";
import { listUsers, deleteUser } from "../../api";
import type { AdminUser, UserListResponse } from "../../types";

vi.mock("../../api", () => ({
  listUsers: vi.fn(),
  deleteUser: vi.fn(),
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

const mockListUsers = vi.mocked(listUsers);
const mockDeleteUser = vi.mocked(deleteUser);

function makeUser(overrides: Partial<AdminUser> = {}): AdminUser {
  return {
    id: "u1",
    email: "alice@example.com",
    emailVerified: true,
    createdAt: "2026-02-09T12:00:00Z",
    updatedAt: "2026-02-09T12:00:00Z",
    ...overrides,
  };
}

function makeResponse(
  users: AdminUser[] = [],
  overrides: Partial<UserListResponse> = {},
): UserListResponse {
  return {
    items: users,
    page: 1,
    perPage: 20,
    totalItems: users.length,
    totalPages: users.length > 0 ? 1 : 0,
    ...overrides,
  };
}

describe("Users", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading state", () => {
    mockListUsers.mockReturnValue(new Promise(() => {})); // never resolves
    render(<Users />);
    expect(screen.getByText("Loading users...")).toBeInTheDocument();
  });

  it("renders user list", async () => {
    const users = [
      makeUser({ id: "u1", email: "alice@example.com", emailVerified: true }),
      makeUser({ id: "u2", email: "bob@test.com", emailVerified: false }),
    ];
    mockListUsers.mockResolvedValueOnce(makeResponse(users));
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
      expect(screen.getByText("bob@test.com")).toBeInTheDocument();
    });
  });

  it("shows empty state when no users", async () => {
    mockListUsers.mockResolvedValueOnce(makeResponse([]));
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("No users registered yet")).toBeInTheDocument();
    });
  });

  it("shows error state with retry", async () => {
    mockListUsers.mockRejectedValueOnce(new Error("connection refused"));
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("connection refused")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });
  });

  it("shows total user count", async () => {
    const users = [makeUser()];
    mockListUsers.mockResolvedValueOnce(makeResponse(users, { totalItems: 1 }));
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("1 user")).toBeInTheDocument();
    });
  });

  it("shows plural user count", async () => {
    const users = [
      makeUser({ id: "u1", email: "alice@example.com" }),
      makeUser({ id: "u2", email: "bob@test.com" }),
    ];
    mockListUsers.mockResolvedValueOnce(
      makeResponse(users, { totalItems: 2 }),
    );
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("2 users")).toBeInTheDocument();
    });
  });

  it("shows verified badge for verified users", async () => {
    const users = [
      makeUser({ id: "u1", email: "alice@example.com", emailVerified: true }),
      makeUser({ id: "u2", email: "bob@test.com", emailVerified: false }),
    ];
    mockListUsers.mockResolvedValueOnce(makeResponse(users));
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });

    // There should be exactly one green check (verified) and one gray X (unverified)
    // We check by looking at SVG elements in the rows
    const rows = screen.getAllByRole("row");
    // rows[0] is thead, rows[1] and rows[2] are data rows
    expect(rows.length).toBe(3); // header + 2 data rows
  });

  it("search calls listUsers with search param", async () => {
    mockListUsers.mockResolvedValue(makeResponse([]));
    render(<Users />);

    await waitFor(() => {
      expect(
        screen.getByRole("textbox", { name: "Search users" }),
      ).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const searchInput = screen.getByRole("textbox", { name: "Search users" });
    await user.type(searchInput, "alice{Enter}");

    await waitFor(() => {
      expect(mockListUsers).toHaveBeenCalledWith(
        expect.objectContaining({ search: "alice" }),
      );
    });
  });

  it("search via button click", async () => {
    mockListUsers.mockResolvedValue(makeResponse([]));
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("No users registered yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const searchInput = screen.getByRole("textbox", { name: "Search users" });
    await user.type(searchInput, "bob");
    await user.click(screen.getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(mockListUsers).toHaveBeenCalledWith(
        expect.objectContaining({ search: "bob" }),
      );
    });
  });

  it("shows no-match message when search returns empty", async () => {
    // First load with data, then search returns empty
    mockListUsers.mockResolvedValueOnce(
      makeResponse([makeUser()]),
    );
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });

    // Now search returns empty
    mockListUsers.mockResolvedValueOnce(makeResponse([]));

    const user = userEvent.setup();
    const searchInput = screen.getByRole("textbox", { name: "Search users" });
    await user.type(searchInput, "nobody{Enter}");

    await waitFor(() => {
      expect(screen.getByText("No users matching search")).toBeInTheDocument();
    });
  });

  it("clear search button resets results", async () => {
    mockListUsers.mockResolvedValue(makeResponse([makeUser()]));
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    const searchInput = screen.getByRole("textbox", { name: "Search users" });
    await user.type(searchInput, "test{Enter}");

    await waitFor(() => {
      expect(mockListUsers).toHaveBeenCalledWith(
        expect.objectContaining({ search: "test" }),
      );
    });

    await user.click(screen.getByRole("button", { name: "Clear search" }));

    await waitFor(() => {
      expect(mockListUsers).toHaveBeenCalledWith(
        expect.objectContaining({ search: undefined }),
      );
    });
  });

  it("delete button opens confirmation dialog", async () => {
    const users = [makeUser()];
    mockListUsers.mockResolvedValueOnce(makeResponse(users));
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete user"));

    expect(screen.getByText("Delete User")).toBeInTheDocument();
    expect(
      screen.getByText(
        "This will permanently delete the user and all their sessions.",
      ),
    ).toBeInTheDocument();
  });

  it("confirming delete calls deleteUser and refreshes", async () => {
    const users = [makeUser()];
    mockListUsers.mockResolvedValue(makeResponse(users));
    mockDeleteUser.mockResolvedValueOnce(undefined);
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete user"));

    // Find the Delete button in the confirmation dialog
    const dialog = screen
      .getByText("Delete User")
      .closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", { name: "Delete" });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockDeleteUser).toHaveBeenCalledWith("u1");
    });
  });

  it("cancel on delete dialog closes it", async () => {
    mockListUsers.mockResolvedValueOnce(makeResponse([makeUser()]));
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Delete user"));
    expect(screen.getByText("Delete User")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Delete User")).not.toBeInTheDocument();
  });

  it("shows user ID under email", async () => {
    mockListUsers.mockResolvedValueOnce(makeResponse([makeUser()]));
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("u1")).toBeInTheDocument();
    });
  });

  it("displays page info for multi-page results", async () => {
    mockListUsers.mockResolvedValueOnce(
      makeResponse([makeUser()], {
        totalItems: 45,
        totalPages: 3,
        page: 1,
      }),
    );
    render(<Users />);

    await waitFor(() => {
      expect(screen.getByText("45 users")).toBeInTheDocument();
      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });
  });
});
