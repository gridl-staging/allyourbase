/**
 * @module Utilities for fetching and managing a list of apps indexed by ID, with a React hook for component integration.
 */
import { useEffect, useState } from "react";
import { listApps } from "../api";
import type { AppListResponse, AppResponse } from "../types";

const APPS_PER_PAGE = 100;

type ListAppsPage = (params: {
  page: number;
  perPage: number;
}) => Promise<AppListResponse>;

/**
 * Fetches and indexes all paginated apps by ID. Iterates through all pages of results until no more pages are available.
 * @param listAppsPage - function that fetches a single page of apps; defaults to listApps
 * @param perPage - number of apps per page; defaults to APPS_PER_PAGE
 * @returns record mapping app IDs to app data
 * @throws propagates errors from listAppsPage
 */
export async function loadAppsById(
  listAppsPage: ListAppsPage = listApps,
  perPage = APPS_PER_PAGE,
): Promise<Record<string, AppResponse>> {
  const appsById: Record<string, AppResponse> = {};
  let currentPage = 1;

  while (true) {
    const response = await listAppsPage({ page: currentPage, perPage });
    for (const app of response.items) {
      appsById[app.id] = app;
    }

    if (response.totalPages <= currentPage || response.totalPages === 0) {
      break;
    }
    currentPage += 1;
  }

  return appsById;
}

/**
 * React hook that loads all apps asynchronously and manages loading/error state. Fetches on mount and when failureMessage changes; cleans up on unmount via cancellation flag.
 * @param options - optional configuration object
 * @param options.failureMessage - error message displayed on failure; defaults to "Failed to load apps"
 * @returns object containing apps indexed by ID and error state
 */
export function useAppsById(options: {
  failureMessage?: string | null;
} = {}): {
  appsById: Record<string, AppResponse>;
  error: string | null;
} {
  const { failureMessage = "Failed to load apps" } = options;
  const [appsById, setAppsById] = useState<Record<string, AppResponse>>({});
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    const loadAllApps = async () => {
      try {
        setError(null);
        const loadedAppsById = await loadAppsById();
        if (cancelled) return;
        setAppsById(loadedAppsById);
      } catch {
        if (cancelled) return;
        setAppsById({});
        setError(failureMessage);
      }
    };

    loadAllApps();
    return () => {
      cancelled = true;
    };
  }, [failureMessage]);

  return { appsById, error };
}
