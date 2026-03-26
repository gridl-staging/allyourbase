import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { RowDetail } from "../TableBrowserDialogs";

describe("RowDetail", () => {
  const defaultProps = {
    row: { id: "1", name: "Alice" } as Record<string, unknown>,
    columns: [
      { name: "id", type: "integer" },
      { name: "name", type: "text" },
    ],
    expandColumns: [],
    isWritable: true,
    onClose: vi.fn(),
    onEdit: vi.fn(),
    onDelete: vi.fn(),
  };

  it("closes on Escape key press", async () => {
    const onClose = vi.fn();
    render(<RowDetail {...defaultProps} onClose={onClose} />);

    expect(screen.getByText("Row Detail")).toBeInTheDocument();

    const user = userEvent.setup();
    await user.keyboard("{Escape}");

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("renders row data", () => {
    render(<RowDetail {...defaultProps} />);

    expect(screen.getByText("Row Detail")).toBeInTheDocument();
    expect(screen.getByText("Alice")).toBeInTheDocument();
  });
});
