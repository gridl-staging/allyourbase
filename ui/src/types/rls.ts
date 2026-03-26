export interface RlsPolicy {
  tableSchema: string;
  tableName: string;
  policyName: string;
  command: string;
  permissive: string;
  roles: string[];
  usingExpr: string | null;
  withCheckExpr: string | null;
}

export interface RlsTableStatus {
  rlsEnabled: boolean;
  forceRls: boolean;
}
