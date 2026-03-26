/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/backups/context.ts.
 */
export interface BackupContextSource {
  project_id?: string;
  database_id?: string;
}

export interface PITRContext {
  projectId: string;
  databaseId: string;
}

export interface PITRContextOption extends PITRContext {
  key: string;
  label: string;
}

export function buildPITRContextKey(projectId: string, databaseId: string): string {
  return `${projectId}:${databaseId}`;
}

/**
 * TODO: Document buildPITRContextOptions.
 */
export function buildPITRContextOptions(
  backups: readonly BackupContextSource[],
): PITRContextOption[] {
  const contexts = new Map<string, PITRContextOption>();
  for (const backup of backups) {
    if (backup.project_id && backup.database_id) {
      const key = buildPITRContextKey(backup.project_id, backup.database_id);
      if (!contexts.has(key)) {
        contexts.set(key, {
          key,
          projectId: backup.project_id,
          databaseId: backup.database_id,
          label: `${backup.project_id} / ${backup.database_id}`,
        });
      }
    }
  }
  return [...contexts.values()];
}

export function resolveSelectedPITRContextKey(
  contexts: readonly PITRContextOption[],
  currentKey: string,
): string {
  if (currentKey && contexts.some((context) => context.key === currentKey)) {
    return currentKey;
  }
  if (contexts.length === 1) {
    return contexts[0].key;
  }
  return "";
}
