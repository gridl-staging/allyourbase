# Tutorial: Realtime Kanban Board

Build a collaborative Kanban board (Trello-lite) with Allyourbase. This tutorial exercises all major features: REST API, Auth, Realtime SSE, Row-Level Security, and foreign key relationships.

**Source code:** [examples/kanban/](https://github.com/gridlhq/allyourbase/tree/main/examples/kanban)

## What You'll Build

- User registration and login
- Create boards with columns and cards
- Drag-and-drop cards between columns
- Realtime sync across browser tabs via SSE
- Per-user data isolation via Postgres RLS

## Prerequisites

- [Allyourbase installed](/guide/getting-started)
- Node.js 18+

## 1. Configure Allyourbase

Create `ayb.toml` with auth enabled:

```toml
[server]
host = "0.0.0.0"
port = 8090
cors_allowed_origins = ["*"]

[auth]
enabled = true
jwt_secret = "change-me-to-a-secret-at-least-32-chars-long!!"
```

Start AYB:

```bash
ayb start
```

## 2. Create the Schema

The Kanban board uses three tables: `boards`, `columns`, and `cards`.

```sql
CREATE TABLE boards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL,
  user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE columns (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  position INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE cards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  column_id UUID NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT DEFAULT '',
  position INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);
```

Run this against the managed Postgres:

```bash
psql "postgresql://ayb:ayb@localhost:15432/ayb" -f schema.sql
```

AYB automatically detects the new tables and exposes REST endpoints for them.

## 3. Add Row-Level Security

RLS ensures each user only sees their own boards and nested data:

```sql
ALTER TABLE boards ENABLE ROW LEVEL SECURITY;
CREATE POLICY boards_owner ON boards
  FOR ALL USING (user_id::text = current_setting('ayb.user_id', true));

ALTER TABLE columns ENABLE ROW LEVEL SECURITY;
CREATE POLICY columns_owner ON columns
  FOR ALL USING (board_id IN (
    SELECT id FROM boards
    WHERE user_id::text = current_setting('ayb.user_id', true)
  ));

ALTER TABLE cards ENABLE ROW LEVEL SECURITY;
CREATE POLICY cards_owner ON cards
  FOR ALL USING (column_id IN (
    SELECT id FROM columns WHERE board_id IN (
      SELECT id FROM boards
      WHERE user_id::text = current_setting('ayb.user_id', true)
    )
  ));
```

AYB injects `ayb.user_id` into the Postgres session for every authenticated request, so these policies work automatically.

## 4. Set Up the Frontend

```bash
mkdir kanban && cd kanban
npm init -y
npm install @allyourbase/js @hello-pangea/dnd react react-dom
npm install -D @types/react @types/react-dom @vitejs/plugin-react typescript vite tailwindcss autoprefixer postcss
```

## 5. Initialize the SDK

```ts
// src/lib/ayb.ts
import { AYBClient } from "@allyourbase/js";

export const ayb = new AYBClient("http://localhost:8090");

// Persist tokens in localStorage
export function persistTokens() {
  if (ayb.token && ayb.refreshToken) {
    localStorage.setItem("ayb_token", ayb.token);
    localStorage.setItem("ayb_refresh_token", ayb.refreshToken);
  }
}
```

## 6. Authentication

Use the SDK's auth methods:

```ts
// Register
await ayb.auth.register("user@example.com", "password123");
persistTokens();

// Login
await ayb.auth.login("user@example.com", "password123");
persistTokens();

// Get current user
const me = await ayb.auth.me();
```

## 7. CRUD Operations

The SDK maps directly to AYB's REST API:

```ts
// Create a board
const board = await ayb.records.create("boards", {
  title: "My Board",
  user_id: me.id,
});

// Create a column
const column = await ayb.records.create("columns", {
  board_id: board.id,
  title: "To Do",
  position: 0,
});

// Create a card
const card = await ayb.records.create("cards", {
  column_id: column.id,
  title: "First task",
  position: 0,
});

// List cards in a column, sorted by position
const { items: cards } = await ayb.records.list("cards", {
  filter: `column_id='${column.id}'`,
  sort: "position",
});

// Move a card to a different column
await ayb.records.update("cards", card.id, {
  column_id: otherColumn.id,
  position: 0,
});

// Delete a card
await ayb.records.delete("cards", card.id);
```

## 8. Realtime Updates

Subscribe to card and column changes via SSE:

```ts
const unsub = ayb.realtime.subscribe(["cards", "columns"], (event) => {
  if (event.action === "create") {
    // A new card/column was created — add it to the UI
  }
  if (event.action === "update") {
    // A card was moved or edited — update the UI
  }
  if (event.action === "delete") {
    // A card/column was deleted — remove from UI
  }
});
```

Events are filtered by RLS — users only receive events for their own data.

## 9. Drag-and-Drop

Using `@hello-pangea/dnd`:

```tsx
import { DragDropContext, Droppable, Draggable } from "@hello-pangea/dnd";

function Board() {
  async function handleDragEnd(result) {
    const { source, destination, draggableId } = result;
    if (!destination) return;

    // Optimistically update the UI
    moveCardLocally(draggableId, destination.droppableId, destination.index);

    // Persist to AYB
    await ayb.records.update("cards", draggableId, {
      column_id: destination.droppableId,
      position: destination.index,
    });
  }

  return (
    <DragDropContext onDragEnd={handleDragEnd}>
      {columns.map((col) => (
        <Droppable key={col.id} droppableId={col.id}>
          {(provided) => (
            <div ref={provided.innerRef} {...provided.droppableProps}>
              {cards
                .filter((c) => c.column_id === col.id)
                .map((card, i) => (
                  <Draggable key={card.id} draggableId={card.id} index={i}>
                    {(provided) => (
                      <div
                        ref={provided.innerRef}
                        {...provided.draggableProps}
                        {...provided.dragHandleProps}
                      >
                        {card.title}
                      </div>
                    )}
                  </Draggable>
                ))}
              {provided.placeholder}
            </div>
          )}
        </Droppable>
      ))}
    </DragDropContext>
  );
}
```

## Running the Full Demo

```bash
# Clone the repo
git clone https://github.com/gridlhq/allyourbase.git
cd allyourbase/examples/kanban

# Start AYB (in another terminal)
ayb start

# Create tables
psql "postgresql://ayb:ayb@localhost:15432/ayb" -f schema.sql

# Install and run
npm install
npm run dev
```

Open `http://localhost:5173` and start building your Kanban board!

## Next Steps

- [File Storage](/guide/file-storage) — Add file attachments to cards
- [Database RPC](/guide/database-rpc) — Add aggregate functions (card counts, board stats)
- [Deployment](/guide/deployment) — Deploy your Kanban board to production
