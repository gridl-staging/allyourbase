import { Draggable } from "@hello-pangea/dnd";
import type { Card } from "../types";

interface Props {
  card: Card;
  index: number;
  onClick: () => void;
}

export default function KanbanCard({ card, index, onClick }: Props) {
  return (
    <Draggable draggableId={card.id} index={index}>
      {(provided, snapshot) => (
        <div
          ref={provided.innerRef}
          {...provided.draggableProps}
          {...provided.dragHandleProps}
          onClick={onClick}
          className={`bg-white rounded-lg p-3 shadow-sm hover:shadow-md transition-shadow cursor-pointer border border-gray-100 ${
            snapshot.isDragging ? "shadow-lg ring-2 ring-blue-200" : ""
          }`}
        >
          <p className="text-sm text-gray-900 font-medium">{card.title}</p>
          {card.description && (
            <p className="text-xs text-gray-400 mt-1 line-clamp-2">
              {card.description}
            </p>
          )}
        </div>
      )}
    </Draggable>
  );
}
