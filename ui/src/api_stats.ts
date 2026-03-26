import { request } from "./api_client";
import type { StatsOverview } from "./types/stats";

export function getStats(): Promise<StatsOverview> {
  return request<StatsOverview>("/api/admin/stats");
}
