import { useCallback, useEffect, useRef, useState } from "react";

function getErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

/**
 * Shared hook for admin dashboard pages that load a resource list with
 * loading/error states and an actionLoading flag for mutating operations.
 *
 * fetchFn is captured by ref so callers can pass inline lambdas without
 * triggering infinite re-fetch loops.
 */
export function useAdminResource<T>(
  fetchFn: () => Promise<T>,
) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState(false);

  const fetchRef = useRef(fetchFn);
  fetchRef.current = fetchFn;

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await fetchRef.current();
      setData(result);
    } catch (err) {
      setError(getErrorMessage(err, "Failed to load"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const runAction = useCallback(
    async (action: () => Promise<void>) => {
      setActionLoading(true);
      try {
        await action();
        await refresh();
      } catch (err) {
        setError(getErrorMessage(err, "Action failed"));
      } finally {
        setActionLoading(false);
      }
    },
    [refresh],
  );

  return { data, setData, loading, error, setError, actionLoading, refresh, runAction };
}
