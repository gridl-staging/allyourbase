import { useState, useEffect, useCallback } from "react";
import type { TOTPEnrollment as TOTPEnrollmentType, MFAFactor } from "../types";
import {
  enrollTOTP,
  confirmTOTPEnroll,
  enrollEmailMFA,
  confirmEmailMFAEnroll,
  generateBackupCodes,
  regenerateBackupCodes,
  getBackupCodeCount,
  getMFAFactors,
} from "../api";
import { Loader2, AlertCircle, Shield, Mail, Key } from "lucide-react";

type EnrollStep =
  | { kind: "idle" }
  | { kind: "totp-enroll"; enrollment: TOTPEnrollmentType }
  | { kind: "email-enroll-pending" }
  | { kind: "backup-display"; codes: string[] };

const METHOD_LABELS: Record<string, string> = {
  totp: "Authenticator App",
  sms: "SMS",
  email: "Email",
};

export function MFAEnrollment() {
  const [factors, setFactors] = useState<MFAFactor[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [step, setStep] = useState<EnrollStep>({ kind: "idle" });
  const [backupCount, setBackupCount] = useState<number | null>(null);
  const [totpCode, setTotpCode] = useState("");
  const [emailCode, setEmailCode] = useState("");

  const fetchData = useCallback(async () => {
    try {
      setError(null);
      setLoading(true);
      const [factorsRes, countRes] = await Promise.all([
        getMFAFactors(),
        getBackupCodeCount().catch(() => ({ remaining: 0 })),
      ]);
      const safeFactors = Array.isArray(factorsRes?.factors) ? factorsRes.factors : [];
      const safeRemaining = typeof countRes?.remaining === "number" ? countRes.remaining : 0;
      setFactors(safeFactors);
      setBackupCount(safeRemaining);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load MFA data");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const handleTOTPEnroll = async () => {
    setError(null);
    setSuccess(null);
    try {
      const enrollment = await enrollTOTP();
      setStep({ kind: "totp-enroll", enrollment });
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to start TOTP enrollment");
    }
  };

  const handleTOTPConfirm = async () => {
    setError(null);
    try {
      await confirmTOTPEnroll(totpCode);
      setSuccess("TOTP MFA enrolled successfully");
      setStep({ kind: "idle" });
      setTotpCode("");
      fetchData();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to confirm TOTP enrollment");
    }
  };

  const handleEmailMFAEnroll = async () => {
    setError(null);
    setSuccess(null);
    try {
      await enrollEmailMFA();
      setStep({ kind: "email-enroll-pending" });
      setSuccess("Verification code sent to your email");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to start email MFA enrollment");
    }
  };

  const handleEmailMFAConfirm = async () => {
    setError(null);
    try {
      await confirmEmailMFAEnroll(emailCode);
      setSuccess("Email MFA enrolled successfully");
      setStep({ kind: "idle" });
      setEmailCode("");
      fetchData();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to confirm email MFA enrollment");
    }
  };

  const handleGenerateBackup = async () => {
    setError(null);
    setSuccess(null);
    try {
      const res = await generateBackupCodes();
      setStep({ kind: "backup-display", codes: res.codes });
      setBackupCount(res.codes.length);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to generate backup codes");
    }
  };

  const handleRegenerateBackup = async () => {
    setError(null);
    setSuccess(null);
    try {
      const res = await regenerateBackupCodes();
      setStep({ kind: "backup-display", codes: res.codes });
      setBackupCount(res.codes.length);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to regenerate backup codes");
    }
  };

  const hasMFA = factors.length > 0;

  return (
    <div className="p-6 max-w-2xl mx-auto space-y-6">
      <h2 className="text-lg font-semibold">Multi-Factor Authentication</h2>

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

      {loading ? (
        <div className="flex items-center justify-center h-64 text-gray-400 dark:text-gray-500">
          <Loader2 className="w-5 h-5 animate-spin mr-2" />
          Loading...
        </div>
      ) : (
        <>
          {/* Enrolled factors */}
          <div data-testid="mfa-enrolled-methods" className="space-y-2">
            <h3 className="text-sm font-medium text-gray-700 dark:text-gray-200">Enrolled Methods</h3>
            {factors.length === 0 ? (
              <p className="text-sm text-gray-500 dark:text-gray-400">No MFA methods enrolled</p>
            ) : (
              <div className="space-y-2">
                {factors.map((f) => (
                  <div key={f.id} data-testid={`mfa-factor-${f.method}`} className="flex items-center gap-3 p-3 border rounded-lg">
                    {f.method === "totp" ? (
                      <Shield className="w-4 h-4 text-blue-500" />
                    ) : f.method === "email" ? (
                      <Mail className="w-4 h-4 text-blue-500" />
                    ) : (
                      <Key className="w-4 h-4 text-blue-500" />
                    )}
                    <span className="text-sm font-medium">
                      {f.label || METHOD_LABELS[f.method] || f.method}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Backup code count */}
          {hasMFA && backupCount !== null && (
            <div className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
              <Key className="w-4 h-4" />
              <span>{backupCount} backup codes remaining</span>
            </div>
          )}

          {/* Enrollment actions */}
          {step.kind === "idle" && (
            <div className="space-y-3">
              <div className="flex flex-wrap gap-2">
                <button
                  onClick={handleTOTPEnroll}
                  className="px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
                >
                  Set Up Authenticator
                </button>
                <button
                  onClick={handleEmailMFAEnroll}
                  className="px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
                >
                  Set Up Email MFA
                </button>
                {hasMFA && (
                  <>
                    <button
                      onClick={handleGenerateBackup}
                      className="px-4 py-2 text-sm bg-gray-600 text-white rounded hover:bg-gray-700"
                    >
                      Generate Backup Codes
                    </button>
                    {backupCount !== null && backupCount > 0 && (
                      <button
                        onClick={handleRegenerateBackup}
                        className="px-4 py-2 text-sm border border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-200 rounded hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
                      >
                        Regenerate
                      </button>
                    )}
                  </>
                )}
              </div>
            </div>
          )}

          {/* TOTP enrollment form */}
          {step.kind === "totp-enroll" && (
            <div className="p-4 border rounded-lg space-y-4">
              <h3 className="text-sm font-semibold">Set Up Authenticator App</h3>
              <p className="text-sm text-gray-600 dark:text-gray-300">
                Scan this QR code or enter the secret manually in your authenticator app.
              </p>
              <div className="space-y-2">
                <div className="font-mono text-sm bg-gray-50 dark:bg-gray-800 p-3 rounded break-all">
                  {step.enrollment.secret}
                </div>
                <div data-testid="totp-uri" className="text-xs text-gray-400 dark:text-gray-500 break-all">
                  {step.enrollment.uri}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <input
                  data-testid="totp-confirm-code"
                  type="text"
                  inputMode="numeric"
                  maxLength={6}
                  placeholder="Enter 6-digit code"
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value)}
                  className="px-3 py-2 border rounded text-sm w-40"
                />
                <button
                  onClick={handleTOTPConfirm}
                  disabled={!totpCode}
                  className="px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
                >
                  Verify Code
                </button>
                <button
                  onClick={() => { setStep({ kind: "idle" }); setTotpCode(""); }}
                  className="px-4 py-2 text-sm text-gray-600 dark:text-gray-300 hover:text-gray-800 dark:text-gray-200"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}

          {/* Email MFA enrollment confirm */}
          {step.kind === "email-enroll-pending" && (
            <div className="p-4 border rounded-lg space-y-4">
              <h3 className="text-sm font-semibold">Confirm Email MFA</h3>
              <p className="text-sm text-gray-600 dark:text-gray-300">
                Enter the verification code sent to your email.
              </p>
              <div className="flex items-center gap-2">
                <input
                  data-testid="email-mfa-confirm-code"
                  type="text"
                  inputMode="numeric"
                  maxLength={6}
                  placeholder="Enter 6-digit code"
                  value={emailCode}
                  onChange={(e) => setEmailCode(e.target.value)}
                  className="px-3 py-2 border rounded text-sm w-40"
                />
                <button
                  onClick={handleEmailMFAConfirm}
                  disabled={!emailCode}
                  className="px-4 py-2 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
                >
                  Confirm Email MFA
                </button>
                <button
                  onClick={() => { setStep({ kind: "idle" }); setEmailCode(""); }}
                  className="px-4 py-2 text-sm text-gray-600 dark:text-gray-300 hover:text-gray-800 dark:text-gray-200"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}

          {/* Backup code display */}
          {step.kind === "backup-display" && (
            <div className="p-4 border rounded-lg space-y-4">
              <div className="flex items-center gap-2">
                <AlertCircle className="w-4 h-4 text-amber-500" />
                <h3 className="text-sm font-semibold">Save Your Backup Codes</h3>
              </div>
              <p className="text-sm text-gray-600 dark:text-gray-300">
                Store these codes in a safe place. Each code can only be used once.
              </p>
              <div className="grid grid-cols-2 gap-2 font-mono text-sm bg-gray-50 dark:bg-gray-800 p-4 rounded">
                {step.codes.map((code, i) => (
                  <div key={i}>{code}</div>
                ))}
              </div>
              <button
                onClick={() => setStep({ kind: "idle" })}
                className="px-4 py-2 text-sm bg-gray-600 text-white rounded hover:bg-gray-700"
              >
                Done
              </button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
