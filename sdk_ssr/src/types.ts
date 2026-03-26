export interface CookieOptions {
  accessTokenName?: string;
  refreshTokenName?: string;
  path?: string;
  domain?: string;
  secure?: boolean;
  sameSite?: "lax" | "strict" | "none";
  maxAge?: number;
  httpOnly?: boolean;
}

export interface ServerSession {
  token: string;
  refreshToken: string;
  user: Record<string, unknown>;
}

export interface SessionLoadResult {
  session: ServerSession | null;
  setCookieHeaders: string[];
}

export interface SSRClientLike {
  setTokens(token: string, refreshToken: string): void;
  clearTokens(): void;
  auth: {
    me(): Promise<Record<string, unknown>>;
    refresh(): Promise<{ token: string; refreshToken: string; user?: Record<string, unknown> }>;
  };
}
