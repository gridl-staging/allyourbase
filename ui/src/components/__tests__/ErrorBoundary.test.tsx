import { useLayoutEffect, type ReactNode } from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ErrorBoundary } from "../ErrorBoundary";
import { ThemeProvider } from "../ThemeProvider";
import { ToastProvider } from "../ToastProvider";

function StableChild() {
  return <div>stable child</div>;
}

function CrashAlways() {
  throw new Error("render crash");
}

let crashOnce = true;
function CrashOnceThenRecover() {
  useLayoutEffect(() => {
    if (crashOnce) {
      crashOnce = false;
      throw new Error("one-time crash");
    }
  }, []);

  return <div>recovered after retry</div>;
}

function ProviderChain({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider>
      <ToastProvider>{children}</ToastProvider>
    </ThemeProvider>
  );
}

const originalLocation = window.location;
let reloadMock: ReturnType<typeof vi.fn>;
let consoleErrorSpy: ReturnType<typeof vi.spyOn>;

function setWindowLocation(reload = originalLocation.reload) {
  Object.defineProperty(window, "location", {
    value: {
      ...originalLocation,
      reload,
    },
    writable: true,
  });
}

function renderBoundary(children: ReactNode) {
  render(<ErrorBoundary>{children}</ErrorBoundary>);
}

function expectCrashFallback() {
  expect(screen.getByRole("heading", { name: /something went wrong/i })).toBeVisible();
  expect(screen.getByRole("button", { name: /retry/i })).toBeVisible();
  expect(screen.getByRole("button", { name: /reload/i })).toBeVisible();
}

describe("ErrorBoundary", () => {
  beforeEach(() => {
    crashOnce = true;
    reloadMock = vi.fn();
    setWindowLocation(reloadMock);
    consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
    setWindowLocation();
    vi.restoreAllMocks();
  });

  it("renders descendants when nothing throws", () => {
    renderBoundary(<StableChild />);

    expect(screen.getByText("stable child")).toBeVisible();
  });

  it("shows a visible crash fallback with retry and reload actions", () => {
    renderBoundary(<CrashAlways />);

    expectCrashFallback();
    expect(screen.queryByText("stable child")).not.toBeInTheDocument();
  });

  it("retries by remounting children and leaving the fallback screen", async () => {
    const user = userEvent.setup();
    renderBoundary(<CrashOnceThenRecover />);

    await user.click(screen.getByRole("button", { name: /retry/i }));

    expect(screen.getByText("recovered after retry")).toBeVisible();
    expect(
      screen.queryByRole("heading", { name: /something went wrong/i }),
    ).not.toBeInTheDocument();
  });

  it("reload action triggers window.location.reload", async () => {
    const user = userEvent.setup();
    renderBoundary(<CrashAlways />);

    await user.click(screen.getByRole("button", { name: /reload/i }));

    expect(reloadMock).toHaveBeenCalledTimes(1);
  });

  it("catches crashes from the ThemeProvider -> ToastProvider subtree", () => {
    vi.spyOn(window.localStorage, "getItem").mockImplementation(() => {
      throw new Error("theme storage exploded");
    });

    renderBoundary(
      <ProviderChain>
        <div>dashboard child</div>
      </ProviderChain>,
    );

    expectCrashFallback();
  });
});
