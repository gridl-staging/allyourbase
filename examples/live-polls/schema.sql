-- Live Polls schema for AYB
-- Run: psql $DATABASE_URL -f schema.sql

CREATE TABLE IF NOT EXISTS polls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    question TEXT NOT NULL CHECK (length(question) > 0),
    is_closed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS poll_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    label TEXT NOT NULL CHECK (length(label) > 0),
    position SMALLINT NOT NULL DEFAULT 0 CHECK (position >= 0)
);

CREATE TABLE IF NOT EXISTS votes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id UUID NOT NULL REFERENCES poll_options(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (poll_id, user_id)
);

-- Indexes for common queries.
CREATE INDEX IF NOT EXISTS idx_poll_options_poll_id ON poll_options(poll_id);
CREATE INDEX IF NOT EXISTS idx_votes_poll_id ON votes(poll_id);
CREATE INDEX IF NOT EXISTS idx_votes_option_id ON votes(option_id);

-- Drop all policies for idempotency (re-running schema on existing DB).
DROP POLICY IF EXISTS polls_read ON polls;
DROP POLICY IF EXISTS polls_insert ON polls;
DROP POLICY IF EXISTS polls_update ON polls;
DROP POLICY IF EXISTS options_read ON poll_options;
DROP POLICY IF EXISTS options_insert ON poll_options;
DROP POLICY IF EXISTS votes_read ON votes;
DROP POLICY IF EXISTS votes_insert ON votes;
DROP POLICY IF EXISTS votes_update ON votes;

-- RLS: polls
ALTER TABLE polls ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS polls_read ON polls;
CREATE POLICY polls_read ON polls FOR SELECT USING (true);
DROP POLICY IF EXISTS polls_insert ON polls;
CREATE POLICY polls_insert ON polls FOR INSERT WITH CHECK (
    user_id::text = current_setting('ayb.user_id', true)
);
DROP POLICY IF EXISTS polls_update ON polls;
CREATE POLICY polls_update ON polls FOR UPDATE USING (
    user_id::text = current_setting('ayb.user_id', true)
);

-- RLS: poll_options
ALTER TABLE poll_options ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS options_read ON poll_options;
CREATE POLICY options_read ON poll_options FOR SELECT USING (true);
DROP POLICY IF EXISTS options_insert ON poll_options;
CREATE POLICY options_insert ON poll_options FOR INSERT WITH CHECK (
    poll_id IN (
        SELECT id FROM polls
        WHERE user_id::text = current_setting('ayb.user_id', true)
    )
);

-- RLS: votes
ALTER TABLE votes ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS votes_read ON votes;
CREATE POLICY votes_read ON votes FOR SELECT USING (true);
DROP POLICY IF EXISTS votes_insert ON votes;
CREATE POLICY votes_insert ON votes FOR INSERT WITH CHECK (
    user_id::text = current_setting('ayb.user_id', true)
    AND NOT (SELECT is_closed FROM polls WHERE id = poll_id)
);
DROP POLICY IF EXISTS votes_update ON votes;
CREATE POLICY votes_update ON votes FOR UPDATE USING (
    user_id::text = current_setting('ayb.user_id', true)
) WITH CHECK (
    NOT (SELECT is_closed FROM polls WHERE id = poll_id)
);

-- RPC: atomic vote cast (enforces one vote per user per poll, rejects closed polls).
-- Uses ayb.user_id (set by the AYB server from the authenticated JWT claim).
-- Captures the inserted/updated row ID into a local variable to avoid SQLSTATE
-- 42702 (ambiguous column) between RETURNS TABLE output parameter names and the
-- INSERT target table column names in a RETURNING clause.
DROP FUNCTION IF EXISTS cast_vote(UUID, UUID);
CREATE OR REPLACE FUNCTION cast_vote(p_poll_id UUID, p_option_id UUID)
RETURNS SETOF votes AS $$
DECLARE
    v_user_id UUID := current_setting('ayb.user_id', true)::uuid;
    v_vote_id UUID;
BEGIN
    -- Reject if poll is closed.
    IF (SELECT is_closed FROM polls WHERE polls.id = p_poll_id) THEN
        RAISE EXCEPTION 'poll is closed';
    END IF;

    INSERT INTO votes (poll_id, option_id, user_id)
    VALUES (p_poll_id, p_option_id, v_user_id)
    ON CONFLICT ON CONSTRAINT votes_poll_id_user_id_key DO UPDATE SET
        option_id = EXCLUDED.option_id,
        created_at = now()
    RETURNING votes.id INTO v_vote_id;

    RETURN QUERY
    SELECT v.id, v.poll_id, v.option_id, v.user_id, v.created_at
    FROM votes v
    WHERE v.id = v_vote_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
