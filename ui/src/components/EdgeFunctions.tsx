import { useState, useEffect, useCallback } from "react";
import type { EdgeFunctionResponse } from "../types";
import { listEdgeFunctions } from "../api";
import { useAppToast } from "./ToastProvider";
import { FunctionList } from "./edge-functions/FunctionList";
import { FunctionDetail } from "./edge-functions/FunctionDetail";
import { FunctionCreate } from "./edge-functions/FunctionCreate";

type View =
  | { kind: "list" }
  | { kind: "detail"; id: string }
  | { kind: "create" };

export function EdgeFunctions() {
  const [view, setView] = useState<View>({ kind: "list" });
  const [functions, setFunctions] = useState<EdgeFunctionResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const { addToast } = useAppToast();

  const fetchFunctions = useCallback(async () => {
    try {
      setError(null);
      setLoading(true);
      const data = await listEdgeFunctions();
      setFunctions(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load edge functions");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchFunctions();
  }, [fetchFunctions]);

  const handleBack = useCallback(() => {
    setView({ kind: "list" });
    fetchFunctions();
  }, [fetchFunctions]);

  return (
    <>
      {view.kind === "detail" ? (
        <FunctionDetail
          id={view.id}
          onBack={handleBack}
          addToast={addToast}
        />
      ) : view.kind === "create" ? (
        <FunctionCreate
          onBack={handleBack}
          addToast={addToast}
        />
      ) : (
        <div className="p-6">
          <FunctionList
            functions={functions}
            loading={loading}
            error={error}
            onSelect={(id) => setView({ kind: "detail", id })}
            onCreate={() => setView({ kind: "create" })}
          />
        </div>
      )}
    </>
  );
}
