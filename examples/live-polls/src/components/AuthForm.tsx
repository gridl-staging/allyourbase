import { FormEvent, useState } from "react";
import { ayb, persistTokens } from "../lib/ayb";

interface Props {
  onAuth: (email: string) => void;
}

const demoAccounts = [
  { email: "alice@demo.test", password: "password123" },
  { email: "bob@demo.test", password: "password123" },
  { email: "charlie@demo.test", password: "password123" },
];

function CopyIcon({ className }: { className?: string }) {
  return (
    <svg className={className} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  );
}

function CheckIcon({ className }: { className?: string }) {
  return (
    <svg className={className} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy(e: React.MouseEvent) {
    e.stopPropagation();
    navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <span
      role="button"
      tabIndex={0}
      onClick={handleCopy}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") handleCopy(e as unknown as React.MouseEvent); }}
      className="p-0.5 rounded hover:bg-gray-600 text-gray-500 hover:text-gray-300 transition-colors cursor-pointer inline-flex"
      title={copied ? "Copied!" : "Copy"}
    >
      {copied ? <CheckIcon className="text-green-400" /> : <CopyIcon />}
    </span>
  );
}

export default function AuthForm({ onAuth }: Props) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      if (mode === "login") {
        await ayb.auth.login(email, password);
      } else {
        await ayb.auth.register(email, password);
      }
      persistTokens(email);
      onAuth(email);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Something went wrong");
    } finally {
      setLoading(false);
    }
  }

  function fillAccount(acct: { email: string; password: string }) {
    setEmail(acct.email);
    setPassword(acct.password);
    setMode("login");
    setError("");
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="bg-gray-900 border border-gray-700 rounded-xl p-6 w-full max-w-sm shadow-2xl">
        <h1 className="text-2xl font-bold mb-1">Live Polls</h1>
        <p className="text-gray-400 text-sm mb-6">
          {mode === "login" ? "Sign in to create and vote on polls" : "Create your account"}
        </p>
        <form onSubmit={handleSubmit} className="flex flex-col gap-3">
          <input
            type="email"
            placeholder="Email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="bg-gray-800 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-blue-500"
            required
            autoFocus
          />
          <input
            type="password"
            placeholder="Password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="bg-gray-800 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-blue-500"
            required
            minLength={8}
          />
          {error && <p className="text-red-400 text-xs">{error}</p>}
          <button
            type="submit"
            disabled={loading}
            className="bg-blue-600 hover:bg-blue-500 disabled:opacity-50 rounded py-2 text-sm font-semibold"
          >
            {loading ? "..." : mode === "login" ? "Sign In" : "Create Account"}
          </button>
        </form>
        <div className="mt-3 text-center text-xs text-gray-400">
          {mode === "login" ? (
            <>
              No account?{" "}
              <button onClick={() => setMode("register")} className="text-blue-400 hover:underline">
                Register
              </button>
            </>
          ) : (
            <>
              Have an account?{" "}
              <button onClick={() => setMode("login")} className="text-blue-400 hover:underline">
                Sign in
              </button>
            </>
          )}
        </div>

        {/* Demo accounts — only shown in login mode */}
        {mode === "login" && <div className="mt-5 border-t border-gray-700 pt-4">
          <p className="text-[11px] uppercase tracking-wider text-gray-500 font-semibold mb-2">
            Demo accounts
          </p>
          <div className="flex flex-col gap-1">
            {demoAccounts.map((acct) => (
              <button
                key={acct.email}
                type="button"
                onClick={() => fillAccount(acct)}
                className="w-full text-left px-2.5 py-2 rounded-lg bg-gray-800/50 hover:bg-gray-800 border border-transparent hover:border-gray-600 transition-colors group flex items-center gap-2"
              >
                <div className="flex-1 min-w-0">
                  <span className="text-xs font-mono text-gray-300 block truncate">{acct.email}</span>
                  <span className="text-[11px] font-mono text-gray-500">{acct.password}</span>
                </div>
                <CopyButton value={`${acct.email}\t${acct.password}`} />
              </button>
            ))}
          </div>
          <p className="text-[10px] text-gray-600 mt-2 text-center">
            Click to fill, then sign in
          </p>
        </div>}

        {/* Try it out tips */}
        <div className="mt-4 bg-gray-800/50 border border-gray-700 rounded-lg px-4 py-3">
          <p className="text-xs font-semibold text-gray-200 mb-1.5">Try it out</p>
          <ul className="text-[11px] text-gray-400 space-y-1 list-disc list-inside">
            <li>Sign in with a demo account above</li>
            <li>Create a poll and add some options</li>
            <li>Open a second browser and sign in as a different user</li>
            <li>Vote in one window — watch the results update live in the other</li>
          </ul>
        </div>
      </div>
    </div>
  );
}
