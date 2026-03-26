/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/api_logs.ts.
 */
import { request } from "./api_client";
import { parseDateTimeToMs } from "./lib/dateTime";
import { asRecord } from "./lib/normalize";
import type { AdminLogEntry, AdminLogLevel, AdminLogsResult } from "./types/logs";

const LEVEL_NORMALIZATION: Record<string, AdminLogLevel> = {
  debug: "debug",
  info: "info",
  warn: "warn",
  warning: "warn",
  error: "error",
};

function normalizeLevel(rawLevel: string): AdminLogLevel {
  const normalized = rawLevel.trim().toLowerCase();
  return LEVEL_NORMALIZATION[normalized] ?? "unknown";
}

function normalizeAttrs(rawAttrs: unknown): Record<string, unknown> {
  const attrs = asRecord(rawAttrs);
  return attrs ? { ...attrs } : {};
}

function stableSerialize(value: unknown): string {
  if (value === null || typeof value !== "object") {
    return JSON.stringify(value);
  }

  if (Array.isArray(value)) {
    return `[${value.map((entry) => stableSerialize(entry)).join(",")}]`;
  }

  const entries = Object.entries(value as Record<string, unknown>).sort(([a], [b]) =>
    a.localeCompare(b),
  );
  const fields = entries.map(([key, nestedValue]) => `${JSON.stringify(key)}:${stableSerialize(nestedValue)}`);
  return `{${fields.join(",")}}`;
}

/**
 * TODO: Document normalizeRow.
 */
function normalizeRow(rawEntry: unknown, duplicateCounts: Map<string, number>): AdminLogEntry {
  const entry = asRecord(rawEntry);
  const time = typeof entry?.time === "string" ? entry.time : "";
  const rawLevel = typeof entry?.level === "string" ? entry.level : "";
  const message = typeof entry?.message === "string" ? entry.message : "";
  const attrs = normalizeAttrs(entry?.attrs);
  const attrsText = stableSerialize(attrs);
  const level = normalizeLevel(rawLevel);
  const levelLabel = rawLevel.trim() ? rawLevel.trim().toUpperCase() : "UNKNOWN";

  const fingerprint = `${time}|${level}|${message}|${attrsText}`;
  const duplicateCount = duplicateCounts.get(fingerprint) ?? 0;
  duplicateCounts.set(fingerprint, duplicateCount + 1);
  const id = duplicateCount === 0 ? fingerprint : `${fingerprint}#${duplicateCount + 1}`;

  return {
    id,
    time,
    parsedTimeMs: parseDateTimeToMs(time),
    level,
    levelLabel,
    message,
    attrs,
    attrsText,
    searchText: `${time} ${levelLabel} ${message} ${attrsText}`.toLowerCase(),
  };
}

export function normalizeAdminLogsPayload(payload: unknown): AdminLogsResult {
  const record = asRecord(payload);
  const rawEntries = Array.isArray(record?.entries) ? record.entries : [];
  const duplicateCounts = new Map<string, number>();
  const entries = rawEntries.map((rawEntry) => normalizeRow(rawEntry, duplicateCounts));
  const message = typeof record?.message === "string" ? record.message : undefined;
  const bufferingEnabled = !message || !message.toLowerCase().includes("not enabled");

  return {
    entries,
    message,
    bufferingEnabled,
  };
}

export async function listAdminLogs(): Promise<AdminLogsResult> {
  const payload = await request<unknown>("/api/admin/logs");
  return normalizeAdminLogsPayload(payload);
}
