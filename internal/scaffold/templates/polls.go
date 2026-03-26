// Package templates The polls template scaffolds a voting system with PostgreSQL schema definitions, seed data, TypeScript client helpers, and documentation.
package templates

type pollsTemplate struct{}

func init() {
	Register(pollsTemplate{})
}

func (pollsTemplate) Name() string {
	return "polls"
}

// Schema returns SQL statements creating the polls domain with tables for polls, poll options, and votes, including row-level security policies enforcing creator ownership constraints.
func (pollsTemplate) Schema() string {
	return `-- Polls domain schema
-- Apply with: ayb sql < schema.sql

CREATE TABLE IF NOT EXISTS polls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    creator_id UUID NOT NULL REFERENCES _ayb_users(id),
    multiple_choice BOOLEAN NOT NULL DEFAULT false,
    closes_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS poll_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (poll_id, id)
);

CREATE TABLE IF NOT EXISTS votes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id UUID NOT NULL,
    user_id UUID NOT NULL REFERENCES _ayb_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (poll_id, option_id) REFERENCES poll_options(poll_id, id) ON DELETE CASCADE,
    UNIQUE (poll_id, user_id)
);

ALTER TABLE polls ENABLE ROW LEVEL SECURITY;
ALTER TABLE poll_options ENABLE ROW LEVEL SECURITY;
ALTER TABLE votes ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS polls_select ON polls;
CREATE POLICY polls_select ON polls FOR SELECT
    USING (true);

DROP POLICY IF EXISTS polls_insert ON polls;
CREATE POLICY polls_insert ON polls FOR INSERT
    WITH CHECK (creator_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS polls_update ON polls;
CREATE POLICY polls_update ON polls FOR UPDATE
    USING (creator_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (creator_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS polls_delete ON polls;
CREATE POLICY polls_delete ON polls FOR DELETE
    USING (creator_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS poll_options_select ON poll_options;
CREATE POLICY poll_options_select ON poll_options FOR SELECT
    USING (true);

DROP POLICY IF EXISTS poll_options_insert ON poll_options;
CREATE POLICY poll_options_insert ON poll_options FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM polls p
            WHERE p.id = poll_options.poll_id
              AND p.creator_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS poll_options_update ON poll_options;
CREATE POLICY poll_options_update ON poll_options FOR UPDATE
    USING (
        EXISTS (
            SELECT 1
            FROM polls p
            WHERE p.id = poll_options.poll_id
              AND p.creator_id = current_setting('ayb.user_id', true)::uuid
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM polls p
            WHERE p.id = poll_options.poll_id
              AND p.creator_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS poll_options_delete ON poll_options;
CREATE POLICY poll_options_delete ON poll_options FOR DELETE
    USING (
        EXISTS (
            SELECT 1
            FROM polls p
            WHERE p.id = poll_options.poll_id
              AND p.creator_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS votes_select ON votes;
CREATE POLICY votes_select ON votes FOR SELECT
    USING (true);

DROP POLICY IF EXISTS votes_insert ON votes;
CREATE POLICY votes_insert ON votes FOR INSERT
    WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS votes_update ON votes;
DROP POLICY IF EXISTS votes_delete ON votes;
`
}

// SeedData returns SQL insert statements that populate the polls domain with sample users, polls, options, and votes for demonstration purposes.
func (pollsTemplate) SeedData() string {
	return `-- Polls domain seed data
-- Apply with: ayb sql < seed.sql

INSERT INTO _ayb_users (id, email, password_hash)
VALUES
    ('81111111-1111-1111-1111-111111111111', 'polls.alex@example.com', 'seeded-password-hash'),
    ('82222222-2222-2222-2222-222222222222', 'polls.sam@example.com', 'seeded-password-hash'),
    ('83333333-3333-3333-3333-333333333333', 'polls.jamie@example.com', 'seeded-password-hash')
ON CONFLICT DO NOTHING;

INSERT INTO polls (id, title, description, creator_id, multiple_choice, closes_at)
VALUES
    ('91000000-0000-0000-0000-000000000001', 'Preferred API Pagination Style', 'Pick the default pagination strategy for v2 endpoints.', '81111111-1111-1111-1111-111111111111', false, NULL),
    ('91000000-0000-0000-0000-000000000002', 'Roadmap Priorities', 'Which themes should we emphasize in the next milestone?', '82222222-2222-2222-2222-222222222222', true, NULL),
    ('91000000-0000-0000-0000-000000000003', 'Retrospective Time Slot', 'Choose a time slot for the team retrospective.', '83333333-3333-3333-3333-333333333333', false, now() - interval '2 days')
ON CONFLICT DO NOTHING;

INSERT INTO poll_options (id, poll_id, text, position)
VALUES
    ('92000000-0000-0000-0000-000000000001', '91000000-0000-0000-0000-000000000001', 'Offset + limit', 0),
    ('92000000-0000-0000-0000-000000000002', '91000000-0000-0000-0000-000000000001', 'Cursor based', 1),
    ('92000000-0000-0000-0000-000000000003', '91000000-0000-0000-0000-000000000001', 'Keyset pagination', 2),
    ('92000000-0000-0000-0000-000000000004', '91000000-0000-0000-0000-000000000002', 'Developer experience', 0),
    ('92000000-0000-0000-0000-000000000005', '91000000-0000-0000-0000-000000000002', 'Realtime collaboration', 1),
    ('92000000-0000-0000-0000-000000000006', '91000000-0000-0000-0000-000000000002', 'Enterprise controls', 2),
    ('92000000-0000-0000-0000-000000000007', '91000000-0000-0000-0000-000000000003', 'Tuesday 10:00', 0),
    ('92000000-0000-0000-0000-000000000008', '91000000-0000-0000-0000-000000000003', 'Wednesday 14:00', 1),
    ('92000000-0000-0000-0000-000000000009', '91000000-0000-0000-0000-000000000003', 'Friday 09:00', 2)
ON CONFLICT DO NOTHING;

INSERT INTO votes (id, poll_id, option_id, user_id)
VALUES
    ('93000000-0000-0000-0000-000000000001', '91000000-0000-0000-0000-000000000001', '92000000-0000-0000-0000-000000000002', '81111111-1111-1111-1111-111111111111'),
    ('93000000-0000-0000-0000-000000000002', '91000000-0000-0000-0000-000000000001', '92000000-0000-0000-0000-000000000002', '82222222-2222-2222-2222-222222222222'),
    ('93000000-0000-0000-0000-000000000003', '91000000-0000-0000-0000-000000000001', '92000000-0000-0000-0000-000000000003', '83333333-3333-3333-3333-333333333333'),
    ('93000000-0000-0000-0000-000000000004', '91000000-0000-0000-0000-000000000002', '92000000-0000-0000-0000-000000000004', '81111111-1111-1111-1111-111111111111'),
    ('93000000-0000-0000-0000-000000000005', '91000000-0000-0000-0000-000000000002', '92000000-0000-0000-0000-000000000005', '82222222-2222-2222-2222-222222222222'),
    ('93000000-0000-0000-0000-000000000006', '91000000-0000-0000-0000-000000000002', '92000000-0000-0000-0000-000000000006', '83333333-3333-3333-3333-333333333333'),
    ('93000000-0000-0000-0000-000000000007', '91000000-0000-0000-0000-000000000003', '92000000-0000-0000-0000-000000000008', '81111111-1111-1111-1111-111111111111'),
    ('93000000-0000-0000-0000-000000000008', '91000000-0000-0000-0000-000000000003', '92000000-0000-0000-0000-000000000007', '82222222-2222-2222-2222-222222222222'),
    ('93000000-0000-0000-0000-000000000009', '91000000-0000-0000-0000-000000000003', '92000000-0000-0000-0000-000000000008', '83333333-3333-3333-3333-333333333333')
ON CONFLICT DO NOTHING;
`
}

// ClientCode returns a map of TypeScript source files containing type definitions and helper functions for querying and managing polls, options, and votes.
func (pollsTemplate) ClientCode() map[string]string {
	return map[string]string{
		"src/lib/polls.ts": `import { ayb } from "./ayb";

export interface Poll {
  id: string;
  title: string;
  description: string;
  creator_id: string;
  multiple_choice: boolean;
  closes_at: string | null;
  created_at: string;
}

export interface PollOption {
  id: string;
  poll_id: string;
  text: string;
  position: number;
  created_at: string;
}

export interface Vote {
  id: string;
  poll_id: string;
  option_id: string;
  user_id: string;
  created_at: string;
}

export interface CreatePollInput {
  title: string;
  description?: string;
  creator_id: string;
  multiple_choice?: boolean;
  closes_at?: string | null;
}

export interface AddOptionInput {
  text: string;
  position?: number;
}

export function listPolls() {
  return ayb.records.list("polls", { sort: "-created_at" });
}

export function createPoll(data: CreatePollInput) {
  return ayb.records.create("polls", {
    description: "",
    multiple_choice: false,
    closes_at: null,
    ...data,
  });
}

export function getPoll(id: string) {
  return ayb.records.get("polls", id);
}

export function listOptions(pollId: string) {
  return ayb.records.list("poll_options", {
    filter: "poll_id='" + pollId + "'",
    sort: "position",
  });
}

export function addOption(pollId: string, data: AddOptionInput) {
  return ayb.records.create("poll_options", {
    poll_id: pollId,
    ...data,
  });
}

export async function castVote(pollId: string, optionId: string) {
  const me = await ayb.auth.me();
  const userId = (me as { id?: string; user?: { id?: string } }).id
    ?? (me as { id?: string; user?: { id?: string } }).user?.id;
  if (!userId) {
    throw new Error("Cannot cast vote without an authenticated user");
  }

  return ayb.records.create("votes", {
    poll_id: pollId,
    option_id: optionId,
    user_id: userId,
  });
}

export function listVotes(pollId: string) {
  return ayb.records.list("votes", {
    filter: "poll_id='" + pollId + "'",
    sort: "created_at",
  });
}
`,
	}
}

// Readme returns a markdown guide describing the polls template schema, constraints, usage examples, and setup instructions.
func (pollsTemplate) Readme() string {
	return `# Polls Template

This scaffold provisions a voting schema and typed helper client code for polls.

## Included schema

- ` + "`polls`" + `: poll definitions, owner, multiple-choice flag, and optional close time
- ` + "`poll_options`" + `: answer options with explicit ordering
- ` + "`votes`" + `: vote records with write-once semantics

## Single-choice constraint

` + "`UNIQUE (poll_id, user_id)`" + ` enforces one vote per user per poll at the database level.
The ` + "`multiple_choice`" + ` column is currently a client UX hint (or future extension), not a database-level override.

## Write-once vote semantics

Votes have INSERT policy only. There are no UPDATE/DELETE vote policies, so users cannot change or retract a vote.

## Poll expiration

` + "`closes_at`" + ` can be set to stop voting windows in client logic and reporting.

## Apply schema and seed data

` + "```bash" + `
ayb sql < schema.sql && ayb sql < seed.sql
` + "```" + `

## SDK usage example

` + "```ts" + `
import { createPoll, addOption, castVote, listVotes } from "./src/lib/polls";

const poll = await createPoll({
  title: "Preferred release day",
  creator_id: "<current-user-id>",
});
await addOption(poll.id, { text: "Tuesday", position: 0 });
await addOption(poll.id, { text: "Thursday", position: 1 });
await castVote(poll.id, "<option-id>");
const results = await listVotes(poll.id);
console.log(results.totalItems);
` + "```" + `

## Quick start

1. Start AYB with ` + "`ayb start`" + `.
2. Apply schema and seed data.
3. Use ` + "`src/lib/polls.ts`" + ` helpers to build poll and voting flows.
`
}
