export interface DrainStats {
  sent: number;
  failed: number;
  dropped: number;
}

export interface DrainInfo {
  id: string;
  name: string;
  stats: DrainStats;
}

export interface LogDrainConfig {
  id?: string;
  type: string;
  url: string;
  headers?: Record<string, string>;
  batch_size?: number;
  flush_interval_seconds?: number;
  enabled?: boolean;
}
