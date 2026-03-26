import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { UserSearchCombobox } from "../shared/UserSearchCombobox";
import { listUsers } from "../../api_admin";
import type { AdminUser, UserListResponse } from "../../types";

vi.mock("../../api_admin", () => ({
  listUsers: vi.fn(),
}));

const mockListUsers = vi.mocked(listUsers);
const SEARCH_DEBOUNCE_MS = 300;

function makeUser(overrides: Partial<AdminUser> = {}): AdminUser {
  return {
    id: "usr_1234567890",
    email: "alice@example.com",
    emailVerified: true,
    createdAt: "2026-03-01T00:00:00Z",
    updatedAt: "2026-03-01T00:00:00Z",
    ...overrides,
  };
}

function makeResponse(
  items: AdminUser[],
  overrides: Partial<UserListResponse> = {},
): UserListResponse {
  return {
    items,
    page: 1,
    perPage: 10,
    totalItems: items.length,
    totalPages: items.length > 0 ? 1 : 0,
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function ControlledCombobox({
  initialValue = "",
  placeholder,
  id,
  ariaLabel,
  onValueChange,
}: {
  initialValue?: string;
  placeholder?: string;
  id?: string;
  ariaLabel?: string;
  onValueChange?: (nextValue: string) => void;
}) {
  const [value, setValue] = useState(initialValue);
  return (
    <UserSearchCombobox
      value={value}
      onChange={(nextValue) => {
        setValue(nextValue);
        onValueChange?.(nextValue);
      }}
      placeholder={placeholder}
      id={id}
      aria-label={ariaLabel}
    />
  );
}

describe("UserSearchCombobox", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("passes raw typed values through onChange without requiring selection", async () => {
    const onValueChange = vi.fn();
    const user = userEvent.setup();

    render(<ControlledCombobox ariaLabel="Owner User ID" onValueChange={onValueChange} />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });
    await user.type(combobox, "manual-user-id");

    expect(onValueChange).toHaveBeenLastCalledWith("manual-user-id");
  });

  it("debounces listUsers calls with search and perPage=10", async () => {
    mockListUsers.mockResolvedValue(makeResponse([makeUser()]));
    const user = userEvent.setup();

    render(<ControlledCombobox ariaLabel="Owner User ID" />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });
    await user.type(combobox, "alice");

    expect(mockListUsers).not.toHaveBeenCalled();

    await waitFor(() => {
      expect(mockListUsers).toHaveBeenCalledWith({ search: "alice", perPage: 10 });
    });
  });

  it("skips network calls for empty or whitespace-only query", async () => {
    mockListUsers.mockResolvedValue(makeResponse([makeUser()]));
    const user = userEvent.setup();

    render(<ControlledCombobox ariaLabel="Owner User ID" />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });
    await user.type(combobox, "   ");

    expect(mockListUsers).not.toHaveBeenCalled();
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("shows error feedback and clears loading when search request fails", async () => {
    mockListUsers.mockRejectedValue(new Error("search failed"));
    const user = userEvent.setup();

    render(<ControlledCombobox ariaLabel="Owner User ID" />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });
    await user.type(combobox, "alice");

    await waitFor(() => {
      expect(screen.getByText("Unable to load users")).toBeInTheDocument();
    });

    expect(screen.queryByText("Searching users...")).not.toBeInTheDocument();
  });

  it("ignores stale search responses and keeps latest result set", async () => {
    const first = deferred<UserListResponse>();
    const second = deferred<UserListResponse>();

    mockListUsers
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise);

    const user = userEvent.setup();

    render(<ControlledCombobox ariaLabel="Owner User ID" />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });

    await user.type(combobox, "alice");
    await waitFor(() => {
      expect(mockListUsers).toHaveBeenCalledTimes(1);
    });

    await user.clear(combobox);
    await user.type(combobox, "bob");
    await waitFor(() => {
      expect(mockListUsers).toHaveBeenCalledTimes(2);
    });

    second.resolve(makeResponse([makeUser({ id: "usr_bob_12345", email: "bob@example.com" })]));
    await waitFor(() => {
      expect(screen.getByText("bob@example.com")).toBeInTheDocument();
    });

    first.resolve(makeResponse([makeUser({ id: "usr_alice_12345", email: "alice@example.com" })]));

    await waitFor(() => {
      expect(screen.queryByText("alice@example.com")).not.toBeInTheDocument();
    });
  });

  it("ignores stale responses as soon as the input changes", async () => {
    vi.useFakeTimers();
    const first = deferred<UserListResponse>();
    const second = deferred<UserListResponse>();

    mockListUsers
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise);

    render(<ControlledCombobox ariaLabel="Owner User ID" />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });
    fireEvent.change(combobox, { target: { value: "alice" } });
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);

    expect(mockListUsers).toHaveBeenNthCalledWith(1, { search: "alice", perPage: 10 });

    fireEvent.change(combobox, { target: { value: "bob" } });

    await act(async () => {
      first.resolve(makeResponse([makeUser({ id: "usr_alice_12345", email: "alice@example.com" })]));
    });

    expect(screen.queryByText("alice@example.com")).not.toBeInTheDocument();

    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
    expect(mockListUsers).toHaveBeenNthCalledWith(2, { search: "bob", perPage: 10 });

    await act(async () => {
      second.resolve(makeResponse([makeUser({ id: "usr_bob_12345", email: "bob@example.com" })]));
    });

    expect(screen.getByText("bob@example.com")).toBeInTheDocument();
  });

  it("wires combobox and listbox roles and supports ArrowDown + Enter selection", async () => {
    mockListUsers.mockResolvedValue(
      makeResponse([
        makeUser({ id: "usr_alice_12345", email: "alice@example.com" }),
        makeUser({ id: "usr_bob_12345", email: "bob@example.com" }),
      ]),
    );
    const user = userEvent.setup();

    render(<ControlledCombobox ariaLabel="Owner User ID" />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });
    await user.type(combobox, "a");

    const aliceOption = await screen.findByText("alice@example.com");
    const listbox = aliceOption.closest('[role="listbox"]');
    expect(listbox).not.toBeNull();
    if (!listbox) {
      throw new Error("Expected listbox container");
    }
    expect(combobox).toHaveAttribute("aria-controls", listbox.getAttribute("id"));
    expect(combobox).toHaveAttribute("aria-expanded", "true");

    await user.type(combobox, "{ArrowDown}{ArrowDown}{Enter}");

    await waitFor(() => {
      expect(combobox).toHaveValue("usr_bob_12345");
    });
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("closes popup on Escape", async () => {
    mockListUsers.mockResolvedValue(makeResponse([makeUser()]));
    const user = userEvent.setup();

    render(<ControlledCombobox ariaLabel="Owner User ID" />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });
    await user.type(combobox, "alice");

    await screen.findByRole("listbox");
    await user.type(combobox, "{Escape}");

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(combobox).toHaveAttribute("aria-expanded", "false");
  });

  it("reopens dismissed results with ArrowDown and allows keyboard selection", async () => {
    mockListUsers.mockResolvedValue(
      makeResponse([
        makeUser({ id: "usr_alice_12345", email: "alice@example.com" }),
        makeUser({ id: "usr_bob_12345", email: "bob@example.com" }),
      ]),
    );
    const user = userEvent.setup();

    render(<ControlledCombobox ariaLabel="Owner User ID" />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });
    await user.type(combobox, "a");

    await screen.findByText("alice@example.com");
    expect(mockListUsers).toHaveBeenCalledTimes(1);
    await user.type(combobox, "{Escape}");

    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();

    fireEvent.keyDown(combobox, { key: "ArrowDown" });

    await waitFor(() => {
      expect(screen.getByRole("listbox")).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getAllByRole("option")[0]).toHaveAttribute("aria-selected", "true");
    });
    fireEvent.keyDown(combobox, { key: "Enter" });

    await waitFor(() => {
      expect(combobox).toHaveValue("usr_alice_12345");
    });
    expect(mockListUsers).toHaveBeenCalledTimes(1);
  });

  it("reopens dismissed results with ArrowUp and selects the last cached option", async () => {
    mockListUsers.mockResolvedValue(
      makeResponse([
        makeUser({ id: "usr_alice_12345", email: "alice@example.com" }),
        makeUser({ id: "usr_bob_12345", email: "bob@example.com" }),
      ]),
    );
    const user = userEvent.setup();

    render(<ControlledCombobox ariaLabel="Owner User ID" />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });
    await user.type(combobox, "a");

    await screen.findByText("alice@example.com");
    expect(mockListUsers).toHaveBeenCalledTimes(1);
    await user.type(combobox, "{Escape}");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();

    fireEvent.keyDown(combobox, { key: "ArrowUp" });

    await waitFor(() => {
      expect(screen.getByRole("listbox")).toBeInTheDocument();
    });
    await waitFor(() => {
      const options = screen.getAllByRole("option");
      expect(options[1]).toHaveAttribute("aria-selected", "true");
    });
    fireEvent.keyDown(combobox, { key: "Enter" });

    await waitFor(() => {
      expect(combobox).toHaveValue("usr_bob_12345");
    });
    expect(mockListUsers).toHaveBeenCalledTimes(1);
  });

  it("shows loading, empty, and failed-search states", async () => {
    const pending = deferred<UserListResponse>();
    mockListUsers.mockImplementationOnce(() => pending.promise);

    const user = userEvent.setup();
    render(<ControlledCombobox ariaLabel="Owner User ID" />);

    const combobox = screen.getByRole("combobox", { name: "Owner User ID" });
    await user.type(combobox, "alice");

    const listbox = await screen.findByRole("listbox");
    expect(combobox).toHaveAttribute("aria-controls", listbox.getAttribute("id"));
    expect(within(listbox).getByText("Searching users...")).toBeInTheDocument();

    pending.resolve(makeResponse([]));

    await waitFor(() => {
      expect(within(screen.getByRole("listbox")).getByText("No users found")).toBeInTheDocument();
    });

    mockListUsers.mockRejectedValueOnce(new Error("boom"));
    await user.clear(combobox);
    await user.type(combobox, "bob");

    await waitFor(() => {
      expect(within(screen.getByRole("listbox")).getByText("Unable to load users")).toBeInTheDocument();
    });
  });
});
