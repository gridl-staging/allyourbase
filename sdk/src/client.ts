/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_4_scaffold_sdk_first_run_fix/allyourbase_dev/sdk/src/client.ts.
 */
import { AYBError } from "./errors";
import { AuthClient } from "./auth";
import { RecordsClient } from "./records";
import { StorageClient } from "./storage";
import { RealtimeClient } from "./realtime";
import { encodePathSegment } from "./helpers";
import type {
  AuthStateListener,
  AuthSession,
  ClientOptions,
  HealthResponse,
  RpcOptions,
} from "./types";

function normalizeErrorCode(rawCode: unknown): string | undefined {
  if (typeof rawCode === "string") {
    return rawCode;
  }
  if (typeof rawCode === "number") {
    return String(rawCode);
  }
  return undefined;
}

/**
 * TODO: Document AYBClient.
 */
export class AYBClient {
  private baseURL: string;
  private _fetch: typeof globalThis.fetch;
  private _token: string | null = null;
  private _refreshToken: string | null = null;
  private _authListeners: Set<AuthStateListener> = new Set();

  readonly auth: AuthClient;
  readonly records: RecordsClient;
  readonly storage: StorageClient;
  readonly realtime: RealtimeClient;

  constructor(baseURL: string, options?: ClientOptions) {
    this.baseURL = baseURL.replace(/\/+$/, "");
    this._fetch = options?.fetch ?? globalThis.fetch.bind(globalThis);

    this.auth = new AuthClient(this);
    this.records = new RecordsClient(this);
    this.storage = new StorageClient(this);
    this.realtime = new RealtimeClient(this);
  }

  /** Current access token, if authenticated. */
  get token(): string | null {
    return this._token;
  }

  /** Current refresh token, if authenticated. */
  get refreshToken(): string | null {
    return this._refreshToken;
  }

  private setAuthState(token: string | null, refreshToken: string | null): void {
    this._token = token;
    this._refreshToken = refreshToken;
  }

  private currentSession(): AuthSession | null {
    if (!this._token || !this._refreshToken) {
      return null;
    }
    return { token: this._token, refreshToken: this._refreshToken };
  }

  /** Manually set auth tokens (e.g. from storage). */
  setTokens(token: string, refreshToken: string): void {
    this.setAuthState(token, refreshToken);
  }

  /** Clear stored auth tokens. */
  clearTokens(): void {
    this.setAuthState(null, null);
  }

  /** Authenticate with an API key instead of JWT tokens. */
  setApiKey(apiKey: string): void {
    this.setAuthState(apiKey, null);
  }

  /** Clear the API key (or JWT token). Equivalent to clearTokens(). */
  clearApiKey(): void {
    this.clearTokens();
  }

  /** Subscribe to auth state changes and return an unsubscribe function. */
  onAuthStateChange(listener: AuthStateListener): () => void {
    this._authListeners.add(listener);
    return () => {
      this._authListeners.delete(listener);
    };
  }

  /** @internal */
  emitAuthEvent(event: "SIGNED_IN" | "SIGNED_OUT" | "TOKEN_REFRESHED"): void {
    const session = this.currentSession();
    for (const listener of this._authListeners) {
      listener(event, session);
    }
  }

  /** @internal */
  async request<T>(
    path: string,
    init?: RequestInit & { skipAuth?: boolean },
  ): Promise<T> {
    const headers: Record<string, string> = {
      ...(init?.headers as Record<string, string>),
    };
    if (!init?.skipAuth && this._token) {
      headers.Authorization = `Bearer ${this._token}`;
    }

    const url = `${this.baseURL}${path}`;
    const res = await this._fetch(url, { ...init, headers });
    if (!res.ok) {
      const body = await res.json().catch(() => ({ message: res.statusText }));
      throw new AYBError(
        res.status,
        body.message || res.statusText,
        normalizeErrorCode(body.code),
        body.data,
        body.doc_url ?? body.docUrl,
      );
    }
    if (res.status === 204) return undefined as T;
    return res.json();
  }

  /** @internal */
  setTokensInternal(token: string, refreshToken: string): void {
    this.setAuthState(token, refreshToken);
  }

  /** Check server and database health without requiring auth. */
  async health(): Promise<HealthResponse> {
    return this.request<HealthResponse>("/health", { skipAuth: true });
  }

  /**
   * Call a PostgreSQL function via the RPC endpoint.
   * Void functions return `undefined`; scalar functions return unwrapped values.
   */
  async rpc<T = unknown>(
    functionName: string,
    args?: Record<string, unknown>,
    options?: RpcOptions,
  ): Promise<T> {
    const hasArgs = args != null && Object.keys(args).length > 0;
    const headers: Record<string, string> = {};
    if (options?.notify) {
      headers["X-Notify-Table"] = options.notify.table;
      headers["X-Notify-Action"] = options.notify.action;
    }

    const init: RequestInit = { method: "POST" };
    if (Object.keys(headers).length > 0) {
      init.headers = headers;
    }
    if (hasArgs) {
      init.headers = { ...headers, "Content-Type": "application/json" };
      init.body = JSON.stringify(args);
    }

    return this.request<T>(`/api/rpc/${encodePathSegment(functionName)}`, {
      ...init,
    });
  }

  /** @internal */
  getBaseURL(): string {
    return this.baseURL;
  }
}
