import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { SchemaView } from "../SchemaView";
import type { Table } from "../../types";

function makeTable(overrides: Partial<Table> = {}): Table {
  return {
    schema: "public",
    name: "posts",
    kind: "table",
    columns: [
      {
        name: "id",
        position: 1,
        type: "uuid",
        nullable: false,
        isPrimaryKey: true,
        jsonType: "string",
        default: "gen_random_uuid()",
      },
      {
        name: "title",
        position: 2,
        type: "text",
        nullable: false,
        isPrimaryKey: false,
        jsonType: "string",
      },
      {
        name: "body",
        position: 3,
        type: "text",
        nullable: true,
        isPrimaryKey: false,
        jsonType: "string",
      },
    ],
    primaryKey: ["id"],
    ...overrides,
  };
}

describe("SchemaView", () => {
  it("renders column names and types", () => {
    render(<SchemaView table={makeTable()} />);

    expect(screen.getByText("Columns")).toBeInTheDocument();
    expect(screen.getByText("id")).toBeInTheDocument();
    expect(screen.getByText("title")).toBeInTheDocument();
    expect(screen.getByText("body")).toBeInTheDocument();
    expect(screen.getByText("uuid")).toBeInTheDocument();
    // "text" appears twice (title + body columns), so use getAllByText.
    expect(screen.getAllByText("text")).toHaveLength(2);
  });

  it("shows nullable status", () => {
    render(<SchemaView table={makeTable()} />);

    const rows = screen.getAllByRole("row");
    // Header + 3 column rows.
    expect(rows).toHaveLength(4);
  });

  it("shows default values", () => {
    render(<SchemaView table={makeTable()} />);
    expect(screen.getByText("gen_random_uuid()")).toBeInTheDocument();
  });

  it("renders foreign keys when present", () => {
    const table = makeTable({
      foreignKeys: [
        {
          constraintName: "posts_author_id_fkey",
          columns: ["author_id"],
          referencedSchema: "public",
          referencedTable: "users",
          referencedColumns: ["id"],
          onDelete: "CASCADE",
        },
      ],
    });
    render(<SchemaView table={table} />);

    expect(screen.getByText("Foreign Keys")).toBeInTheDocument();
    expect(screen.getByText("posts_author_id_fkey")).toBeInTheDocument();
    expect(screen.getByText(/ON DELETE CASCADE/)).toBeInTheDocument();
  });

  it("hides foreign keys section when empty", () => {
    render(<SchemaView table={makeTable()} />);
    expect(screen.queryByText("Foreign Keys")).not.toBeInTheDocument();
  });

  it("renders indexes when present", () => {
    const table = makeTable({
      indexes: [
        {
          name: "posts_pkey",
          isUnique: true,
          isPrimary: true,
          method: "btree",
          definition: "CREATE UNIQUE INDEX posts_pkey ON public.posts (id)",
        },
      ],
    });
    render(<SchemaView table={table} />);

    expect(screen.getByText("Indexes")).toBeInTheDocument();
    expect(screen.getByText("posts_pkey")).toBeInTheDocument();
    expect(screen.getByText("btree")).toBeInTheDocument();
  });

  it("renders relationships when present", () => {
    const table = makeTable({
      relationships: [
        {
          name: "posts_author",
          type: "many-to-one",
          fromSchema: "public",
          fromTable: "posts",
          fromColumns: ["author_id"],
          toSchema: "public",
          toTable: "users",
          toColumns: ["id"],
          fieldName: "author",
        },
      ],
    });
    render(<SchemaView table={table} />);

    expect(screen.getByText("Relationships")).toBeInTheDocument();
    expect(screen.getByText("author")).toBeInTheDocument();
    expect(screen.getByText("many-to-one")).toBeInTheDocument();
  });

  it("renders table comment when present", () => {
    const table = makeTable({ comment: "Blog posts table" });
    render(<SchemaView table={table} />);

    expect(screen.getByText("Comment")).toBeInTheDocument();
    expect(screen.getByText("Blog posts table")).toBeInTheDocument();
  });

  it("renders enum values for columns", () => {
    const table = makeTable({
      columns: [
        {
          name: "status",
          position: 1,
          type: "text",
          nullable: false,
          isPrimaryKey: false,
          jsonType: "string",
          enumValues: ["draft", "published", "archived"],
        },
      ],
    });
    render(<SchemaView table={table} />);

    expect(screen.getByText("[draft, published, archived]")).toBeInTheDocument();
  });
});
