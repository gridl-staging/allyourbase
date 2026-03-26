export interface StatsOverview {
  uptime_seconds: number;
  go_version: string;
  goroutines: number;
  memory_alloc: number;
  memory_sys: number;
  gc_cycles: number;
  db_pool_total?: number;
  db_pool_idle?: number;
  db_pool_in_use?: number;
  db_pool_max?: number;
}
