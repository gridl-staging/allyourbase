import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { CommandPalette, CommandPaletteHint } from "../CommandPalette";
import type { CommandAction } from "../CommandPalette";
import type { Table } from "../../types";

function makeTable(name: string, schema = "public"): Table {
  return {
    name,
    schema,
    kind: "table",
    columns: [],
    primaryKey: ["id"],
  };
}

const tables = [
  makeTable("users"),
  makeTable("posts"),
  makeTable("comments"),
  makeTable("audit_log", "internal"),
];

describe("CommandPalette", () => {
  const onClose = vi.fn();
  const onSelect = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not render when closed", () => {
    render(
      <CommandPalette open={false} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("renders when open with search input and all items", () => {
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Search tables, pages...")).toBeInTheDocument();
    // All 4 tables + 13 navigation items
    expect(screen.getByText("users")).toBeInTheDocument();
    expect(screen.getByText("posts")).toBeInTheDocument();
    expect(screen.getByText("SQL Editor")).toBeInTheDocument();
    expect(screen.getByText("API Explorer")).toBeInTheDocument();
  });

  it("includes SMS Health and SMS Messages in navigation items", () => {
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );
    expect(screen.getByText("SMS Health")).toBeInTheDocument();
    expect(screen.getByText("SMS Messages")).toBeInTheDocument();
  });

  it("includes Jobs and Schedules in navigation items", () => {
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );
    expect(screen.getByText("Jobs")).toBeInTheDocument();
    expect(screen.getByText("Schedules")).toBeInTheDocument();
  });

  it("includes Email Templates in navigation items", () => {
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );
    expect(screen.getByText("Email Templates")).toBeInTheDocument();
  });

  it("calls onSelect with sms-health view when SMS Health clicked", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    await user.click(screen.getByText("SMS Health"));

    expect(onSelect).toHaveBeenCalledOnce();
    const action: CommandAction = onSelect.mock.calls[0][0];
    expect(action.kind).toBe("view");
    if (action.kind === "view") {
      expect(action.view).toBe("sms-health");
    }
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("calls onSelect with sms-messages view when SMS Messages clicked", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    await user.click(screen.getByText("SMS Messages"));

    expect(onSelect).toHaveBeenCalledOnce();
    const action: CommandAction = onSelect.mock.calls[0][0];
    expect(action.kind).toBe("view");
    if (action.kind === "view") {
      expect(action.view).toBe("sms-messages");
    }
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("calls onSelect with jobs view when Jobs clicked", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    await user.click(screen.getByText("Jobs"));

    expect(onSelect).toHaveBeenCalledOnce();
    const action: CommandAction = onSelect.mock.calls[0][0];
    expect(action.kind).toBe("view");
    if (action.kind === "view") {
      expect(action.view).toBe("jobs");
    }
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("calls onSelect with schedules view when Schedules clicked", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    await user.click(screen.getByText("Schedules"));

    expect(onSelect).toHaveBeenCalledOnce();
    const action: CommandAction = onSelect.mock.calls[0][0];
    expect(action.kind).toBe("view");
    if (action.kind === "view") {
      expect(action.view).toBe("schedules");
    }
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("calls onSelect with email-templates view when Email Templates clicked", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    await user.click(screen.getByText("Email Templates"));

    expect(onSelect).toHaveBeenCalledOnce();
    const action: CommandAction = onSelect.mock.calls[0][0];
    expect(action.kind).toBe("view");
    if (action.kind === "view") {
      expect(action.view).toBe("email-templates");
    }
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("filters items as user types", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    const input = screen.getByPlaceholderText("Search tables, pages...");
    await user.type(input, "user");

    // "users" table and "Users" nav item should match
    expect(screen.getByText("users")).toBeInTheDocument();
    // "posts" and "comments" should be filtered out
    expect(screen.queryByText("posts")).not.toBeInTheDocument();
    expect(screen.queryByText("comments")).not.toBeInTheDocument();
  });

  it("shows schema prefix for non-public tables", () => {
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );
    expect(screen.getByText("internal.")).toBeInTheDocument();
  });

  it("calls onSelect with table action when table clicked", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    await user.click(screen.getByText("posts"));

    expect(onSelect).toHaveBeenCalledOnce();
    const action: CommandAction = onSelect.mock.calls[0][0];
    expect(action.kind).toBe("table");
    if (action.kind === "table") {
      expect(action.table.name).toBe("posts");
    }
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("calls onSelect with view action when nav item clicked", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    await user.click(screen.getByText("Webhooks"));

    expect(onSelect).toHaveBeenCalledOnce();
    const action: CommandAction = onSelect.mock.calls[0][0];
    expect(action.kind).toBe("view");
    if (action.kind === "view") {
      expect(action.view).toBe("webhooks");
    }
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("navigates with arrow keys and selects with Enter", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    const input = screen.getByPlaceholderText("Search tables, pages...");
    // First item is "users" (index 0). Press down to go to "posts" (index 1).
    await user.type(input, "{ArrowDown}");
    await user.type(input, "{Enter}");

    expect(onSelect).toHaveBeenCalledOnce();
    const action: CommandAction = onSelect.mock.calls[0][0];
    expect(action.kind).toBe("table");
    if (action.kind === "table") {
      expect(action.table.name).toBe("posts");
    }
  });

  it("closes on Escape key", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    const input = screen.getByPlaceholderText("Search tables, pages...");
    await user.type(input, "{Escape}");
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("closes when clicking backdrop", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    // Click the backdrop (the outermost fixed container)
    const backdrop = screen.getByRole("dialog").parentElement!;
    await user.click(backdrop);
    expect(onClose).toHaveBeenCalled();
  });

  it("shows no results message when nothing matches", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    const input = screen.getByPlaceholderText("Search tables, pages...");
    await user.type(input, "zzzzzznothing");

    expect(screen.getByText("No results found")).toBeInTheDocument();
  });

  it("fuzzy matches table names", async () => {
    const user = userEvent.setup();
    render(
      <CommandPalette open={true} onClose={onClose} onSelect={onSelect} tables={tables} />,
    );

    const input = screen.getByPlaceholderText("Search tables, pages...");
    // "cmts" should fuzzy-match "comments"
    await user.type(input, "cmts");

    expect(screen.getByText("comments")).toBeInTheDocument();
    expect(screen.queryByText("users")).not.toBeInTheDocument();
  });
});

describe("CommandPaletteHint", () => {
  it("renders clickable search hint", async () => {
    const onClick = vi.fn();
    const user = userEvent.setup();
    render(<CommandPaletteHint onClick={onClick} />);

    const btn = screen.getByText("Search...");
    expect(btn).toBeInTheDocument();

    await user.click(btn.closest("button")!);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("shows keyboard shortcut", () => {
    render(<CommandPaletteHint onClick={vi.fn()} />);
    expect(screen.getByText("K")).toBeInTheDocument();
  });
});
