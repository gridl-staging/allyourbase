/**
 * @module React hook for fetching paginated records with Suspense support and automatic request deduplication.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAYBClient } from "./provider";
import type { UseQueryOptions, UseQueryResult } from "./types";

type ListResult<T> = {
  items: T[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
};

type SuspenseResource<T> = {
  promise: Promise<void> | null;
  data: ListResult<T> | null;
  error: Error | null;
};

const suspenseCache = new WeakMap<object, Map<string, SuspenseResource<unknown>>>();

function getSuspenseResource<T>(client: object, key: string): SuspenseResource<T> {
  let byClient = suspenseCache.get(client);
  if (!byClient) {
    byClient = new Map<string, SuspenseResource<unknown>>();
    suspenseCache.set(client, byClient);
  }

  let resource = byClient.get(key) as SuspenseResource<T> | undefined;
  if (!resource) {
    resource = { promise: null, data: null, error: null };
    byClient.set(key, resource as SuspenseResource<unknown>);
  }
  return resource;
}

/**
 * Fetches a paginated list of records from a collection with support for React Suspense and conditional enabling.
 * 
 * @param collection - Collection name to fetch from
 * @param params - Optional query parameters to filter results
 * @param options - Query configuration including enabled state and suspense mode
 * @returns Query state object with data, loading flag, error, and refetch function
 */
export function useQuery<T>(
  collection: string,
  params?: Record<string, unknown>,
  options?: UseQueryOptions,
): UseQueryResult<T> {
  const client = useAYBClient();
  const enabled = options?.enabled ?? true;
  const suspense = options?.suspense ?? false;

  const queryKey = useMemo(() => JSON.stringify([collection, params ?? {}]), [collection, params]);
  const requestIdRef = useRef(0);
  const inFlightRef = useRef<Promise<void> | null>(null);
  const [data, setData] = useState<ListResult<T> | null>(null);
  const [loading, setLoading] = useState<boolean>(enabled);
  const [error, setError] = useState<Error | null>(null);

  const runFetch = useCallback(async () => {
    if (!enabled) {
      setLoading(false);
      return;
    }

    const currentRequest = ++requestIdRef.current;
    setLoading(true);

    try {
      const result = await client.records.list<T>(collection, params);
      if (currentRequest !== requestIdRef.current) return;
      setData(result);
      setError(null);
    } catch (err) {
      if (currentRequest !== requestIdRef.current) return;
      setError(err as Error);
    } finally {
      if (currentRequest === requestIdRef.current) {
        setLoading(false);
      }
      inFlightRef.current = null;
    }
  }, [client, collection, enabled, params]);

  const refetch = useCallback(async () => {
    const p = runFetch();
    inFlightRef.current = p;
    await p;
  }, [runFetch]);

  if (suspense) {
    const resource = getSuspenseResource<T>(client as object, queryKey);

    const suspenseFetch = () => {
      if (!enabled) return;
      resource.promise = client.records
        .list<T>(collection, params)
        .then((result) => {
          resource.data = result;
          resource.error = null;
        })
        .catch((err) => {
          resource.error = err as Error;
        })
        .finally(() => {
          resource.promise = null;
        });
    };

    if (!enabled) {
      return {
        data: null,
        loading: false,
        error: null,
        refetch,
      };
    }

    if (resource.error) throw resource.error;
    if (!resource.data) {
      if (!resource.promise) {
        suspenseFetch();
      }
      throw resource.promise;
    }

    return {
      data: resource.data,
      loading: false,
      error: null,
      refetch,
    };
  }

  useEffect(() => {
    setData(null);
    setError(null);
    if (!enabled) {
      setLoading(false);
      return;
    }
    void refetch();
  }, [queryKey, enabled, refetch]);

  return {
    data,
    loading,
    error,
    refetch,
  };
}
