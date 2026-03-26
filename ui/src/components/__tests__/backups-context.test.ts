import { describe, expect, it } from "vitest";
import {
  buildPITRContextOptions,
  resolveSelectedPITRContextKey,
} from "../backups/context";

type BackupStub = {
  project_id?: string;
  database_id?: string;
};

describe("backups context helpers", () => {
  it("deduplicates contexts and skips rows missing project/database ids", () => {
    const contexts = buildPITRContextOptions([
      { project_id: "project-1", database_id: "db-1" },
      { project_id: "project-1", database_id: "db-1" },
      { project_id: "project-2", database_id: "db-2" },
      { project_id: "project-3" },
      { database_id: "db-4" },
    ] satisfies BackupStub[]);

    expect(contexts).toEqual([
      {
        key: "project-1:db-1",
        projectId: "project-1",
        databaseId: "db-1",
        label: "project-1 / db-1",
      },
      {
        key: "project-2:db-2",
        projectId: "project-2",
        databaseId: "db-2",
        label: "project-2 / db-2",
      },
    ]);
  });

  it("keeps valid selected key, auto-selects a single context, otherwise returns empty", () => {
    const oneContext = buildPITRContextOptions([
      { project_id: "project-1", database_id: "db-1" },
    ] satisfies BackupStub[]);
    const manyContexts = buildPITRContextOptions([
      { project_id: "project-1", database_id: "db-1" },
      { project_id: "project-2", database_id: "db-2" },
    ] satisfies BackupStub[]);

    expect(resolveSelectedPITRContextKey(manyContexts, "project-2:db-2")).toBe(
      "project-2:db-2",
    );
    expect(resolveSelectedPITRContextKey(oneContext, "")).toBe("project-1:db-1");
    expect(resolveSelectedPITRContextKey(manyContexts, "missing:key")).toBe("");
  });
});
