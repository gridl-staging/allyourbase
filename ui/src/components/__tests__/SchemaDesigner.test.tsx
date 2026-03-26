import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { SchemaCache } from "../../types";
import { SchemaDesigner } from "../SchemaDesigner";

function makeSchema(): SchemaCache {
  return {
    schemas: ["public"],
    builtAt: "2026-02-28T12:00:00Z",
    tables: {
      "public.users": {
        schema: "public",
        name: "users",
        kind: "table",
        columns: [
          { name: "id", position: 1, type: "uuid", nullable: false, isPrimaryKey: true, jsonType: "string" },
          { name: "email", position: 2, type: "text", nullable: false, isPrimaryKey: false, jsonType: "string" },
        ],
        primaryKey: ["id"],
      },
      "public.posts": {
        schema: "public",
        name: "posts",
        kind: "table",
        columns: [
          { name: "id", position: 1, type: "uuid", nullable: false, isPrimaryKey: true, jsonType: "string" },
          { name: "author_id", position: 2, type: "uuid", nullable: false, isPrimaryKey: false, jsonType: "string" },
        ],
        primaryKey: ["id"],
        foreignKeys: [
          {
            constraintName: "posts_author_id_fkey",
            columns: ["author_id"],
            referencedSchema: "public",
            referencedTable: "users",
            referencedColumns: ["id"],
          },
        ],
      },
    },
  };
}

describe("SchemaDesigner", () => {
  it("renders graph nodes and edge labels", () => {
    render(<SchemaDesigner schema={makeSchema()} />);

    expect(screen.getByTestId("schema-node-public.posts")).toBeInTheDocument();
    expect(screen.getByTestId("schema-node-public.users")).toBeInTheDocument();
    expect(screen.getAllByText(/posts_author_id_fkey/i).length).toBeGreaterThan(0);
  });

  it("click-to-explore updates details panel", async () => {
    const user = userEvent.setup();
    render(<SchemaDesigner schema={makeSchema()} />);

    await user.click(screen.getByTestId("schema-node-public.posts"));

    expect(screen.getByRole("heading", { name: /posts/i })).toBeInTheDocument();
    expect(screen.getAllByText(/author_id/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/Foreign Keys/i)).toBeInTheDocument();
  });

  it("layout controls update zoom and call arrange", async () => {
    const user = userEvent.setup();
    const onArrange = vi.fn();
    render(<SchemaDesigner schema={makeSchema()} onAutoArrange={onArrange} />);

    expect(screen.getByTestId("schema-zoom-level")).toHaveTextContent("100%");

    await user.click(screen.getByRole("button", { name: /Zoom In/i }));
    expect(screen.getByTestId("schema-zoom-level")).not.toHaveTextContent("100%");

    await user.click(screen.getByRole("button", { name: /Auto Arrange/i }));
    expect(onArrange).toHaveBeenCalledTimes(1);
  });

  it("renders loading, error, and empty states", async () => {
    const user = userEvent.setup();
    const retry = vi.fn();

    const { rerender } = render(<SchemaDesigner schema={makeSchema()} loading />);
    expect(screen.getByText(/Loading schema designer/i)).toBeInTheDocument();

    rerender(<SchemaDesigner schema={makeSchema()} error="boom" onRetry={retry} />);
    expect(screen.getByText(/boom/i)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Retry/i }));
    expect(retry).toHaveBeenCalledTimes(1);

    rerender(<SchemaDesigner schema={{ schemas: ["public"], builtAt: "x", tables: {} }} />);
    expect(screen.getByText(/No tables available/i)).toBeInTheDocument();
  });
});
