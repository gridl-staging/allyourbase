// Package templates Kanban defines a scaffold template for a collaborative kanban board system with PostgreSQL schema, seed data, TypeScript client helpers, and documentation.
package templates

type kanbanTemplate struct{}

func init() {
	Register(kanbanTemplate{})
}

func (kanbanTemplate) Name() string {
	return "kanban"
}

// Schema returns the PostgreSQL table definitions and row-level security policies for a collaborative kanban board system, including boards, columns, cards, labels, and their relationships with cascading deletes and owner-based access control.
func (kanbanTemplate) Schema() string {
	return `-- Kanban domain schema
-- Apply with: ayb sql < schema.sql

CREATE TABLE IF NOT EXISTS boards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    owner_id UUID REFERENCES _ayb_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS columns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    column_id UUID NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
    assignee_id UUID REFERENCES _ayb_users(id),
    due_date TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    color TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS card_labels (
    card_id UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (card_id, label_id)
);

ALTER TABLE boards ENABLE ROW LEVEL SECURITY;
ALTER TABLE columns ENABLE ROW LEVEL SECURITY;
ALTER TABLE cards ENABLE ROW LEVEL SECURITY;
ALTER TABLE labels ENABLE ROW LEVEL SECURITY;
ALTER TABLE card_labels ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS boards_select ON boards;
CREATE POLICY boards_select ON boards FOR SELECT
    USING (true);

DROP POLICY IF EXISTS boards_insert ON boards;
CREATE POLICY boards_insert ON boards FOR INSERT
    WITH CHECK (owner_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS boards_update ON boards;
CREATE POLICY boards_update ON boards FOR UPDATE
    USING (owner_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (owner_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS boards_delete ON boards;
CREATE POLICY boards_delete ON boards FOR DELETE
    USING (owner_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS columns_select ON columns;
CREATE POLICY columns_select ON columns FOR SELECT
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS columns_insert ON columns;
CREATE POLICY columns_insert ON columns FOR INSERT
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS columns_update ON columns;
CREATE POLICY columns_update ON columns FOR UPDATE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL)
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS columns_delete ON columns;
CREATE POLICY columns_delete ON columns FOR DELETE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS cards_select ON cards;
CREATE POLICY cards_select ON cards FOR SELECT
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS cards_insert ON cards;
CREATE POLICY cards_insert ON cards FOR INSERT
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS cards_update ON cards;
CREATE POLICY cards_update ON cards FOR UPDATE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL)
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS cards_delete ON cards;
CREATE POLICY cards_delete ON cards FOR DELETE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS labels_select ON labels;
CREATE POLICY labels_select ON labels FOR SELECT
    USING (true);

DROP POLICY IF EXISTS labels_insert ON labels;
CREATE POLICY labels_insert ON labels FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM boards b
            WHERE b.id = labels.board_id
              AND b.owner_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS labels_update ON labels;
CREATE POLICY labels_update ON labels FOR UPDATE
    USING (
        EXISTS (
            SELECT 1
            FROM boards b
            WHERE b.id = labels.board_id
              AND b.owner_id = current_setting('ayb.user_id', true)::uuid
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM boards b
            WHERE b.id = labels.board_id
              AND b.owner_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS labels_delete ON labels;
CREATE POLICY labels_delete ON labels FOR DELETE
    USING (
        EXISTS (
            SELECT 1
            FROM boards b
            WHERE b.id = labels.board_id
              AND b.owner_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS card_labels_select ON card_labels;
CREATE POLICY card_labels_select ON card_labels FOR SELECT
    USING (true);

DROP POLICY IF EXISTS card_labels_insert ON card_labels;
CREATE POLICY card_labels_insert ON card_labels FOR INSERT
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS card_labels_delete ON card_labels;
CREATE POLICY card_labels_delete ON card_labels FOR DELETE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);
`
}

// SeedData returns SQL INSERT statements that populate the kanban schema with example users, boards, columns, cards, labels, and card-label associations for testing and demonstration.
func (kanbanTemplate) SeedData() string {
	return `-- Kanban domain seed data
-- Apply with: ayb sql < seed.sql

INSERT INTO _ayb_users (id, email, password_hash)
VALUES
    ('31111111-1111-1111-1111-111111111111', 'kanban.owner@example.com', 'seeded-password-hash'),
    ('32222222-2222-2222-2222-222222222222', 'kanban.member@example.com', 'seeded-password-hash')
ON CONFLICT DO NOTHING;

INSERT INTO boards (id, name, owner_id)
VALUES
    ('41000000-0000-0000-0000-000000000001', 'Platform Sprint Board', '31111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

INSERT INTO columns (id, board_id, name, position)
VALUES
    ('42000000-0000-0000-0000-000000000001', '41000000-0000-0000-0000-000000000001', 'Backlog', 0),
    ('42000000-0000-0000-0000-000000000002', '41000000-0000-0000-0000-000000000001', 'Todo', 1),
    ('42000000-0000-0000-0000-000000000003', '41000000-0000-0000-0000-000000000001', 'In Progress', 2),
    ('42000000-0000-0000-0000-000000000004', '41000000-0000-0000-0000-000000000001', 'Done', 3)
ON CONFLICT DO NOTHING;

INSERT INTO cards (id, column_id, title, description, position, assignee_id, due_date)
VALUES
    ('43000000-0000-0000-0000-000000000001', '42000000-0000-0000-0000-000000000001', 'Design webhook retry strategy', 'Define retry policy and dead-letter behavior.', 0, '31111111-1111-1111-1111-111111111111', now() + interval '4 days'),
    ('43000000-0000-0000-0000-000000000002', '42000000-0000-0000-0000-000000000001', 'Draft rollout checklist', 'Create launch checklist for beta users.', 1, '32222222-2222-2222-2222-222222222222', now() + interval '6 days'),
    ('43000000-0000-0000-0000-000000000003', '42000000-0000-0000-0000-000000000002', 'Implement board permissions', 'Add board owner policy checks.', 0, '31111111-1111-1111-1111-111111111111', now() + interval '3 days'),
    ('43000000-0000-0000-0000-000000000004', '42000000-0000-0000-0000-000000000002', 'Create SDK examples', 'Publish usage snippets for API docs.', 1, '32222222-2222-2222-2222-222222222222', NULL),
    ('43000000-0000-0000-0000-000000000005', '42000000-0000-0000-0000-000000000003', 'Refactor template registry', 'Eliminate duplicate template declarations.', 0, '31111111-1111-1111-1111-111111111111', now() + interval '2 days'),
    ('43000000-0000-0000-0000-000000000006', '42000000-0000-0000-0000-000000000003', 'Add integration tests', 'Cover scaffolding output with fixtures.', 1, '32222222-2222-2222-2222-222222222222', now() + interval '1 day'),
    ('43000000-0000-0000-0000-000000000007', '42000000-0000-0000-0000-000000000003', 'Tune SQL indexes', 'Review high-frequency query paths.', 2, NULL, NULL),
    ('43000000-0000-0000-0000-000000000008', '42000000-0000-0000-0000-000000000004', 'Ship stage 1', 'Finalize stage 1 deliverables.', 0, '31111111-1111-1111-1111-111111111111', now() - interval '1 day'),
    ('43000000-0000-0000-0000-000000000009', '42000000-0000-0000-0000-000000000004', 'Publish release notes', 'Document stage outcomes and risks.', 1, '32222222-2222-2222-2222-222222222222', now() - interval '2 days'),
    ('43000000-0000-0000-0000-000000000010', '42000000-0000-0000-0000-000000000001', 'Investigate flaky CI test', 'Root-cause intermittent test timeout.', 2, NULL, now() + interval '5 days')
ON CONFLICT DO NOTHING;

INSERT INTO labels (id, board_id, name, color)
VALUES
    ('44000000-0000-0000-0000-000000000001', '41000000-0000-0000-0000-000000000001', 'Bug', '#ef4444'),
    ('44000000-0000-0000-0000-000000000002', '41000000-0000-0000-0000-000000000001', 'Feature', '#3b82f6'),
    ('44000000-0000-0000-0000-000000000003', '41000000-0000-0000-0000-000000000001', 'Urgent', '#f59e0b'),
    ('44000000-0000-0000-0000-000000000004', '41000000-0000-0000-0000-000000000001', 'Design', '#a855f7'),
    ('44000000-0000-0000-0000-000000000005', '41000000-0000-0000-0000-000000000001', 'Backend', '#10b981')
ON CONFLICT DO NOTHING;

INSERT INTO card_labels (card_id, label_id)
VALUES
    ('43000000-0000-0000-0000-000000000003', '44000000-0000-0000-0000-000000000005'),
    ('43000000-0000-0000-0000-000000000004', '44000000-0000-0000-0000-000000000002'),
    ('43000000-0000-0000-0000-000000000005', '44000000-0000-0000-0000-000000000005'),
    ('43000000-0000-0000-0000-000000000006', '44000000-0000-0000-0000-000000000003'),
    ('43000000-0000-0000-0000-000000000007', '44000000-0000-0000-0000-000000000001'),
    ('43000000-0000-0000-0000-000000000008', '44000000-0000-0000-0000-000000000002'),
    ('43000000-0000-0000-0000-000000000010', '44000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;
`
}

// ClientCode returns a map of TypeScript source files containing domain interfaces and helper functions for interacting with kanban records through the AYB client API, including board, column, card, and label operations.
func (kanbanTemplate) ClientCode() map[string]string {
	return map[string]string{
		"src/lib/kanban.ts": `import { ayb } from "./ayb";

export interface Board {
  id: string;
  name: string;
  owner_id: string;
  created_at: string;
}

export interface Column {
  id: string;
  board_id: string;
  name: string;
  position: number;
  created_at: string;
}

export interface Card {
  id: string;
  column_id: string;
  title: string;
  description: string;
  position: number;
  assignee_id: string | null;
  due_date: string | null;
  created_at: string;
  updated_at: string;
}

export interface Label {
  id: string;
  board_id: string;
  name: string;
  color: string;
  created_at: string;
}

export interface CreateBoardInput {
  name: string;
  owner_id: string;
}

export interface CreateColumnInput {
  name: string;
  position?: number;
}

export interface CreateCardInput {
  title: string;
  description?: string;
  position?: number;
  assignee_id?: string | null;
  due_date?: string | null;
}

export function listBoards() {
  return ayb.records.list("boards", { sort: "created_at" });
}

export function createBoard(data: CreateBoardInput) {
  return ayb.records.create("boards", data);
}

export function listColumns(boardId: string) {
  return ayb.records.list("columns", {
    filter: "board_id='" + boardId + "'",
    sort: "position",
  });
}

export function createColumn(boardId: string, data: CreateColumnInput) {
  return ayb.records.create("columns", {
    board_id: boardId,
    ...data,
  });
}

export function listCards(columnId: string) {
  return ayb.records.list("cards", {
    filter: "column_id='" + columnId + "'",
    sort: "position",
  });
}

export function createCard(columnId: string, data: CreateCardInput) {
  return ayb.records.create("cards", {
    column_id: columnId,
    description: "",
    ...data,
  });
}

export function moveCard(id: string, columnId: string, position: number) {
  return ayb.records.update("cards", id, {
    column_id: columnId,
    position,
  });
}

export function listLabels(boardId: string) {
  return ayb.records.list("labels", {
    filter: "board_id='" + boardId + "'",
    sort: "name",
  });
}

export function addLabel(cardId: string, labelId: string) {
  return ayb.records.create("card_labels", {
    card_id: cardId,
    label_id: labelId,
  });
}
`,
	}
}

// Readme returns formatted documentation describing the kanban template's schema structure, installation instructions, SDK usage examples, and quick start guide.
func (kanbanTemplate) Readme() string {
	return `# Kanban Template

This scaffold provisions a collaborative kanban schema and helper client code.

## Included schema

- ` + "`boards`" + `: board container and owner metadata
- ` + "`columns`" + `: ordered lanes per board (Backlog/Todo/In Progress/Done)
- ` + "`cards`" + `: work items with assignee and optional due date
- ` + "`labels`" + ` and ` + "`card_labels`" + `: board labels and card assignments

## Apply schema and seed data

` + "```bash" + `
ayb sql < schema.sql && ayb sql < seed.sql
` + "```" + `

## SDK usage example

` + "```ts" + `
import {
  createBoard,
  listColumns,
  createCard,
  moveCard,
} from "./src/lib/kanban";

const board = await createBoard({
  name: "Q2 Platform",
  owner_id: "<current-user-id>",
});
const { items: columns } = await listColumns(board.id);
const card = await createCard(columns[0].id, { title: "Investigate incident" });
await moveCard(card.id, columns[1].id, 0);
` + "```" + `

## Quick start

1. Start AYB with ` + "`ayb start`" + `.
2. Apply schema and seed data.
3. Use ` + "`src/lib/kanban.ts`" + ` helpers to build board workflows.
`
}
