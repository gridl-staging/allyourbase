// Package templates Chat scaffolds a room-based messaging system with SQL schema, seed data, TypeScript client library, and documentation.
package templates

type chatTemplate struct{}

func init() {
	Register(chatTemplate{})
}

func (chatTemplate) Name() string {
	return "chat"
}

// Schema returns the SQL data definition language for the chat domain, including rooms, participants, messages, and read_receipts tables with row-level security policies that enforce participant scoping.
func (chatTemplate) Schema() string {
	return `-- Chat domain schema
-- Apply with: ayb sql < schema.sql

CREATE TABLE IF NOT EXISTS rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'group' CHECK (type IN ('direct', 'group', 'channel')),
    created_by UUID NOT NULL REFERENCES _ayb_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS participants (
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, user_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES _ayb_users(id),
    body TEXT NOT NULL,
    edited_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS read_receipts (
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    last_read_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, user_id)
);

ALTER TABLE rooms ENABLE ROW LEVEL SECURITY;
ALTER TABLE participants ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE read_receipts ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rooms_select ON rooms;
CREATE POLICY rooms_select ON rooms FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = rooms.id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS rooms_insert ON rooms;
CREATE POLICY rooms_insert ON rooms FOR INSERT
    WITH CHECK (created_by = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS rooms_update ON rooms;
CREATE POLICY rooms_update ON rooms FOR UPDATE
    USING (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = rooms.id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
              AND p.role IN ('owner', 'admin')
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = rooms.id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
              AND p.role IN ('owner', 'admin')
        )
    );

DROP POLICY IF EXISTS rooms_delete ON rooms;
CREATE POLICY rooms_delete ON rooms FOR DELETE
    USING (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = rooms.id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
              AND p.role IN ('owner', 'admin')
        )
    );

DROP POLICY IF EXISTS participants_select ON participants;
CREATE POLICY participants_select ON participants FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = participants.room_id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS participants_insert ON participants;
CREATE POLICY participants_insert ON participants FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM rooms r
            WHERE r.id = participants.room_id
              AND r.created_by = current_setting('ayb.user_id', true)::uuid
        )
        OR EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = participants.room_id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
              AND p.role IN ('owner', 'admin')
        )
    );

DROP POLICY IF EXISTS participants_delete ON participants;
CREATE POLICY participants_delete ON participants FOR DELETE
    USING (
        participants.user_id = current_setting('ayb.user_id', true)::uuid
        OR EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = participants.room_id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
              AND p.role IN ('owner', 'admin')
        )
    );

DROP POLICY IF EXISTS messages_select ON messages;
CREATE POLICY messages_select ON messages FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = messages.room_id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS messages_insert ON messages;
CREATE POLICY messages_insert ON messages FOR INSERT
    WITH CHECK (
        sender_id = current_setting('ayb.user_id', true)::uuid
        AND EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = messages.room_id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS messages_update ON messages;
CREATE POLICY messages_update ON messages FOR UPDATE
    USING (sender_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (sender_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS messages_delete ON messages;
CREATE POLICY messages_delete ON messages FOR DELETE
    USING (sender_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS read_receipts_select ON read_receipts;
CREATE POLICY read_receipts_select ON read_receipts FOR SELECT
    USING (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS read_receipts_insert ON read_receipts;
CREATE POLICY read_receipts_insert ON read_receipts FOR INSERT
    WITH CHECK (
        user_id = current_setting('ayb.user_id', true)::uuid
        AND (
            last_read_message_id IS NULL
            OR EXISTS (
                SELECT 1
                FROM messages m
                WHERE m.id = read_receipts.last_read_message_id
                  AND m.room_id = read_receipts.room_id
            )
        )
    );

DROP POLICY IF EXISTS read_receipts_update ON read_receipts;
CREATE POLICY read_receipts_update ON read_receipts FOR UPDATE
    USING (user_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (
        user_id = current_setting('ayb.user_id', true)::uuid
        AND (
            last_read_message_id IS NULL
            OR EXISTS (
                SELECT 1
                FROM messages m
                WHERE m.id = read_receipts.last_read_message_id
                  AND m.room_id = read_receipts.room_id
            )
        )
    );
`
}

// SeedData returns sample SQL INSERT statements including three test users, two chat rooms, room participants with various roles, and example messages.
func (chatTemplate) SeedData() string {
	return `-- Chat domain seed data
-- Apply with: ayb sql < seed.sql

INSERT INTO _ayb_users (id, email, password_hash)
VALUES
    ('a1111111-1111-1111-1111-111111111111', 'chat.alex@example.com', 'seeded-password-hash'),
    ('a2222222-2222-2222-2222-222222222222', 'chat.sam@example.com', 'seeded-password-hash'),
    ('a3333333-3333-3333-3333-333333333333', 'chat.jordan@example.com', 'seeded-password-hash')
ON CONFLICT DO NOTHING;

INSERT INTO rooms (id, name, type, created_by)
VALUES
    ('b1000000-0000-0000-0000-000000000001', 'Platform Team', 'group', 'a1111111-1111-1111-1111-111111111111'),
    ('b1000000-0000-0000-0000-000000000002', 'Alex & Sam', 'direct', 'a1111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

INSERT INTO participants (room_id, user_id, role)
VALUES
    ('b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'owner'),
    ('b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'admin'),
    ('b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'member'),
    ('b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'member'),
    ('b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'member')
ON CONFLICT DO NOTHING;

INSERT INTO messages (id, room_id, sender_id, body)
VALUES
    ('c1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'Morning team, standup starts in 10 minutes.'),
    ('c1000000-0000-0000-0000-000000000002', 'b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'On it, posting blocker updates now.'),
    ('c1000000-0000-0000-0000-000000000003', 'b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'I can take the webhook retry task.'),
    ('c1000000-0000-0000-0000-000000000004', 'b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'Great, can you also own the test coverage follow-up?'),
    ('c1000000-0000-0000-0000-000000000005', 'b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'Yes, I will open a PR by this afternoon.'),
    ('c1000000-0000-0000-0000-000000000006', 'b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'I pushed the migration draft, please review.'),
    ('c1000000-0000-0000-0000-000000000007', 'b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'Reviewing now; policy naming looks consistent.'),
    ('c1000000-0000-0000-0000-000000000008', 'b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'Should we split chat and polls into separate PRs?'),
    ('c1000000-0000-0000-0000-000000000009', 'b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'Yes, smaller reviews will move faster.'),
    ('c1000000-0000-0000-0000-000000000010', 'b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'Agreed. Let us ship polls first.'),
    ('c1000000-0000-0000-0000-000000000011', 'b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'Polls PR opened: please review when free.'),
    ('c1000000-0000-0000-0000-000000000012', 'b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'Reviewed and left two comments on seed data.'),
    ('c1000000-0000-0000-0000-000000000013', 'b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'Can we sync on the release checklist after lunch?'),
    ('c1000000-0000-0000-0000-000000000014', 'b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'Yes, 1:30 PM works for me.'),
    ('c1000000-0000-0000-0000-000000000015', 'b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'Perfect, I will bring rollout metrics.'),
    ('c1000000-0000-0000-0000-000000000016', 'b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'Also need to confirm support runbook updates.'),
    ('c1000000-0000-0000-0000-000000000017', 'b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'Good call, adding that to agenda.'),
    ('c1000000-0000-0000-0000-000000000018', 'b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'Do you want me to draft the announcement copy?'),
    ('c1000000-0000-0000-0000-000000000019', 'b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'Yes please, that would help a lot.'),
    ('c1000000-0000-0000-0000-000000000020', 'b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'Done. Shared in the docs channel.')
ON CONFLICT DO NOTHING;

INSERT INTO read_receipts (room_id, user_id, last_read_message_id)
VALUES
    ('b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'c1000000-0000-0000-0000-000000000012'),
    ('b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'c1000000-0000-0000-0000-000000000011'),
    ('b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'c1000000-0000-0000-0000-000000000010'),
    ('b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'c1000000-0000-0000-0000-000000000020'),
    ('b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'c1000000-0000-0000-0000-000000000019')
ON CONFLICT DO NOTHING;
`
}

// ClientCode returns a map of TypeScript client files providing type-safe interfaces and helper functions for room and message operations, participant management, and read receipt tracking.
func (chatTemplate) ClientCode() map[string]string {
	return map[string]string{
		"src/lib/chat.ts": `import { ayb } from "./ayb";

export interface Room {
  id: string;
  name: string;
  type: "direct" | "group" | "channel";
  created_by: string;
  created_at: string;
}

export interface Participant {
  room_id: string;
  user_id: string;
  role: "owner" | "admin" | "member";
  joined_at: string;
}

export interface Message {
  id: string;
  room_id: string;
  sender_id: string;
  body: string;
  edited_at: string | null;
  created_at: string;
}

export interface ReadReceipt {
  room_id: string;
  user_id: string;
  last_read_message_id: string | null;
  updated_at: string;
}

export interface CreateRoomInput {
  name: string;
  type?: "direct" | "group" | "channel";
  created_by: string;
}

async function requireCurrentUserID(): Promise<string> {
  const me = await ayb.auth.me();
  const userId = (me as { id?: string; user?: { id?: string } }).id
    ?? (me as { id?: string; user?: { id?: string } }).user?.id;
  if (!userId) {
    throw new Error("Cannot continue without an authenticated user");
  }
  return userId;
}

export function listRooms() {
  return ayb.records.list("rooms", { sort: "-created_at" });
}

export function createRoom(data: CreateRoomInput) {
  return ayb.records.create("rooms", {
    type: "group",
    ...data,
  });
}

export function getRoom(id: string) {
  return ayb.records.get("rooms", id);
}

export function listParticipants(roomId: string) {
  return ayb.records.list("participants", {
    filter: "room_id='" + roomId + "'",
    sort: "joined_at",
  });
}

export function addParticipant(roomId: string, userId: string, role: "owner" | "admin" | "member" = "member") {
  return ayb.records.create("participants", {
    room_id: roomId,
    user_id: userId,
    role,
  });
}

export async function removeParticipant(roomId: string, userId: string) {
  const res = await ayb.records.list<Participant>("participants", {
    filter: "room_id='" + roomId + "' && user_id='" + userId + "'",
    limit: 1,
  });
  if (!res.items?.length) {
    return;
  }
  return ayb.records.delete("participants", roomId + "," + userId);
}

export function listMessages(roomId: string) {
  return ayb.records.list("messages", {
    filter: "room_id='" + roomId + "'",
    sort: "created_at",
  });
}

export async function sendMessage(roomId: string, body: string) {
  const userId = await requireCurrentUserID();

  return ayb.records.create("messages", {
    room_id: roomId,
    sender_id: userId,
    body,
  });
}

export function editMessage(id: string, body: string) {
  return ayb.records.update("messages", id, {
    body,
    edited_at: new Date().toISOString(),
  });
}

export async function markRead(roomId: string, messageId: string) {
  const userId = await requireCurrentUserID();

  const existing = await ayb.records.list<ReadReceipt>("read_receipts", {
    filter: "room_id='" + roomId + "' && user_id='" + userId + "'",
    limit: 1,
  });
  if (existing.items?.length) {
    return ayb.records.update("read_receipts", roomId + "," + userId, {
      last_read_message_id: messageId,
      updated_at: new Date().toISOString(),
    });
  }

  return ayb.records.create("read_receipts", {
    room_id: roomId,
    user_id: userId,
    last_read_message_id: messageId,
  });
}
`,
	}
}

// Readme returns markdown documentation describing the template schema structure, access control model, realtime subscription patterns, and usage examples.
func (chatTemplate) Readme() string {
	return `# Chat Template

This scaffold provisions a room-based chat schema with participant-scoped access control.

## Included schema

- ` + "`rooms`" + `: chat rooms with ` + "`direct`" + `, ` + "`group`" + `, or ` + "`channel`" + ` types
- ` + "`participants`" + `: room membership and role (` + "`owner`" + `, ` + "`admin`" + `, ` + "`member`" + `)
- ` + "`messages`" + `: room messages with sender ownership and optional edit timestamp
- ` + "`read_receipts`" + `: per-user read position per room

## RLS model

Room, participant, and message visibility is scoped to room participants. Room management and participant management are restricted to owner/admin roles (with creator bootstrap support). Messages are editable/deletable by sender only.

## Realtime hint

Use AYB realtime subscriptions on the ` + "`messages`" + ` table to drive live updates:

` + "```ts" + `
const unsubscribe = ayb.realtime.subscribe(["messages"], (event) => {
  const roomId = (event.record as { room_id?: string }).room_id;
  if (roomId === "<room-id>") {
    // update local chat timeline
  }
});
` + "```" + `

## Apply schema and seed data

` + "```bash" + `
ayb sql < schema.sql && ayb sql < seed.sql
` + "```" + `

## SDK usage example

` + "```ts" + `
import {
  createRoom,
  addParticipant,
  sendMessage,
  markRead,
} from "./src/lib/chat";

const room = await createRoom({
  name: "Incident War Room",
  type: "group",
  created_by: "<current-user-id>",
});
await addParticipant(room.id, "<teammate-user-id>", "member");
const message = await sendMessage(room.id, "Starting timeline doc now.");
await markRead(room.id, message.id);
` + "```" + `

## Quick start

1. Start AYB with ` + "`ayb start`" + `.
2. Apply schema and seed data.
3. Use ` + "`src/lib/chat.ts`" + ` helpers to build room, messaging, and read-state flows.
`
}
