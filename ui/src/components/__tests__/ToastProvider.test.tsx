import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act, within, fireEvent } from "@testing-library/react";
import { ToastProvider, useAppToast } from "../ToastProvider";

// Helper component that exposes addToast for testing
function ToastTrigger({
  type,
  text,
  duration,
}: {
  type: "success" | "error" | "warning" | "info";
  text: string;
  duration?: number;
}) {
  const { addToast } = useAppToast();
  return (
    <button onClick={() => addToast(type, text, duration)}>
      trigger-{type}
    </button>
  );
}

function MultiTrigger() {
  const { addToast } = useAppToast();
  return (
    <button
      onClick={() => {
        addToast("success", "First toast");
        addToast("error", "Second toast");
        addToast("warning", "Third toast");
      }}
    >
      trigger-multi
    </button>
  );
}

describe("ToastProvider", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders success variant with correct styling and icon", () => {
    render(
      <ToastProvider>
        <ToastTrigger type="success" text="Operation succeeded" />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByText("trigger-success"));

    const toast = screen.getByTestId("toast");
    expect(toast).toHaveTextContent("Operation succeeded");
    expect(toast.className).toMatch(/green/);
  });

  it("renders error variant with correct styling", () => {
    render(
      <ToastProvider>
        <ToastTrigger type="error" text="Something failed" />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByText("trigger-error"));

    const toast = screen.getByTestId("toast");
    expect(toast).toHaveTextContent("Something failed");
    expect(toast.className).toMatch(/red/);
  });

  it("renders warning variant with correct styling", () => {
    render(
      <ToastProvider>
        <ToastTrigger type="warning" text="Be careful" />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByText("trigger-warning"));

    const toast = screen.getByTestId("toast");
    expect(toast).toHaveTextContent("Be careful");
    expect(toast.className).toMatch(/amber/);
  });

  it("renders info variant with correct styling", () => {
    render(
      <ToastProvider>
        <ToastTrigger type="info" text="FYI note" />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByText("trigger-info"));

    const toast = screen.getByTestId("toast");
    expect(toast).toHaveTextContent("FYI note");
    expect(toast.className).toMatch(/blue/);
  });

  it("stacks multiple toasts in queue order", () => {
    render(
      <ToastProvider>
        <MultiTrigger />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByText("trigger-multi"));

    const toasts = screen.getAllByTestId("toast");
    expect(toasts).toHaveLength(3);
    expect(toasts[0]).toHaveTextContent("First toast");
    expect(toasts[1]).toHaveTextContent("Second toast");
    expect(toasts[2]).toHaveTextContent("Third toast");
  });

  it("auto-dismisses after default 4s", () => {
    render(
      <ToastProvider>
        <ToastTrigger type="success" text="Auto dismiss me" />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByText("trigger-success"));
    expect(screen.getByTestId("toast")).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(4100);
    });
    expect(screen.queryByTestId("toast")).not.toBeInTheDocument();
  });

  it("supports configurable auto-dismiss duration", () => {
    render(
      <ToastProvider>
        <ToastTrigger type="info" text="Long toast" duration={8000} />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByText("trigger-info"));
    expect(screen.getByTestId("toast")).toBeInTheDocument();

    // Should still be visible after 4s (the old default)
    act(() => {
      vi.advanceTimersByTime(4100);
    });
    expect(screen.getByTestId("toast")).toBeInTheDocument();

    // Should be gone after 8s
    act(() => {
      vi.advanceTimersByTime(4000);
    });
    expect(screen.queryByTestId("toast")).not.toBeInTheDocument();
  });

  it("removes toast on manual close click", () => {
    render(
      <ToastProvider>
        <ToastTrigger type="error" text="Close me" />
      </ToastProvider>,
    );
    fireEvent.click(screen.getByText("trigger-error"));

    const toast = screen.getByTestId("toast");
    expect(toast).toBeInTheDocument();

    const closeBtn = within(toast).getByRole("button");
    fireEvent.click(closeBtn);

    expect(screen.queryByTestId("toast")).not.toBeInTheDocument();
  });

  it("throws when useAppToast is used outside ToastProvider", () => {
    function Bad() {
      useAppToast();
      return null;
    }
    expect(() => render(<Bad />)).toThrow(
      "useAppToast must be used within a ToastProvider",
    );
  });
});
