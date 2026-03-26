import { describe, expect, it } from "vitest";
import type { SchemaCache } from "../../types";
import { buildSchemaDesignerGraph } from "./graph";

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
          {
            name: "id",
            position: 1,
            type: "uuid",
            nullable: false,
            isPrimaryKey: true,
            jsonType: "string",
          },
        ],
        primaryKey: ["id"],
      },
      "public.posts": {
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
          },
          {
            name: "author_id",
            position: 2,
            type: "uuid",
            nullable: false,
            isPrimaryKey: false,
            jsonType: "string",
          },
        ],
        primaryKey: ["id"],
        foreignKeys: [
          {
            constraintName: "posts_author_id_fkey",
            columns: ["author_id"],
            referencedSchema: "public",
            referencedTable: "users",
            referencedColumns: ["id"],
            onDelete: "cascade",
          },
        ],
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
        indexes: [
          {
            name: "posts_author_idx",
            isUnique: false,
            isPrimary: false,
            method: "btree",
            definition: "CREATE INDEX posts_author_idx ON posts(author_id)",
          },
        ],
      },
      "public.broken": {
        schema: "public",
        name: "broken",
        kind: "table",
        columns: [],
        primaryKey: [],
        // malformed FK target should be safely ignored
        foreignKeys: [
          {
            constraintName: "broken_fk",
            columns: ["x"],
            referencedSchema: "public",
            referencedTable: "missing",
            referencedColumns: ["id"],
          },
        ],
      },
    },
  };
}

describe("buildSchemaDesignerGraph", () => {
  it("builds deterministic nodes, edges, and detail lookup", () => {
    const out = buildSchemaDesignerGraph(makeSchema());

    expect(out.nodes.map((n) => n.id)).toEqual(["public.broken", "public.posts", "public.users"]);
    expect(out.edges).toHaveLength(1);
    expect(out.edges[0].source).toBe("public.posts");
    expect(out.edges[0].target).toBe("public.users");
    expect(out.edges[0].label.toLowerCase()).toContain("many");

    expect(out.detailsByTableId["public.posts"]?.foreignKeys).toHaveLength(1);
    expect(out.detailsByTableId["public.posts"]?.indexes).toHaveLength(1);
    expect(out.detailsByTableId["public.posts"]?.relationships).toHaveLength(1);
  });
});
