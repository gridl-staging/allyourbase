/**
 * @module Utility functions for push notification UI formatting and styling, including helpers for device token preview, provider/status badge styling, and JSON push data parsing with validation.
 */
import type { PushDeliveryStatus } from "../../types";
export { formatDate } from "../shared/format";

export function previewDeviceToken(token: string): string {
  if (token.length <= 16) {
    return token;
  }
  return `${token.slice(0, 10)}...${token.slice(-6)}`;
}

export function providerBadgeClass(provider: string): string {
  switch (provider) {
    case "fcm":
      return "bg-orange-100 text-orange-700";
    case "apns":
      return "bg-blue-100 text-blue-700";
    default:
      return "bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200";
  }
}

export function statusBadgeClass(status: PushDeliveryStatus): string {
  switch (status) {
    case "pending":
      return "bg-yellow-100 text-yellow-700";
    case "sent":
      return "bg-green-100 text-green-700";
    case "failed":
      return "bg-red-100 text-red-700";
    case "invalid_token":
      return "bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200";
    default:
      return "bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-200";
  }
}

/**
 * Parses and validates JSON push notification data, ensuring it's an object with all string values. @param input - JSON string to parse. @returns Object with either the parsed data and null error, or null data with an error message describing the validation failure (invalid JSON, not an object, or non-string property values).
 */
export function parsePushDataJSON(input: string): {
  data: Record<string, string> | null;
  error: string | null;
} {
  const raw = input.trim();
  if (!raw) {
    return { data: {}, error: null };
  }

  let decoded: unknown;
  try {
    decoded = JSON.parse(raw);
  } catch {
    return { data: null, error: "Data must be valid JSON." };
  }

  if (decoded === null || typeof decoded !== "object" || Array.isArray(decoded)) {
    return { data: null, error: "Data must be a JSON object." };
  }

  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(decoded)) {
    if (typeof value !== "string") {
      return { data: null, error: `Data value for ${key} must be a string.` };
    }
    out[key] = value;
  }

  return { data: out, error: null };
}
