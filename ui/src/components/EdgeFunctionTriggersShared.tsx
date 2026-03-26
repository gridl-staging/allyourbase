import type {
  DBTriggerResponse,
  DBTriggerEvent,
  CronTriggerResponse,
  StorageTriggerResponse,
} from "../types";

export type TriggerTab = "db" | "cron" | "storage";

export const DB_EVENTS: DBTriggerEvent[] = ["INSERT", "UPDATE", "DELETE"];
export const STORAGE_EVENTS = ["upload", "delete"];
const DB_EVENT_SET = new Set<string>(DB_EVENTS);

function toObject(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object") {
    return {};
  }
  return value as Record<string, unknown>;
}

function toString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function toBool(value: unknown, fallback = false): boolean {
  return typeof value === "boolean" ? value : fallback;
}

function toStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string");
}

export function normalizeDBTrigger(raw: unknown): DBTriggerResponse {
  const obj = toObject(raw);
  const normalizedEvents = toStringArray(obj.events)
    .map((event) => event.toUpperCase())
    .filter((event): event is DBTriggerEvent => DB_EVENT_SET.has(event));

  return {
    id: toString(obj.id),
    functionId: toString(obj.functionId ?? obj.function_id),
    tableName: toString(obj.tableName ?? obj.table_name),
    schema: toString(obj.schema ?? obj.schema_name, "public"),
    events: normalizedEvents,
    filterColumns: toStringArray(obj.filterColumns ?? obj.filter_columns),
    enabled: toBool(obj.enabled),
    createdAt: toString(obj.createdAt ?? obj.created_at),
    updatedAt: toString(obj.updatedAt ?? obj.updated_at),
  };
}

export function normalizeCronTrigger(raw: unknown): CronTriggerResponse {
  const obj = toObject(raw);
  return {
    id: toString(obj.id),
    functionId: toString(obj.functionId ?? obj.function_id),
    scheduleId: toString(obj.scheduleId ?? obj.schedule_id),
    cronExpr: toString(obj.cronExpr ?? obj.cron_expr),
    timezone: toString(obj.timezone, "UTC"),
    payload: obj.payload,
    enabled: toBool(obj.enabled),
    createdAt: toString(obj.createdAt ?? obj.created_at),
    updatedAt: toString(obj.updatedAt ?? obj.updated_at),
  };
}

export function normalizeStorageTrigger(raw: unknown): StorageTriggerResponse {
  const obj = toObject(raw);
  return {
    id: toString(obj.id),
    functionId: toString(obj.functionId ?? obj.function_id),
    bucket: toString(obj.bucket),
    eventTypes: toStringArray(obj.eventTypes ?? obj.event_types),
    prefixFilter: toString(obj.prefixFilter ?? obj.prefix_filter),
    suffixFilter: toString(obj.suffixFilter ?? obj.suffix_filter),
    enabled: toBool(obj.enabled),
    createdAt: toString(obj.createdAt ?? obj.created_at),
    updatedAt: toString(obj.updatedAt ?? obj.updated_at),
  };
}

export function normalizeDBTriggers(raw: unknown): DBTriggerResponse[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw.map((trigger) => normalizeDBTrigger(trigger));
}

export function normalizeCronTriggers(raw: unknown): CronTriggerResponse[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw.map((trigger) => normalizeCronTrigger(trigger));
}

export function normalizeStorageTriggers(raw: unknown): StorageTriggerResponse[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw.map((trigger) => normalizeStorageTrigger(trigger));
}

export function upsertByID<T extends { id: string }>(items: T[], item: T): T[] {
  if (!item.id) {
    return items;
  }
  const idx = items.findIndex((existing) => existing.id === item.id);
  if (idx === -1) {
    return [...items, item];
  }
  const next = [...items];
  next[idx] = item;
  return next;
}

export interface AddToastFn {
  (type: "success" | "error", message: string): void;
}
