/**
 * @module Converts database schema metadata into a visual graph representation with grid layout positioning and relationship edges for the schema designer.
 */
import type {
  ForeignKey,
  Index,
  Relationship,
  SchemaCache,
  SchemaDesignerTableDetails,
  SchemaGraphEdge,
  SchemaGraphNode,
  Table,
} from "../../types";

export interface SchemaDesignerGraph {
  nodes: SchemaGraphNode[];
  edges: SchemaGraphEdge[];
  detailsByTableId: Record<string, SchemaDesignerTableDetails>;
}

function tableId(table: Table): string {
  return `${table.schema}.${table.name}`;
}

function cardinalityLabel(relType?: string): string {
  if (!relType) return "relationship";
  const t = relType.toLowerCase();
  if (t === "many-to-one") return "many -> one";
  if (t === "one-to-many") return "one -> many";
  if (t === "one-to-one") return "one -> one";
  if (t === "many-to-many") return "many -> many";
  return relType;
}

function firstColumnsPreview(table: Table): string[] {
  return table.columns
    .slice()
    .sort((a, b) => a.position - b.position)
    .slice(0, 4)
    .map((c) => `${c.name}:${c.type}`);
}

function toDetails(table: Table): SchemaDesignerTableDetails {
  return {
    tableId: tableId(table),
    schema: table.schema,
    name: table.name,
    kind: table.kind,
    columns: table.columns.slice().sort((a, b) => a.position - b.position),
    indexes: (table.indexes ?? []) as Index[],
    foreignKeys: (table.foreignKeys ?? []) as ForeignKey[],
    relationships: (table.relationships ?? []) as Relationship[],
    comment: table.comment,
  };
}

/**
 * Converts a schema cache into a visual graph structure for the schema designer. Tables are arranged in a 4-column grid, each node includes column count and preview, and edges represent foreign key constraints with cardinality labels.
 * @param schema - the schema cache containing tables and their relationships
 * @returns a graph with positioned table nodes, foreign key constraint edges, and table details indexed by ID
 */
export function buildSchemaDesignerGraph(schema: SchemaCache): SchemaDesignerGraph {
  const tables = Object.values(schema.tables).slice().sort((a, b) => {
    return `${a.schema}.${a.name}`.localeCompare(`${b.schema}.${b.name}`);
  });

  const nodes: SchemaGraphNode[] = tables.map((table, idx) => {
    const id = tableId(table);
    const columnCount = table.columns.length;
    const row = Math.floor(idx / 4);
    const col = idx % 4;
    return {
      id,
      tableId: id,
      label: table.schema === "public" ? table.name : `${table.schema}.${table.name}`,
      schema: table.schema,
      table: table.name,
      kind: table.kind,
      columnCount,
      columnsPreview: firstColumnsPreview(table),
      position: { x: col * 280, y: row * 180 },
    };
  });

  const nodeIds = new Set(nodes.map((n) => n.id));
  const edges: SchemaGraphEdge[] = [];

  for (const table of tables) {
    const source = tableId(table);

    for (const fk of table.foreignKeys ?? []) {
      const target = `${fk.referencedSchema}.${fk.referencedTable}`;
      if (!nodeIds.has(source) || !nodeIds.has(target)) continue;

      const rel = (table.relationships ?? []).find(
        (r) => r.fromSchema === table.schema && r.fromTable === table.name && r.toSchema === fk.referencedSchema && r.toTable === fk.referencedTable,
      );

      edges.push({
        id: `${source}->${target}:${fk.constraintName}`,
        source,
        target,
        label: `${fk.constraintName} (${cardinalityLabel(rel?.type)})`,
        relationshipName: rel?.name,
        cardinality: rel?.type ?? "many-to-one",
      });
    }
  }

  const detailsByTableId = Object.fromEntries(tables.map((t) => [tableId(t), toDetails(t)]));

  return {
    nodes,
    edges,
    detailsByTableId,
  };
}
