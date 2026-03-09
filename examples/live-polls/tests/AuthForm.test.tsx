import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import AuthForm from "../src/components/AuthForm";
import { ayb, persistTokens } from "../src/lib/ayb";

vi.mock("../src/lib/ayb", () => ({
  ayb: {
    auth: {
      login: vi.fn(),
      register: vi.fn(),
    },
  },
  persistTokens: vi.fn(),
}));

const mockLogin = vi.mocked(ayb.auth.login);
const mockRegister = vi.mocked(ayb.auth.register);
const mockPersistTokens = vi.mocked(persistTokens);

describe("AuthForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // ── Rendering ──────────────────────────────────────────────────────────────

  it("renders in login mode by default", () => {
    render(<AuthForm onAuth={vi.fn()} />);
    expect(screen.getByPlaceholderText("Email")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Password")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign In" })).toBeInTheDocument();
  });

  it("shows demo accounts in login mode", () => {
    render(<AuthForm onAuth={vi.fn()} />);
    expect(screen.getByText("alice@demo.test")).toBeInTheDocument();
    expect(screen.getByText("bob@demo.test")).toBeInTheDocument();
    expect(screen.getByText("charlie@demo.test")).toBeInTheDocument();
  });

  it("can switch to register mode via the Register link", () => {
    render(<AuthForm onAuth={vi.fn()} />);
    fireEvent.click(screen.getByText("Register"));
    expect(screen.getByRole("button", { name: "Create Account" })).toBeInTheDocument();
    // Demo accounts must NOT be shown in register mode.
    expect(screen.queryByText("alice@demo.test")).not.toBeInTheDocument();
  });

  it("can switch back to login mode from register mode", () => {
    render(<AuthForm onAuth={vi.fn()} />);
    fireEvent.click(screen.getByText("Register"));
    fireEvent.click(screen.getByText("Sign in"));
    expect(screen.getByRole("button", { name: "Sign In" })).toBeInTheDocument();
  });

  // ── Demo account prefill ──────────────────────────────────────────────────

  it("clicking a demo account fills email and password fields", () => {
    render(<AuthForm onAuth={vi.fn()} />);
    fireEvent.click(screen.getByText("alice@demo.test"));
    expect(screen.getByPlaceholderText("Email")).toHaveValue("alice@demo.test");
    expect(screen.getByPlaceholderText("Password")).toHaveValue("password123");
  });

  it("clicking a demo account clears any previous error", async () => {
    mockLogin.mockRejectedValueOnce(new Error("Bad credentials"));
    render(<AuthForm onAuth={vi.fn()} />);

    // Trigger an error first.
    fireEvent.change(screen.getByPlaceholderText("Email"), {
      target: { value: "wrong@test.com" },
    });
    fireEvent.change(screen.getByPlaceholderText("Password"), {
      target: { value: "badpassword" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "Sign In" }).closest("form")!);
    await screen.findByText("Bad credentials");

    // Clicking a demo account should clear the error.
    fireEvent.click(screen.getByText("bob@demo.test"));
    expect(screen.queryByText("Bad credentials")).not.toBeInTheDocument();
  });

  // ── Login ─────────────────────────────────────────────────────────────────

  it("calls ayb.auth.login with email and password on submit", async () => {
    mockLogin.mockResolvedValueOnce(undefined as never);
    render(<AuthForm onAuth={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText("Email"), {
      target: { value: "user@test.com" },
    });
    fireEvent.change(screen.getByPlaceholderText("Password"), {
      target: { value: "password123" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "Sign In" }).closest("form")!);

    await waitFor(() => expect(mockLogin).toHaveBeenCalledOnce());
    expect(mockLogin).toHaveBeenCalledWith("user@test.com", "password123");
  });

  it("calls persistTokens and onAuth after successful login", async () => {
    mockLogin.mockResolvedValueOnce(undefined as never);
    const onAuth = vi.fn();
    render(<AuthForm onAuth={onAuth} />);

    fireEvent.change(screen.getByPlaceholderText("Email"), {
      target: { value: "user@test.com" },
    });
    fireEvent.change(screen.getByPlaceholderText("Password"), {
      target: { value: "password123" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "Sign In" }).closest("form")!);

    await waitFor(() => expect(onAuth).toHaveBeenCalledOnce());
    expect(mockPersistTokens).toHaveBeenCalledOnce();
    // persistTokens must run before onAuth so tokens are saved before the app
    // reads them on the next render.
    expect(mockPersistTokens.mock.invocationCallOrder[0]).toBeLessThan(
      onAuth.mock.invocationCallOrder[0],
    );
  });

  it("shows an error message when login fails", async () => {
    mockLogin.mockRejectedValueOnce(new Error("Invalid email or password"));
    render(<AuthForm onAuth={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText("Email"), {
      target: { value: "user@test.com" },
    });
    fireEvent.change(screen.getByPlaceholderText("Password"), {
      target: { value: "wrongpassword" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "Sign In" }).closest("form")!);

    expect(await screen.findByText("Invalid email or password")).toBeInTheDocument();
  });

  it("does not call onAuth when login fails", async () => {
    mockLogin.mockRejectedValueOnce(new Error("Server error"));
    const onAuth = vi.fn();
    render(<AuthForm onAuth={onAuth} />);

    fireEvent.change(screen.getByPlaceholderText("Email"), {
      target: { value: "user@test.com" },
    });
    fireEvent.change(screen.getByPlaceholderText("Password"), {
      target: { value: "password" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "Sign In" }).closest("form")!);

    await screen.findByText("Server error");
    expect(onAuth).not.toHaveBeenCalled();
  });

  // ── Register ──────────────────────────────────────────────────────────────

  it("calls ayb.auth.register (not login) in register mode", async () => {
    mockRegister.mockResolvedValueOnce(undefined as never);
    render(<AuthForm onAuth={vi.fn()} />);

    fireEvent.click(screen.getByText("Register"));

    fireEvent.change(screen.getByPlaceholderText("Email"), {
      target: { value: "new@test.com" },
    });
    fireEvent.change(screen.getByPlaceholderText("Password"), {
      target: { value: "password123" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "Create Account" }).closest("form")!);

    await waitFor(() => expect(mockRegister).toHaveBeenCalledOnce());
    expect(mockRegister).toHaveBeenCalledWith("new@test.com", "password123");
    expect(mockLogin).not.toHaveBeenCalled();
  });

  it("shows an error message when registration fails", async () => {
    mockRegister.mockRejectedValueOnce(new Error("Email already taken"));
    render(<AuthForm onAuth={vi.fn()} />);

    fireEvent.click(screen.getByText("Register"));
    fireEvent.change(screen.getByPlaceholderText("Email"), {
      target: { value: "existing@test.com" },
    });
    fireEvent.change(screen.getByPlaceholderText("Password"), {
      target: { value: "password123" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "Create Account" }).closest("form")!);

    expect(await screen.findByText("Email already taken")).toBeInTheDocument();
  });
});
