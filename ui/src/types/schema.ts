export interface SchemaCache {
  tables: Record<string, Table>;
  functions?: Record<string, SchemaFunction>;
  schemas: string[];
  builtAt: string;
}

export interface Table {
  schema: string;
  name: string;
  kind: string;
  comment?: string;
  columns: Column[];
  primaryKey: string[];
  foreignKeys?: ForeignKey[];
  indexes?: Index[];
  relationships?: Relationship[];
}

export interface Column {
  name: string;
  position: number;
  type: string;
  nullable: boolean;
  default?: string;
  comment?: string;
  isPrimaryKey: boolean;
  jsonType: string;
  enumValues?: string[];
}

export interface ForeignKey {
  constraintName: string;
  columns: string[];
  referencedSchema: string;
  referencedTable: string;
  referencedColumns: string[];
  onUpdate?: string;
  onDelete?: string;
}

export interface Index {
  name: string;
  isUnique: boolean;
  isPrimary: boolean;
  method: string;
  definition: string;
}

export interface Relationship {
  name: string;
  type: string;
  fromSchema: string;
  fromTable: string;
  fromColumns: string[];
  toSchema: string;
  toTable: string;
  toColumns: string[];
  fieldName: string;
}

export interface SchemaGraphNode {
  id: string;
  tableId: string;
  label: string;
  schema: string;
  table: string;
  kind: string;
  columnCount: number;
  columnsPreview: string[];
  position: { x: number; y: number };
}

export interface SchemaGraphEdge {
  id: string;
  source: string;
  target: string;
  label: string;
  relationshipName?: string;
  cardinality: string;
}

export interface SchemaDesignerTableDetails {
  tableId: string;
  schema: string;
  name: string;
  kind: string;
  columns: Column[];
  indexes: Index[];
  foreignKeys: ForeignKey[];
  relationships: Relationship[];
  comment?: string;
}

export interface FuncParam {
  name: string;
  type: string;
  position: number;
}

export interface SchemaFunction {
  schema: string;
  name: string;
  comment?: string;
  parameters: FuncParam[] | null;
  returnType: string;
  returnsSet: boolean;
  isVoid: boolean;
}
