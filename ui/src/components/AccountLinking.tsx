import { useState } from "react";
import type { AuthTokens } from "../types";
import { createAnonymousSession, getAuthToken, linkEmail } from "../api";
import { Loader2 } from "lucide-react";

interface AccountLinkingProps {
  onLinked: (tokens: AuthTokens) => void;
}

export function AccountLinking({ onLinked }: AccountLinkingProps) {
  const [sessionReady, setSessionReady] = useState(() => getAuthToken() !== null);
  const [startingSession, setStartingSession] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const canSubmit = sessionReady && email.trim() !== "" && password.trim() !== "" && !loading;

  const handleStartAnonymousSession = async () => {
    setError(null);
    setSuccess(null);
    setStartingSession(true);
    try {
      await createAnonymousSession();
      setSessionReady(true);
      setSuccess("Anonymous session started. You can now link your account.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to start anonymous session");
    } finally {
      setStartingSession(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!sessionReady) {
      setError("Start an anonymous session before linking your account.");
      return;
    }
    setError(null);
    setLoading(true);
    try {
      const tokens = await linkEmail(email, password);
      setSuccess("Account linked successfully");
      onLinked(tokens);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to link account");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="p-6 max-w-md mx-auto space-y-4">
      <h2 className="text-lg font-semibold">Link Your Account</h2>
      <p className="text-sm text-gray-600 dark:text-gray-300">
        Set an email and password to secure your anonymous account.
      </p>

      {error && (
        <div className="px-4 py-2 bg-red-50 border border-red-200 rounded-lg text-red-800 text-sm">
          {error}
        </div>
      )}
      {success && (
        <div className="px-4 py-2 bg-green-50 border border-green-200 rounded-lg text-green-800 text-sm">
          {success}
        </div>
      )}

      {!sessionReady && (
        <button
          type="button"
          onClick={handleStartAnonymousSession}
          disabled={startingSession}
          className="w-full px-4 py-2 text-sm bg-gray-900 text-white rounded hover:bg-black disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {startingSession ? (
            <>
              <Loader2 className="w-4 h-4 animate-spin" />
              Starting Session...
            </>
          ) : (
            "Start Anonymous Session"
          )}
        </button>
      )}

      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label htmlFor="link-email" className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">
            Email
          </label>
          <input
            id="link-email"
            data-testid="link-email-input"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
            className="w-full px-3 py-2 border rounded text-sm"
          />
        </div>
        <div>
          <label htmlFor="link-password" className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">
            Password
          </label>
          <input
            id="link-password"
            data-testid="link-password-input"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Choose a password"
            className="w-full px-3 py-2 border rounded text-sm"
          />
        </div>
        <button
          type="submit"
          disabled={!canSubmit}
          className="w-full px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {loading ? (
            <>
              <Loader2 className="w-4 h-4 animate-spin" />
              Linking...
            </>
          ) : (
            "Link Account"
          )}
        </button>
      </form>
    </div>
  );
}
