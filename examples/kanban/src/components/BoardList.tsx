import { useEffect, useState } from "react";
import { ayb } from "../lib/ayb";
import type { Board } from "../types";

interface Props {
  onSelectBoard: (board: Board) => void;
}

export default function BoardList({ onSelectBoard }: Props) {
  const [boards, setBoards] = useState<Board[]>([]);
  const [loading, setLoading] = useState(true);
  const [newTitle, setNewTitle] = useState("");
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    loadBoards();
  }, []);

  async function loadBoards() {
    try {
      const res = await ayb.records.list<Board>("boards", {
        sort: "-created_at",
      });
      setBoards(res.items);
    } catch (err) {
      console.error("Failed to load boards:", err);
    } finally {
      setLoading(false);
    }
  }

  async function createBoard(e: React.FormEvent) {
    e.preventDefault();
    if (!newTitle.trim()) return;
    setCreating(true);
    try {
      const me = await ayb.auth.me();
      const board = await ayb.records.create<Board>("boards", {
        title: newTitle.trim(),
        user_id: me.id,
      });
      setBoards([board, ...boards]);
      setNewTitle("");
    } catch (err) {
      console.error("Failed to create board:", err);
    } finally {
      setCreating(false);
    }
  }

  async function deleteBoard(board: Board, e: React.MouseEvent) {
    e.stopPropagation();
    if (!confirm(`Delete "${board.title}"?`)) return;
    try {
      await ayb.records.delete("boards", board.id);
      setBoards(boards.filter((b) => b.id !== board.id));
    } catch (err) {
      console.error("Failed to delete board:", err);
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-gray-500">Loading boards...</p>
      </div>
    );
  }

  return (
    <div className="max-w-2xl mx-auto py-8 px-4">
      <h2 className="text-xl font-bold text-gray-900 mb-6">Your Boards</h2>

      <form onSubmit={createBoard} className="flex gap-2 mb-6">
        <input
          type="text"
          value={newTitle}
          onChange={(e) => setNewTitle(e.target.value)}
          placeholder="New board name..."
          className="flex-1 px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        <button
          type="submit"
          disabled={creating || !newTitle.trim()}
          className="bg-blue-600 text-white px-4 py-2 rounded-lg font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
        >
          {creating ? "..." : "Create"}
        </button>
      </form>

      {boards.length === 0 ? (
        <div className="text-center py-12 bg-white rounded-xl border-2 border-dashed border-gray-200">
          <p className="text-gray-500 text-lg">No boards yet</p>
          <p className="text-gray-400 text-sm mt-1">
            Create your first board above
          </p>
        </div>
      ) : (
        <div className="grid gap-3">
          {boards.map((board) => (
            <button
              key={board.id}
              onClick={() => onSelectBoard(board)}
              aria-label={`Open board ${board.title}`}
              className="flex items-center justify-between bg-white p-4 rounded-xl shadow-sm hover:shadow-md transition-shadow text-left group"
            >
              <div>
                <h3 className="font-semibold text-gray-900">{board.title}</h3>
                <p className="text-xs text-gray-400 mt-1">
                  {new Date(board.created_at).toLocaleDateString()}
                </p>
              </div>
              <button
                onClick={(e) => deleteBoard(board, e)}
                aria-label={`Delete board ${board.title}`}
                className="text-gray-300 hover:text-red-500 opacity-0 group-hover:opacity-100 transition-all p-1"
                title="Delete board"
              >
                <svg
                  className="w-5 h-5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                  />
                </svg>
              </button>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
