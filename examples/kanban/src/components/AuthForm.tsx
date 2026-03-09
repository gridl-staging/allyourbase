import { useState } from "react";
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
      className="p-0.5 rounded hover:bg-gray-200 text-gray-400 hover:text-gray-600 transition-colors cursor-pointer inline-flex"
      title={copied ? "Copied!" : "Copy"}
    >
      {copied ? <CheckIcon className="text-green-600" /> : <CopyIcon />}
    </span>
  );
}

export default function AuthForm({ onAuth }: Props) {
  const [isLogin, setIsLogin] = useState(true);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      if (isLogin) {
        await ayb.auth.login(email, password);
      } else {
        await ayb.auth.register(email, password);
      }
      persistTokens(email);
      onAuth(email);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Authentication failed");
    } finally {
      setLoading(false);
    }
  }

  function fillAccount(acct: { email: string; password: string }) {
    setEmail(acct.email);
    setPassword(acct.password);
    setIsLogin(true);
    setError("");
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="bg-white rounded-xl shadow-lg p-8 w-full max-w-md">
        <div className="text-center mb-6">
          <h1 className="text-2xl font-bold text-gray-900">Kanban Board</h1>
          <p className="text-sm text-gray-500 mt-1">
            Powered by <span className="font-semibold">Allyourbase</span>
          </p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="auth-email" className="block text-sm font-medium text-gray-700 mb-1">
              Email
            </label>
            <input
              id="auth-email"
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              placeholder="you@example.com"
            />
          </div>

          <div>
            <label htmlFor="auth-password" className="block text-sm font-medium text-gray-700 mb-1">
              Password
            </label>
            <input
              id="auth-password"
              type="password"
              required
              minLength={8}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              placeholder="At least 8 characters"
            />
          </div>

          {error && (
            <p role="alert" className="text-sm text-red-600 bg-red-50 rounded-lg px-3 py-2">
              {error}
            </p>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full bg-blue-600 text-white py-2 px-4 rounded-lg font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
          >
            {loading ? "..." : isLogin ? "Sign In" : "Create Account"}
          </button>
        </form>

        <p className="text-center text-sm text-gray-500 mt-4">
          {isLogin ? "Don't have an account?" : "Already have an account?"}{" "}
          <button
            onClick={() => {
              setIsLogin(!isLogin);
              setError("");
            }}
            className="text-blue-600 hover:underline font-medium"
          >
            {isLogin ? "Sign up" : "Sign in"}
          </button>
        </p>

        {/* Demo accounts */}
        <div className="mt-5 border-t border-gray-200 pt-4">
          <p className="text-[11px] uppercase tracking-wider text-gray-400 font-semibold mb-2">
            Demo accounts
          </p>
          <div className="flex flex-col gap-1">
            {demoAccounts.map((acct) => (
              <button
                key={acct.email}
                type="button"
                onClick={() => fillAccount(acct)}
                className="w-full text-left px-2.5 py-2 rounded-lg bg-gray-50 hover:bg-gray-100 border border-transparent hover:border-gray-200 transition-colors group flex items-center gap-2"
              >
                <div className="flex-1 min-w-0">
                  <span className="text-xs font-mono text-gray-700 block truncate">{acct.email}</span>
                  <span className="text-[11px] font-mono text-gray-400">{acct.password}</span>
                </div>
                <CopyButton value={`${acct.email}\t${acct.password}`} />
              </button>
            ))}
          </div>
          <p className="text-[10px] text-gray-400 mt-2 text-center">
            Click to fill, then sign in
          </p>
        </div>

        {/* Try it out tips */}
        <div className="mt-4 bg-blue-50 border border-blue-100 rounded-lg px-4 py-3">
          <p className="text-xs font-semibold text-blue-900 mb-1.5">Try it out</p>
          <ul className="text-[11px] text-blue-800 space-y-1 list-disc list-inside">
            <li>Sign in with a demo account above</li>
            <li>Create a board and add some cards</li>
            <li>Open a second browser and sign in as a different user</li>
            <li>Edit cards in one window — watch them update instantly in the other</li>
          </ul>
        </div>
      </div>
    </div>
  );
}
