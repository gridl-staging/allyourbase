import { useEffect, useState } from "react";
import { isLoggedIn, clearPersistedTokens, getPersistedEmail, ayb } from "./lib/ayb";
import type { Board } from "./types";
import AuthForm from "./components/AuthForm";
import BoardList from "./components/BoardList";
import BoardView from "./components/BoardView";

export default function App() {
  const [authed, setAuthed] = useState(isLoggedIn());
  const [email, setEmail] = useState<string | null>(getPersistedEmail());
  const [selectedBoard, setSelectedBoard] = useState<Board | null>(null);

  // Fetch email from server on reload if not cached locally.
  useEffect(() => {
    if (authed && !email) {
      ayb.auth.me().then((u) => setEmail(u.email)).catch(() => {});
    }
  }, [authed, email]);

  function handleLogout() {
    ayb.clearTokens();
    clearPersistedTokens();
    setAuthed(false);
    setEmail(null);
    setSelectedBoard(null);
  }

  if (!authed) {
    return (
      <AuthForm
        onAuth={(e) => {
          setEmail(e);
          setAuthed(true);
        }}
      />
    );
  }

  if (selectedBoard) {
    return (
      <BoardView
        board={selectedBoard}
        onBack={() => setSelectedBoard(null)}
      />
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white border-b px-6 py-3 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-bold text-gray-900">Kanban Board</h1>
          <span className="text-xs text-gray-400">
            powered by Allyourbase
          </span>
        </div>
        <div className="flex items-center gap-3">
          {email && (
            <span data-testid="user-email" className="text-sm text-gray-500">
              {email}
            </span>
          )}
          <button
            onClick={handleLogout}
            className="text-sm text-gray-500 hover:text-gray-700 transition-colors"
          >
            Sign out
          </button>
        </div>
      </header>
      <BoardList onSelectBoard={setSelectedBoard} />
    </div>
  );
}
