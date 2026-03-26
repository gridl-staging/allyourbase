/**
 * @module React hook for accessing authentication state, user data, and login/logout functionality with automatic session synchronization.
 */
import { useCallback, useEffect, useState } from "react";
import { useAYBClient } from "./provider";
import type { UseAuthResult, UserLike } from "./types";

/**
 * Manages authentication state and automatically syncs with the client's auth provider. Loads the current user on mount and resubscribes to auth state changes, handling token updates and session management. Returns current user data, tokens, loading/error states, and authentication methods.
 */
export function useAuth(): UseAuthResult {
  const client = useAYBClient();
  const [loading, setLoading] = useState<boolean>(Boolean(client.token));
  const [user, setUser] = useState<UserLike | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const [token, setToken] = useState<string | null>(client.token);
  const [refreshToken, setRefreshToken] = useState<string | null>(client.refreshToken);

  const loadMe = useCallback(async () => {
    if (!client.token) {
      setUser(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    try {
      const me = await client.auth.me();
      setUser(me);
      setError(null);
    } catch (err) {
      setUser(null);
      setError(err as Error);
    } finally {
      setLoading(false);
      setToken(client.token);
      setRefreshToken(client.refreshToken);
    }
  }, [client]);

  useEffect(() => {
    let mounted = true;

    const run = async () => {
      try {
        if (mounted) {
          await loadMe();
        }
      } catch {
        // loadMe sets local error state.
      }
    };

    void run();

    const unsubscribe = client.onAuthStateChange((event, session) => {
      if (!mounted) return;

      setToken(session?.token ?? client.token);
      setRefreshToken(session?.refreshToken ?? client.refreshToken);

      if (event === "SIGNED_OUT") {
        setUser(null);
        setError(null);
        setLoading(false);
        return;
      }

      void loadMe();
    });

    return () => {
      mounted = false;
      unsubscribe();
    };
  }, [client, loadMe]);

  const login = useCallback(
    async (email: string, password: string) => {
      await client.auth.login(email, password);
      await loadMe();
    },
    [client, loadMe],
  );

  const register = useCallback(
    async (email: string, password: string) => {
      await client.auth.register(email, password);
      await loadMe();
    },
    [client, loadMe],
  );

  const logout = useCallback(async () => {
    await client.auth.logout();
    setUser(null);
    setError(null);
    setToken(client.token);
    setRefreshToken(client.refreshToken);
  }, [client]);

  const refresh = useCallback(async () => {
    await client.auth.refresh();
    await loadMe();
  }, [client, loadMe]);

  return {
    loading,
    user,
    error,
    token,
    refreshToken,
    login,
    register,
    logout,
    refresh,
  };
}
