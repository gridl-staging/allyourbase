import { useState, useEffect, useCallback } from "react";
import { checkOAuthAuthorize, submitOAuthConsent, ApiError } from "../api";
import type { OAuthConsentPrompt } from "../api";
import { Loader2, AlertCircle, Shield } from "lucide-react";

const SCOPE_DESCRIPTIONS: Record<string, string> = {
  readonly: "Read your data",
  readwrite: "Read and modify your data",
  "*": "Full access to your account",
};

function getScopeDescription(scope: string): string {
  return SCOPE_DESCRIPTIONS[scope] || scope;
}

type State =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "consent"; prompt: OAuthConsentPrompt }
  | { kind: "redirecting" };

export function OAuthConsent() {
  const [state, setState] = useState<State>({ kind: "loading" });
  const [submitting, setSubmitting] = useState(false);

  // Parse URL params.
  const params = new URLSearchParams(window.location.search);
  const clientId = params.get("client_id") || "";
  const redirectUri = params.get("redirect_uri") || "";
  const scope = params.get("scope") || "";
  const responseType = params.get("response_type") || "";
  const stateParam = params.get("state") || "";
  const codeChallenge = params.get("code_challenge") || "";
  const codeChallengeMethod = params.get("code_challenge_method") || "";

  const hasRequiredParams = clientId && redirectUri && scope && responseType && stateParam && codeChallenge && codeChallengeMethod;

  const checkAuth = useCallback(async () => {
    if (!hasRequiredParams) {
      setState({ kind: "error", message: "Missing required parameters" });
      return;
    }

    try {
      const result = await checkOAuthAuthorize(params);
      if (!result.requires_consent && result.redirect_to) {
        // Consent already granted — redirect immediately.
        setState({ kind: "redirecting" });
        window.location.assign(result.redirect_to);
        return;
      }
      setState({ kind: "consent", prompt: result });
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        // User is not authenticated — redirect to login with return_to
        // so they come back to this consent page after logging in.
        const returnTo = encodeURIComponent(
          `${window.location.pathname}${window.location.search}${window.location.hash}`,
        );
        window.location.assign(`/?return_to=${returnTo}`);
        return;
      }
      setState({
        kind: "error",
        message: e instanceof Error ? e.message : "Authorization check failed",
      });
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

  const handleDecision = async (decision: "approve" | "deny") => {
    if (state.kind !== "consent") return;
    setSubmitting(true);
    try {
      const result = await submitOAuthConsent({
        decision,
        response_type: responseType,
        client_id: state.prompt.client_id,
        redirect_uri: state.prompt.redirect_uri,
        scope: state.prompt.scope,
        state: state.prompt.state,
        code_challenge: state.prompt.code_challenge,
        code_challenge_method: state.prompt.code_challenge_method,
        allowed_tables: state.prompt.allowed_tables,
      });
      if (result.redirect_to) {
        setState({ kind: "redirecting" });
        window.location.assign(result.redirect_to);
      }
    } catch (e) {
      setState({
        kind: "error",
        message: e instanceof Error ? e.message : "Consent submission failed",
      });
    } finally {
      setSubmitting(false);
    }
  };

  if (state.kind === "loading" || state.kind === "redirecting") {
    return (
      <div className="flex items-center justify-center min-h-screen bg-gray-50">
        <div className="text-center">
          <Loader2 className="w-6 h-6 animate-spin text-gray-400 mx-auto mb-2" />
          <p className="text-sm text-gray-500">
            {state.kind === "loading"
              ? "Checking authorization..."
              : "Redirecting..."}
          </p>
        </div>
      </div>
    );
  }

  if (state.kind === "error") {
    return (
      <div className="flex items-center justify-center min-h-screen bg-gray-50">
        <div className="bg-white rounded-lg shadow-lg p-6 max-w-md w-full mx-4 text-center">
          <AlertCircle className="w-8 h-8 text-red-400 mx-auto mb-3" />
          <h2 className="font-semibold text-gray-900 mb-2">
            Authorization Error
          </h2>
          <p className="text-sm text-red-600">{state.message}</p>
        </div>
      </div>
    );
  }

  const { prompt } = state;

  return (
    <div className="flex items-center justify-center min-h-screen bg-gray-50">
      <div className="bg-white rounded-lg shadow-lg p-6 max-w-md w-full mx-4">
        <div className="text-center mb-6">
          <Shield className="w-10 h-10 text-blue-500 mx-auto mb-3" />
          <h1 className="text-lg font-semibold text-gray-900">
            Authorization Request
          </h1>
          <p className="text-sm text-gray-600 mt-1">
            <strong>{prompt.client_name}</strong> wants access to your account
          </p>
        </div>

        <div className="border rounded-lg p-4 mb-6 bg-gray-50">
          <h2 className="text-xs font-medium text-gray-500 uppercase tracking-wider mb-2">
            Permissions requested
          </h2>
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <div className="w-2 h-2 rounded-full bg-blue-500 shrink-0" />
              <span className="text-sm text-gray-700">
                {getScopeDescription(prompt.scope)}
              </span>
            </div>
            {prompt.allowed_tables && prompt.allowed_tables.length > 0 && (
              <div className="ml-4 text-xs text-gray-500">
                Tables: {prompt.allowed_tables.join(", ")}
              </div>
            )}
          </div>
        </div>

        <div className="flex gap-3">
          <button
            onClick={() => handleDecision("deny")}
            disabled={submitting}
            className="flex-1 px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-lg hover:bg-gray-50 disabled:opacity-50"
          >
            Deny
          </button>
          <button
            onClick={() => handleDecision("approve")}
            disabled={submitting}
            className="flex-1 px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
          >
            {submitting ? "Approving..." : "Approve"}
          </button>
        </div>
      </div>
    </div>
  );
}
