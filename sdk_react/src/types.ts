/**
 * @module Type definitions for the React SDK including authentication state management, client interface, and query/auth result types.
 */
export type AuthStateEvent = "SIGNED_IN" | "SIGNED_OUT" | "TOKEN_REFRESHED";

export type AuthStateListener = (
  event: AuthStateEvent,
  session: { token: string; refreshToken: string } | null,
) => void;

export interface UserLike {
  id: string;
  email: string;
  [key: string]: unknown;
}

/**
 * Client interface providing authentication, records access, and real-time subscriptions. Maintains auth state through token and refreshToken properties. The auth namespace includes login, register, logout, refresh, and me operations; records provides data querying; realtime enables table subscriptions. Register listeners for authentication state changes with onAuthStateChange, which returns an unsubscribe function.
 */
export interface AYBClientLike {
  token: string | null;
  refreshToken: string | null;
  onAuthStateChange(listener: AuthStateListener): () => void;
  auth: {
    login(email: string, password: string): Promise<unknown>;
    register(email: string, password: string): Promise<unknown>;
    logout(): Promise<void>;
    refresh(): Promise<unknown>;
    me(): Promise<UserLike>;
  };
  records: {
    list<T = Record<string, unknown>>(
      collection: string,
      params?: Record<string, unknown>,
    ): Promise<{ items: T[]; page: number; perPage: number; totalItems: number; totalPages: number }>;
  };
  realtime: {
    subscribe(
      tables: string[],
      callback: (event: Record<string, unknown>) => void,
    ): () => void;
  };
}

export interface UseQueryOptions {
  enabled?: boolean;
  suspense?: boolean;
}

export interface UseQueryResult<T> {
  data: { items: T[]; page: number; perPage: number; totalItems: number; totalPages: number } | null;
  loading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

export interface UseAuthResult {
  loading: boolean;
  user: UserLike | null;
  error: Error | null;
  token: string | null;
  refreshToken: string | null;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
}
