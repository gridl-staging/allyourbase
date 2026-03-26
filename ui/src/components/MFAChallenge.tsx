import { useState, useEffect, useCallback } from "react";
import type { MFAFactor, AuthTokens } from "../types";
import {
  getMFAFactors,
  challengeTOTP,
  verifyTOTP,
  challengeSMSMFA,
  verifySMSMFA,
  challengeEmailMFA,
  verifyEmailMFA,
  verifyBackupCode,
} from "../api";
import { Loader2, Shield, Mail, Key } from "lucide-react";

interface MFAChallengeProps {
  onVerified: (tokens: AuthTokens) => void;
  codeExpirySeconds?: {
    email: number;
    sms: number;
  };
}

type ChallengeState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "select-factor"; factors: MFAFactor[] }
  | { kind: "code-entry"; method: string; challengeId: string }
  | { kind: "backup-entry" };

const METHOD_LABELS: Record<string, string> = {
  totp: "Authenticator App",
  email: "Email",
  sms: "SMS",
};

const DEFAULT_CODE_EXPIRY_SECONDS = {
  email: 600,
  sms: 300,
} as const;

const MFA_LOCKOUT_MESSAGE =
  "Too many failed attempts. Verification is temporarily locked. Try again later or use a backup code.";

function hasStatus(error: unknown, status: number): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "status" in error &&
    typeof (error as { status?: unknown }).status === "number" &&
    (error as { status: number }).status === status
  );
}

function extractErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message.trim();
  }
  if (
    typeof error === "object" &&
    error !== null &&
    "message" in error &&
    typeof (error as { message?: unknown }).message === "string"
  ) {
    return (error as { message: string }).message.trim();
  }
  return "";
}

function formatLockoutMessage(errorMessage: string): string {
  if (errorMessage.length === 0) {
    return MFA_LOCKOUT_MESSAGE;
  }
  if (errorMessage.toLowerCase() === MFA_LOCKOUT_MESSAGE.toLowerCase()) {
    return MFA_LOCKOUT_MESSAGE;
  }
  return `${MFA_LOCKOUT_MESSAGE} ${errorMessage}`;
}

function normalizeVerifyError(error: unknown): string {
  const message = extractErrorMessage(error);
  if (hasStatus(error, 429) || message.toLowerCase().includes("too many failed attempts")) {
    return formatLockoutMessage(message);
  }
  if (message.length > 0) {
    return message;
  }
  return "Verification failed";
}

export function MFAChallenge({ onVerified, codeExpirySeconds = DEFAULT_CODE_EXPIRY_SECONDS }: MFAChallengeProps) {
  const [state, setState] = useState<ChallengeState>({ kind: "loading" });
  const [code, setCode] = useState("");
  const [verifyError, setVerifyError] = useState<string | null>(null);
  const [verifying, setVerifying] = useState(false);
  const [resending, setResending] = useState(false);
  const [expiresAtMs, setExpiresAtMs] = useState<number | null>(null);
  const [nowMs, setNowMs] = useState(() => Date.now());

  const setChallengeExpiry = useCallback((durationSeconds: number | null) => {
    if (durationSeconds === null) {
      setExpiresAtMs(null);
      return;
    }
    const now = Date.now();
    setNowMs(now);
    setExpiresAtMs(now + (durationSeconds * 1000));
  }, []);

  const expirySeconds =
    expiresAtMs === null ? null : Math.max(0, Math.ceil((expiresAtMs - nowMs) / 1000));
  const isCodeEntryWithExpiringCode =
    state.kind === "code-entry" && (state.method === "email" || state.method === "sms");
  const isCodeExpired = isCodeEntryWithExpiringCode && expirySeconds !== null && expirySeconds <= 0;

  const startChallenge = useCallback(async (factor: MFAFactor) => {
    try {
      if (factor.method === "totp") {
        const res = await challengeTOTP();
        setChallengeExpiry(null);
        setState({ kind: "code-entry", method: "totp", challengeId: res.challenge_id });
      } else if (factor.method === "email") {
        const res = await challengeEmailMFA();
        setChallengeExpiry(codeExpirySeconds.email);
        setState({ kind: "code-entry", method: "email", challengeId: res.challenge_id });
      } else if (factor.method === "sms") {
        await challengeSMSMFA();
        setChallengeExpiry(codeExpirySeconds.sms);
        setState({ kind: "code-entry", method: "sms", challengeId: "" });
      } else {
        setChallengeExpiry(null);
        setState({ kind: "error", message: `Unsupported MFA method: ${factor.method}` });
      }
    } catch (e) {
      setState({ kind: "error", message: e instanceof Error ? e.message : "Failed to create challenge" });
    }
  }, [codeExpirySeconds.email, codeExpirySeconds.sms, setChallengeExpiry]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await getMFAFactors();
        if (cancelled) return;
        if (res.factors.length === 0) {
          setState({ kind: "error", message: "No MFA factors enrolled" });
          return;
        }
        if (res.factors.length === 1) {
          await startChallenge(res.factors[0]);
        } else {
          setState({ kind: "select-factor", factors: res.factors });
        }
      } catch (e) {
        if (cancelled) return;
        setState({ kind: "error", message: e instanceof Error ? e.message : "Failed to load factors" });
      }
    })();
    return () => { cancelled = true; };
  }, [startChallenge]);

  // Countdown timer for email/SMS code expiry
  useEffect(() => {
    if (expiresAtMs === null) return;
    const timer = setInterval(() => {
      setNowMs(Date.now());
    }, 1000);
    return () => clearInterval(timer);
  }, [expiresAtMs]);

  const handleResend = async () => {
    if (state.kind !== "code-entry") return;
    setResending(true);
    setVerifyError(null);
    try {
      if (state.method === "email") {
        const res = await challengeEmailMFA();
        setChallengeExpiry(codeExpirySeconds.email);
        setState({ kind: "code-entry", method: "email", challengeId: res.challenge_id });
      } else if (state.method === "sms") {
        await challengeSMSMFA();
        setChallengeExpiry(codeExpirySeconds.sms);
      }
      setCode("");
    } catch (e) {
      setVerifyError(e instanceof Error ? e.message : "Failed to resend code");
    } finally {
      setResending(false);
    }
  };

  const handleVerify = async () => {
    if (isCodeExpired) {
      setVerifyError("Code expired. Resend Code to get a new one.");
      return;
    }
    setVerifyError(null);
    setVerifying(true);
    try {
      if (state.kind === "backup-entry") {
        const tokens = await verifyBackupCode(code);
        onVerified(tokens);
      } else if (state.kind === "code-entry") {
        let tokens: AuthTokens;
        if (state.method === "totp") {
          tokens = await verifyTOTP(state.challengeId, code);
        } else if (state.method === "sms") {
          tokens = await verifySMSMFA(code);
        } else {
          tokens = await verifyEmailMFA(state.challengeId, code);
        }
        onVerified(tokens);
      }
    } catch (e) {
      setVerifyError(normalizeVerifyError(e));
    } finally {
      setVerifying(false);
    }
  };

  const handleUseBackup = () => {
    setCode("");
    setVerifyError(null);
    setChallengeExpiry(null);
    setState({ kind: "backup-entry" });
  };

  if (state.kind === "loading") {
    return (
      <div className="flex items-center justify-center h-64 text-gray-400 dark:text-gray-500">
        <Loader2 className="w-5 h-5 animate-spin mr-2" />
        Loading...
      </div>
    );
  }

  if (state.kind === "error") {
    return (
      <div className="p-6 max-w-md mx-auto">
        <div className="px-4 py-2 bg-red-50 border border-red-200 rounded-lg text-red-800 text-sm">
          {state.message}
        </div>
      </div>
    );
  }

  if (state.kind === "select-factor") {
    return (
      <div className="p-6 max-w-md mx-auto space-y-4">
        <h2 className="text-lg font-semibold">Verify Your Identity</h2>
        <p className="text-sm text-gray-600 dark:text-gray-300">Choose a verification method:</p>
        <div className="space-y-2">
          {state.factors.map((f) => (
            <button
              key={f.id}
              onClick={() => startChallenge(f)}
              className="w-full flex items-center gap-3 p-3 border rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800 text-left"
            >
              {f.method === "totp" ? (
                <Shield className="w-5 h-5 text-blue-500" />
              ) : f.method === "email" ? (
                <Mail className="w-5 h-5 text-blue-500" />
              ) : (
                <Key className="w-5 h-5 text-blue-500" />
              )}
              <span className="text-sm font-medium">
                {f.label || METHOD_LABELS[f.method] || f.method}
              </span>
            </button>
          ))}
        </div>
      </div>
    );
  }

  const isBackup = state.kind === "backup-entry";
  const displayExpirySeconds = expirySeconds ?? 0;
  const methodLabel = isBackup
    ? "Backup Code"
    : state.kind === "code-entry"
    ? METHOD_LABELS[state.method] || state.method
    : "";

  return (
    <div className="p-6 max-w-md mx-auto space-y-4">
      <h2 className="text-lg font-semibold">Verify Your Identity</h2>
      <p className="text-sm text-gray-600 dark:text-gray-300">
        {isBackup
          ? "Enter one of your backup codes."
          : state.kind === "code-entry" && state.method === "email"
          ? "A code was sent to your email."
          : state.kind === "code-entry" && state.method === "sms"
          ? "A code was sent to your phone."
          : `Enter the code from your ${methodLabel.toLowerCase()}.`}
      </p>

      {verifyError && (
        <div className="px-4 py-2 bg-red-50 border border-red-200 rounded-lg text-red-800 text-sm">
          {verifyError}
        </div>
      )}

      <div className="space-y-3">
        {isBackup ? (
          <input
            data-testid="backup-code-input"
            type="text"
            placeholder="xxxxx-xxxxx"
            value={code}
            onChange={(e) => setCode(e.target.value)}
            className="w-full px-3 py-2 border rounded text-sm font-mono"
          />
        ) : (
          <input
            data-testid="mfa-code-input"
            type="text"
            inputMode="numeric"
            maxLength={6}
            placeholder="Enter 6-digit code"
            value={code}
            onChange={(e) => setCode(e.target.value)}
            className="w-full px-3 py-2 border rounded text-sm font-mono"
          />
        )}
        <button
          onClick={handleVerify}
          disabled={!code || verifying || isCodeExpired}
          className="w-full px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {verifying ? (
            <>
              <Loader2 className="w-4 h-4 animate-spin" />
              Verifying...
            </>
          ) : (
            "Verify"
          )}
        </button>
        {!isBackup && state.kind === "code-entry" && (state.method === "email" || state.method === "sms") && (
          <div className="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
            {isCodeExpired ? (
              <span className="text-red-600">Code expired. Resend Code to get a new one.</span>
            ) : (
              <span>
                Code expires in {Math.floor(displayExpirySeconds / 60)}:{String(displayExpirySeconds % 60).padStart(2, "0")}
              </span>
            )}
            <button
              onClick={handleResend}
              disabled={resending}
              className="text-blue-600 hover:text-blue-700 disabled:opacity-50"
            >
              {resending ? "Resending..." : "Resend Code"}
            </button>
          </div>
        )}
        {!isBackup && (
          <button
            onClick={handleUseBackup}
            className="w-full text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 dark:text-gray-200"
          >
            Use Backup Code
          </button>
        )}
      </div>
    </div>
  );
}
