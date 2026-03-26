<!-- audited 2026-03-20 -->

# Tutorial: Realtime Kanban Board

Build a collaborative Kanban board (Trello-lite) with Allyourbase. This tutorial exercises all major features: REST API, Auth, Realtime SSE, Row-Level Security, and foreign key relationships.

**Source code:** [examples/kanban/](https://github.com/gridlhq/allyourbase/tree/main/examples/kanban)

## What You'll Build

- User registration and login
- Create boards with columns and cards
- Drag-and-drop cards between columns
- Realtime sync across browser tabs via SSE
- Collaborative RLS — all authenticated users can read all boards, but only the board owner can modify or delete their own boards

## Prerequisites

- [Allyourbase installed](/guide/getting-started)
- Node.js 18+

## Quick Start

The fastest way to run the demo:

```bash
ayb demo kanban
```

Open `http://localhost:5173`, register an account, and start creating boards.

### Manual Setup

```bash
ayb start
ayb sql < examples/kanban/schema.sql
cd examples/kanban
npm install
npm run dev
```

## 1. Configure Allyourbase

Create `ayb.toml` with auth enabled. For local development, keep the API bound to
loopback and allow only the Vite dev origin:

```toml
[server]
host = "127.0.0.1"
port = 8090
cors_allowed_origins = ["http://localhost:5173"]

[auth]
enabled = true
jwt_secret = "replace-with-a-random-secret-at-least-32-chars-long"
```

Only widen `host` or `cors_allowed_origins` if you intentionally need LAN access.

Start AYB:

```bash
ayb start
```

## 2. Create the Schema

The Kanban board uses three tables: `boards`, `columns`, and `cards`.

```sql
CREATE TABLE IF NOT EXISTS boards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL CHECK (length(title) > 0),
  user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS columns (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  title TEXT NOT NULL CHECK (length(title) > 0),
  position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  column_id UUID NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  title TEXT NOT NULL CHECK (length(title) > 0),
  description TEXT DEFAULT '',
  position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);
```

Apply the schema:

```bash
ayb sql < schema.sql
```

AYB automatically detects the new tables and exposes REST endpoints for them.

## 3. Add Row-Level Security

The kanban demo uses **collaborative** RLS policies — all authenticated users can read all boards, columns, and cards, but only the board owner can modify or delete their own boards. Columns and cards are fully open for all authenticated users to create, update, and delete.

```sql
ALTER TABLE boards ENABLE ROW LEVEL SECURITY;
CREATE POLICY boards_select ON boards FOR SELECT USING (true);
CREATE POLICY boards_insert ON boards FOR INSERT WITH CHECK (
  user_id::text = current_setting('ayb.user_id', true)
);
CREATE POLICY boards_update ON boards FOR UPDATE USING (
  user_id::text = current_setting('ayb.user_id', true)
);
CREATE POLICY boards_delete ON boards FOR DELETE USING (
  user_id::text = current_setting('ayb.user_id', true)
);

ALTER TABLE columns ENABLE ROW LEVEL SECURITY;
CREATE POLICY columns_select ON columns FOR SELECT USING (true);
CREATE POLICY columns_insert ON columns FOR INSERT WITH CHECK (true);
CREATE POLICY columns_update ON columns FOR UPDATE USING (true);
CREATE POLICY columns_delete ON columns FOR DELETE USING (true);

ALTER TABLE cards ENABLE ROW LEVEL SECURITY;
CREATE POLICY cards_select ON cards FOR SELECT USING (true);
CREATE POLICY cards_insert ON cards FOR INSERT WITH CHECK (true);
CREATE POLICY cards_update ON cards FOR UPDATE USING (true);
CREATE POLICY cards_delete ON cards FOR DELETE USING (true);
```

AYB injects `ayb.user_id` into the Postgres session for every authenticated request, so the board ownership policies work automatically.

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

const TOKEN_KEY = "ayb_token";
const REFRESH_KEY = "ayb_refresh_token";
const EMAIL_KEY = "ayb_email";

export const ayb = new AYBClient(
  import.meta.env.VITE_AYB_URL ?? "http://localhost:8090",
);

// Restore tokens from localStorage on load
const savedToken = localStorage.getItem(TOKEN_KEY);
const savedRefresh = localStorage.getItem(REFRESH_KEY);
if (savedToken && savedRefresh) {
  ayb.setTokens(savedToken, savedRefresh);
}

export function persistTokens(email?: string) {
  if (ayb.token && ayb.refreshToken) {
    localStorage.setItem(TOKEN_KEY, ayb.token);
    localStorage.setItem(REFRESH_KEY, ayb.refreshToken);
  }
  if (email) localStorage.setItem(EMAIL_KEY, email);
}

export function clearPersistedTokens() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(REFRESH_KEY);
  localStorage.removeItem(EMAIL_KEY);
}

export function getPersistedEmail(): string | null {
  return localStorage.getItem(EMAIL_KEY);
}

export function isLoggedIn(): boolean {
  return ayb.token !== null;
}
```

## 6. Authentication

Use the SDK's auth methods:

```ts
// Register
await ayb.auth.register("user@example.com", "password123");
persistTokens("user@example.com");

// Login
await ayb.auth.login("user@example.com", "password123");
persistTokens("user@example.com");

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

Because the kanban schema uses collaborative `USING (true)` SELECT policies, all authenticated users receive events for all boards, columns, and cards. See [Realtime](/guide/realtime) for transport details and RLS filtering semantics.

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

## Next Steps

- [Realtime](/guide/realtime) — SSE and WebSocket transport details
- [File Storage](/guide/file-storage) — Add file attachments to cards
- [JavaScript SDK](/guide/javascript-sdk) — Full SDK reference
- [Deployment](/guide/deployment) — Deploy your Kanban board to production
