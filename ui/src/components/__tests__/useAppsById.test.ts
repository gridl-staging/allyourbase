import { describe, expect, it, vi } from "vitest";
import type { AppListResponse, AppResponse } from "../../types";
import { loadAppsById } from "../useAppsById";

function makeApp(id: string, name: string): AppResponse {
  return {
    id,
    name,
    description: `${name} description`,
    ownerUserId: "owner-1",
    rateLimitRps: 100,
    rateLimitWindowSeconds: 60,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

function makeListResponse(
  items: AppResponse[],
  page: number,
  totalPages: number,
): AppListResponse {
  return {
    items,
    page,
    perPage: 100,
    totalItems: items.length,
    totalPages,
  };
}

describe("loadAppsById", () => {
  it("loads all pages and maps apps by id", async () => {
    const listAppsPage = vi
      .fn()
      .mockResolvedValueOnce(
        makeListResponse(
          [
            makeApp("app-1", "App One"),
            makeApp("app-2", "App Two"),
          ],
          1,
          2,
        ),
      )
      .mockResolvedValueOnce(
        makeListResponse([makeApp("app-3", "App Three")], 2, 2),
      );

    const appsById = await loadAppsById(listAppsPage);

    expect(listAppsPage).toHaveBeenCalledTimes(2);
    expect(listAppsPage).toHaveBeenNthCalledWith(1, { page: 1, perPage: 100 });
    expect(listAppsPage).toHaveBeenNthCalledWith(2, { page: 2, perPage: 100 });
    expect(Object.keys(appsById)).toEqual(["app-1", "app-2", "app-3"]);
    expect(appsById["app-3"]?.name).toBe("App Three");
  });

  it("stops after the first page when there are no results", async () => {
    const listAppsPage = vi
      .fn()
      .mockResolvedValueOnce(makeListResponse([], 1, 0));

    const appsById = await loadAppsById(listAppsPage);

    expect(listAppsPage).toHaveBeenCalledTimes(1);
    expect(appsById).toEqual({});
  });
});
