/**
 * @module Type definitions for job queue and schedule management API responses and requests.
 */
export type JobState = "queued" | "running" | "completed" | "failed" | "canceled";

/**
 * Represents a job in the queue system with its current execution state, timing information, and retry history. Includes metadata about the job's lifecycle from creation through completion or failure.
 */
export interface JobResponse {
  id: string;
  type: string;
  payload: Record<string, unknown>;
  state: JobState;
  runAt: string;
  leaseUntil: string | null;
  workerId: string | null;
  attempts: number;
  maxAttempts: number;
  lastError: string | null;
  lastRunAt: string | null;
  idempotencyKey: string | null;
  scheduleId: string | null;
  createdAt: string;
  updatedAt: string;
  completedAt: string | null;
  canceledAt: string | null;
}

export interface JobListResponse {
  items: JobResponse[];
  count: number;
}

export interface QueueStats {
  queued: number;
  running: number;
  completed: number;
  failed: number;
  canceled: number;
  oldestQueuedAgeSec: number | null;
}

export interface ScheduleResponse {
  id: string;
  name: string;
  jobType: string;
  payload: Record<string, unknown>;
  cronExpr: string;
  timezone: string;
  enabled: boolean;
  maxAttempts: number;
  nextRunAt: string | null;
  lastRunAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface ScheduleListResponse {
  items: ScheduleResponse[];
  count: number;
}

export interface CreateScheduleRequest {
  name: string;
  jobType: string;
  cronExpr: string;
  timezone: string;
  payload?: Record<string, unknown>;
  enabled?: boolean;
  maxAttempts?: number;
}

export interface UpdateScheduleRequest {
  cronExpr?: string;
  timezone?: string;
  payload?: Record<string, unknown>;
  enabled?: boolean;
}
