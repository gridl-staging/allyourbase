/**
 * @module Custom React hook for managing schema designer data loading and building a graph representation for visualization.
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import { getSchemaDesignerSchema } from "../../api";
import type { SchemaCache } from "../../types";
import { buildSchemaDesignerGraph } from "./graph";

export interface UseSchemaDesignerDataInput {
  initialSchema?: SchemaCache;
  loader?: () => Promise<SchemaCache>;
}

/**
 * Manages schema designer data loading and graph visualization. Loads data on mount from initialSchema or loader function, catching and storing any errors without throwing.
 * @param input - Optional configuration with initialSchema to bypass loading or custom loader function
 * @returns Object with schemaData, loading flag, error string, retry function, and graph properties (nodes, edges, detailsByTableId)
 */
export function useSchemaDesignerData(input: UseSchemaDesignerDataInput = {}) {
  const { initialSchema, loader = getSchemaDesignerSchema } = input;
  const [schemaData, setSchemaData] = useState<SchemaCache | undefined>(initialSchema);
  const [loading, setLoading] = useState<boolean>(!initialSchema);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (initialSchema) {
      setSchemaData(initialSchema);
      setLoading(false);
      setError(null);
      return;
    }

    setLoading(true);
    try {
      const next = await loader();
      setSchemaData(next);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load schema");
    } finally {
      setLoading(false);
    }
  }, [initialSchema, loader]);

  useEffect(() => {
    void load();
  }, [load]);

  const graph = useMemo(() => {
    if (!schemaData) {
      return {
        nodes: [],
        edges: [],
        detailsByTableId: {},
      };
    }
    return buildSchemaDesignerGraph(schemaData);
  }, [schemaData]);

  return {
    schemaData,
    loading,
    error,
    retry: load,
    ...graph,
  };
}
