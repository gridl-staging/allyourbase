export interface BranchRecord {
  id: string;
  name: string;
  source_database: string;
  branch_database: string;
  status: "creating" | "ready" | "failed" | "deleting";
  created_at: string;
  updated_at: string;
  error_message?: string;
}
