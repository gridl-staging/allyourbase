import { useCallback, useEffect, useState } from "react";
import { DragDropContext, type DropResult } from "@hello-pangea/dnd";
import { ayb } from "../lib/ayb";
import { useRealtime } from "../hooks/useRealtime";
import type { Board, Card, Column } from "../types";
import KanbanColumn from "./KanbanColumn";
import CardModal from "./CardModal";

interface Props {
  board: Board;
  onBack: () => void;
}

export default function BoardView({ board, onBack }: Props) {
  const [columns, setColumns] = useState<Column[]>([]);
  const [cards, setCards] = useState<Card[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingCard, setEditingCard] = useState<Card | null>(null);
  const [newColTitle, setNewColTitle] = useState("");
  const [addingCol, setAddingCol] = useState(false);

  useEffect(() => {
    loadBoard();
  }, [board.id]);

  async function loadBoard() {
    try {
      const colRes = await ayb.records.list<Column>("columns", {
        filter: `board_id='${board.id}'`,
        sort: "position",
        perPage: 100,
      });
      setColumns(colRes.items);

      if (colRes.items.length > 0) {
        const colIds = colRes.items.map((c) => `'${c.id}'`).join(",");
        const cardRes = await ayb.records.list<Card>("cards", {
          filter: `column_id IN (${colIds})`,
          sort: "position",
          perPage: 500,
        });
        setCards(cardRes.items);
      }
    } catch (err) {
      console.error("Failed to load board:", err);
    } finally {
      setLoading(false);
    }
  }

  // Realtime: update local state when SSE events arrive.
  // Filter events to only process those belonging to the current board.
  const handleRealtime = useCallback(
    (event: { action: string; table: string; record: Record<string, unknown> }) => {
      if (event.table === "cards") {
        const card = event.record as unknown as Card;
        setCards((prev) => {
          if (event.action === "create") {
            if (prev.find((c) => c.id === card.id)) return prev;
            return [...prev, card];
          }
          if (event.action === "update") {
            // Only update cards already tracked (belonging to this board).
            if (!prev.find((c) => c.id === card.id)) return prev;
            return prev.map((c) => (c.id === card.id ? card : c));
          }
          if (event.action === "delete") {
            return prev.filter((c) => c.id !== card.id);
          }
          return prev;
        });
      }
      if (event.table === "columns") {
        const col = event.record as unknown as Column;
        if (event.action === "delete") {
          // Delete events only contain PK fields (no board_id), so just
          // remove the column if it's currently tracked.
          setColumns((prev) => prev.filter((c) => c.id !== col.id));
          return;
        }
        // Ignore columns from other boards.
        if (col.board_id !== board.id) return;
        setColumns((prev) => {
          if (event.action === "create") {
            if (prev.find((c) => c.id === col.id)) return prev;
            return [...prev, col].sort((a, b) => a.position - b.position);
          }
          if (event.action === "update") {
            return prev
              .map((c) => (c.id === col.id ? col : c))
              .sort((a, b) => a.position - b.position);
          }
          return prev;
        });
      }
    },
    [board.id],
  );

  useRealtime(["cards", "columns"], handleRealtime);

  async function addColumn(e: React.FormEvent) {
    e.preventDefault();
    if (!newColTitle.trim()) return;
    setAddingCol(true);
    try {
      const col = await ayb.records.create<Column>("columns", {
        board_id: board.id,
        title: newColTitle.trim(),
        position: columns.length,
      });
      setColumns([...columns, col]);
      setNewColTitle("");
    } catch (err) {
      console.error("Failed to create column:", err);
    } finally {
      setAddingCol(false);
    }
  }

  function handleDeleteColumn(columnId: string) {
    setColumns(columns.filter((c) => c.id !== columnId));
    setCards(cards.filter((c) => c.column_id !== columnId));
  }

  function handleCardCreated(card: Card) {
    setCards([...cards, card]);
  }

  function handleCardUpdate(updated: Card) {
    setCards(cards.map((c) => (c.id === updated.id ? updated : c)));
  }

  function handleCardDelete(cardId: string) {
    setCards(cards.filter((c) => c.id !== cardId));
  }

  async function handleDragEnd(result: DropResult) {
    const { source, destination, draggableId } = result;
    if (!destination) return;
    if (
      source.droppableId === destination.droppableId &&
      source.index === destination.index
    )
      return;

    const card = cards.find((c) => c.id === draggableId);
    if (!card) return;

    // Optimistic update
    const newCards = cards.filter((c) => c.id !== draggableId);
    const destCards = newCards
      .filter((c) => c.column_id === destination.droppableId)
      .sort((a, b) => a.position - b.position);

    const updatedCard: Card = {
      ...card,
      column_id: destination.droppableId,
      position: destination.index,
    };

    // Insert at destination index
    destCards.splice(destination.index, 0, updatedCard);

    // Recalculate positions for all cards in the destination column
    const positionUpdates = destCards.map((c, i) => ({ ...c, position: i }));

    // Apply optimistic update
    const finalCards = newCards
      .filter((c) => c.column_id !== destination.droppableId)
      .concat(positionUpdates);
    setCards(finalCards);

    // Persist to server
    try {
      await ayb.records.update("cards", card.id, {
        column_id: destination.droppableId,
        position: destination.index,
      });

      // Update positions of other cards in the destination column
      for (const c of positionUpdates) {
        if (c.id !== card.id && c.position !== cards.find((x) => x.id === c.id)?.position) {
          await ayb.records.update("cards", c.id, { position: c.position });
        }
      }
    } catch (err) {
      console.error("Failed to move card:", err);
      loadBoard(); // Revert on error
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-gray-500">Loading board...</p>
      </div>
    );
  }

  return (
    <div className="h-screen flex flex-col">
      {/* Header */}
      <div className="bg-white border-b px-6 py-3 flex items-center gap-4 flex-shrink-0">
        <button
          onClick={onBack}
          aria-label="Back to boards"
          className="text-gray-500 hover:text-gray-700 transition-colors"
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
              d="M15 19l-7-7 7-7"
            />
          </svg>
        </button>
        <h1 className="text-lg font-bold text-gray-900">{board.title}</h1>
        <span className="text-xs bg-green-100 text-green-700 px-2 py-0.5 rounded-full font-medium">
          Live
        </span>
      </div>

      {/* Board */}
      <div className="flex-1 overflow-x-auto p-6">
        <DragDropContext onDragEnd={handleDragEnd}>
          <div className="flex gap-4 items-start h-full">
            {columns.map((col) => (
              <KanbanColumn
                key={col.id}
                column={col}
                cards={cards
                  .filter((c) => c.column_id === col.id)
                  .sort((a, b) => a.position - b.position)}
                onCardClick={setEditingCard}
                onCardCreated={handleCardCreated}
                onDeleteColumn={handleDeleteColumn}
              />
            ))}

            {/* Add Column */}
            <div className="w-72 flex-shrink-0">
              <form
                onSubmit={addColumn}
                className="bg-gray-50 border-2 border-dashed border-gray-200 rounded-xl p-3"
              >
                <input
                  type="text"
                  value={newColTitle}
                  onChange={(e) => setNewColTitle(e.target.value)}
                  placeholder="+ Add column..."
                  className="w-full px-3 py-2 text-sm bg-transparent border-none focus:outline-none placeholder:text-gray-400"
                />
                {newColTitle.trim() && (
                  <button
                    type="submit"
                    disabled={addingCol}
                    className="mt-2 w-full bg-blue-600 text-white text-sm px-3 py-1.5 rounded-lg font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
                  >
                    {addingCol ? "Adding..." : "Add Column"}
                  </button>
                )}
              </form>
            </div>
          </div>
        </DragDropContext>
      </div>

      {/* Card Edit Modal */}
      {editingCard && (
        <CardModal
          card={editingCard}
          onClose={() => setEditingCard(null)}
          onUpdate={handleCardUpdate}
          onDelete={handleCardDelete}
        />
      )}
    </div>
  );
}
