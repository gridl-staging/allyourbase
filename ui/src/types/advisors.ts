export type DashboardTimeRange = "15m" | "1h" | "6h" | "24h" | "7d";

// --- Realtime Inspector (maps to realtime.Snapshot from backend) ---

export interface RealtimeConnections {
  sse: number;
  ws: number;
  total: number;
}

export interface RealtimeSubscriptionRow {
  name: string;
  type: "table" | "broadcast" | "presence";
  count: number;
}

export interface RealtimeCounters {
  droppedMessages: number;
  heartbeatFailures: number;
}

export interface RealtimeInspectorSnapshot {
  version: string;
  timestamp: string;
  connections: RealtimeConnections;
  subscriptions: RealtimeSubscriptionRow[];
  counters: RealtimeCounters;
}

export type AdvisorSeverity = "critical" | "high" | "medium" | "low";
export type AdvisorStatus = "open" | "accepted" | "resolved";

export interface SecurityFinding {
  id: string;
  severity: AdvisorSeverity;
  category: string;
  status: AdvisorStatus;
  title: string;
  description: string;
  remediation: string;
}

export interface SecurityAdvisorReport {
  evaluatedAt: string;
  stale: boolean;
  findings: SecurityFinding[];
}

export interface PerformanceQueryStat {
  fingerprint: string;
  normalizedQuery: string;
  meanMs: number;
  totalMs: number;
  calls: number;
  rows: number;
  endpoints: string[];
  trend: "up" | "down" | "flat";
}

export interface PerformanceAdvisorReport {
  generatedAt: string;
  stale: boolean;
  range: DashboardTimeRange;
  queries: PerformanceQueryStat[];
}
