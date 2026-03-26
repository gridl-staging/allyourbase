import type {
  AuthResponse,
  RealtimeEvent,
  StorageObject,
  User,
} from "./types";

export function normalizeAuthResponse(value: AuthResponse): AuthResponse {
  const source = toRecord(value);
  return {
    token: String(source.token ?? ""),
    refreshToken: String(source.refreshToken ?? ""),
    user: normalizeUser(source.user as User),
  };
}

export function normalizeUser(value: User): User {
  const source = toRecord(value);
  return {
    id: String(source.id ?? ""),
    email: String(source.email ?? ""),
    emailVerified: readBoolean(source, ["emailVerified", "email_verified"]),
    createdAt: readString(source, ["createdAt", "created_at"]),
    updatedAt: readString(source, ["updatedAt", "updated_at"]),
  };
}

export function normalizeStorageObject(value: StorageObject): StorageObject {
  const source = toRecord(value);
  return {
    id: String(source.id ?? ""),
    bucket: String(source.bucket ?? ""),
    name: String(source.name ?? ""),
    size: Number(source.size ?? 0),
    contentType: String(source.contentType ?? source.content_type ?? ""),
    userId: readString(source, ["userId", "user_id"]),
    createdAt: String(source.createdAt ?? source.created_at ?? ""),
    updatedAt: readString(source, ["updatedAt", "updated_at"]),
  };
}

export function normalizeStorageListResponse(value: { items: StorageObject[]; totalItems: number }): {
  items: StorageObject[];
  totalItems: number;
} {
  const source = value as unknown as Record<string, unknown>;
  const rawItems = Array.isArray(source.items) ? source.items : [];
  const items = rawItems.map((item) => normalizeStorageObject(item as StorageObject));
  return {
    items,
    totalItems: typeof source.totalItems === "number" ? source.totalItems : items.length,
  };
}

export function normalizeRealtimeEvent(value: RealtimeEvent): RealtimeEvent {
  const source = toRecord(value);
  const normalized: RealtimeEvent = {
    action: String(source.action ?? "") as RealtimeEvent["action"],
    table: String(source.table ?? ""),
    record: asRecord(source.record) ?? {},
  };
  const oldRecord = asRecord(source.oldRecord ?? source.old_record);
  if (oldRecord) {
    normalized.oldRecord = oldRecord;
  }
  return normalized;
}

export function readString(source: Record<string, unknown>, keys: string[]): string | undefined {
  for (const key of keys) {
    const value = source[key];
    if (typeof value === "string") {
      return value;
    }
  }
  return undefined;
}

export function readBoolean(source: Record<string, unknown>, keys: string[]): boolean | undefined {
  for (const key of keys) {
    const value = source[key];
    if (typeof value === "boolean") {
      return value;
    }
  }
  return undefined;
}

export function encodePathSegment(value: string): string {
  return encodeURIComponent(value);
}

export function encodePathWithSlashes(value: string): string {
  return value
    .split("/")
    .map((segment) => encodeURIComponent(segment))
    .join("/");
}

export function toRecord(value: unknown): Record<string, unknown> {
  return asRecord(value) ?? {};
}

export function asRecord(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  return value as Record<string, unknown>;
}

/**
 * Opens a centered popup window for OAuth. Must be called synchronously
 * in the click handler's call stack to avoid Safari's popup blocker.
 */
export function openPopup(): Window | null {
  const width = 1024;
  const height = 768;
  const left = Math.max(0, (screen.width - width) / 2);
  const top = Math.max(0, (screen.height - height) / 2);
  return window.open(
    "about:blank",
    "ayb-oauth",
    `width=${width},height=${height},left=${left},top=${top},scrollbars=yes`,
  );
}
