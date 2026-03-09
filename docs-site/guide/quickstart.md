# Quickstart: Build a Todo App

Build a working CRUD app with Allyourbase in 5 minutes.

## 1. Start AYB

```bash
# Install
curl -fsSL https://allyourbase.io/install.sh | sh

# Start with managed PostgreSQL (zero config)
ayb start
```

AYB is now running at `http://localhost:8090`.

## 2. Create a todos table

Open another terminal and create the table:

```bash
ayb sql "CREATE TABLE todos (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  completed BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT now()
)"
```

Or create it via the admin dashboard at `http://localhost:8090/admin`.

## 3. Set up the project

```bash
mkdir todo-app && cd todo-app
npm init -y
npm install @allyourbase/js
```

## 4. Write the app

Create `index.mjs`:

```js
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");

// Create todos
await ayb.records.create("todos", { title: "Buy groceries" });
await ayb.records.create("todos", { title: "Write docs", completed: true });
await ayb.records.create("todos", { title: "Ship v1" });

// List all todos
const { items: all } = await ayb.records.list("todos", {
  sort: "-created_at",
});
console.log("All todos:", all);

// Filter: only incomplete
const { items: pending } = await ayb.records.list("todos", {
  filter: "completed=false",
});
console.log("Pending:", pending);

// Update: mark one as done
const todo = pending[0];
await ayb.records.update("todos", String(todo.id), { completed: true });
console.log(`Marked "${todo.title}" as done`);

// Delete
await ayb.records.delete("todos", String(todo.id));
console.log(`Deleted "${todo.title}"`);

// Final state
const { items: final } = await ayb.records.list("todos");
console.log("Remaining:", final);
```

## 5. Run it

```bash
node index.mjs
```

Output:

```
All todos: [ { id: 3, title: 'Ship v1', ... }, { id: 2, title: 'Write docs', ... }, { id: 1, title: 'Buy groceries', ... } ]
Pending: [ { id: 3, title: 'Ship v1', completed: false }, { id: 1, title: 'Buy groceries', completed: false } ]
Marked "Ship v1" as done
Deleted "Ship v1"
Remaining: [ { id: 2, title: 'Write docs', ... }, { id: 1, title: 'Buy groceries', ... } ]
```

## 6. Add realtime

Subscribe to changes from another process:

```js
import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");

const unsubscribe = ayb.realtime.subscribe(["todos"], (event) => {
  console.log(`[${event.action}]`, event.record);
});

console.log("Listening for todo changes... (Ctrl-C to stop)");
// Keep the process alive
await new Promise(() => {});
```

Now create/update/delete todos in another terminal or via the admin dashboard — you'll see the events stream in.

## Next steps

- [Authentication](/guide/authentication) — Add user auth and per-user todos with RLS
- [File Storage](/guide/file-storage) — Attach files to your records
- [Deployment](/guide/deployment) — Deploy to production with Docker or bare metal
