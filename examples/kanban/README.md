# Kanban Board Demo

A realtime collaborative Kanban board (Trello-lite) built with Allyourbase.

## Quick Start

```bash
ayb demo kanban
```

Open http://localhost:5173, register an account, and start creating boards!

### Manual Setup

```bash
ayb start
ayb sql < schema.sql
npm install
npm run dev
```

## Features

### Drag & Drop
Move cards between columns. Changes persist instantly and sync across tabs via SSE.

### Realtime
Open two browser tabs. Create or move a card in one — it appears in the other instantly.

### Row-Level Security
Register two different users. Each user only sees their own boards. This is enforced at the Postgres level via RLS policies.

## Demonstrates

- **REST API** — CRUD operations on boards, columns, and cards
- **Authentication** — Email/password registration and login with JWT
- **Row-Level Security** — Users only see their own boards
- **Realtime SSE** — Live updates when cards are created, moved, or deleted
- **Foreign Key relationships** — Boards → Columns → Cards with cascading deletes
- **Drag-and-drop** — Move cards between columns with instant persistence

## Tech Stack

- React 18 + TypeScript + Vite
- Tailwind CSS
- [@allyourbase/js](https://www.npmjs.com/package/@allyourbase/js) SDK
- [@hello-pangea/dnd](https://github.com/hello-pangea/dnd) for drag-and-drop

## Schema

See [schema.sql](./schema.sql) for the complete database schema including RLS policies.

```
boards (id, title, user_id)
  └── columns (id, board_id, title, position)
       └── cards (id, column_id, title, description, position)
```
