/**
 * @module Custom React hook for polling data with configurable intervals, automatic visibility detection, and built-in race-condition handling.
 */
import { useCallback, useEffect, useRef, useState } from "react";

export interface UsePollingOptions {
  enabled?: boolean;
  pauseWhenHidden?: boolean;
  refreshKey?: string | number | boolean | null | undefined;
}

/**
 * Automatically polls a fetcher function at a regular interval, managing data, loading, and error states with race-condition protection. Optionally pauses when the page is hidden and supports manual refresh triggers. Cleans up polling intervals on unmount.
 * 
 * @param fetcher - Async function to execute on each poll cycle
 * @param intervalMs - Time in milliseconds between poll attempts
 * @param options - Configuration object with enabled, pauseWhenHidden, and refreshKey properties
 * @returns Object with data, loading state, error, and a refresh function to manually trigger fetching
 */
export function usePolling<T>(
  fetcher: () => Promise<T>,
  intervalMs: number,
  options: UsePollingOptions = {},
) {
  const { enabled = true, pauseWhenHidden = true, refreshKey } = options;
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);
  const mountedRef = useRef(true);
  const fetcherRef = useRef(fetcher);
  const requestIdRef = useRef(0);

  useEffect(() => {
    fetcherRef.current = fetcher;
  }, [fetcher]);

  const refresh = useCallback(async () => {
    const requestId = ++requestIdRef.current;
    try {
      setError(null);
      const next = await fetcherRef.current();
      if (!mountedRef.current || requestId !== requestIdRef.current) return;
      setData(next);
    } catch (err) {
      if (!mountedRef.current || requestId !== requestIdRef.current) return;
      setError(err);
    } finally {
      if (mountedRef.current && requestId === requestIdRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return;
    }

    void refresh();

    const tick = () => {
      if (pauseWhenHidden && document.visibilityState === "hidden") {
        return;
      }
      void refresh();
    };

    const id = window.setInterval(tick, intervalMs);
    return () => window.clearInterval(id);
  }, [enabled, intervalMs, pauseWhenHidden, refresh, refreshKey]);

  return { data, loading, error, refresh };
}
