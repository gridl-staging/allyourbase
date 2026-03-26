-- AI prompt management: versioned prompt templates for reuse in edge functions.

CREATE TABLE IF NOT EXISTS _ayb_ai_prompts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL UNIQUE,
    version      INT NOT NULL DEFAULT 1,
    template     TEXT NOT NULL,
    variables    JSONB,
    model        TEXT,
    provider     TEXT,
    max_tokens   INT,
    temperature  DOUBLE PRECISION,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS _ayb_ai_prompt_versions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prompt_id  UUID NOT NULL REFERENCES _ayb_ai_prompts(id) ON DELETE CASCADE,
    version    INT NOT NULL,
    template   TEXT NOT NULL,
    variables  JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_ai_prompt_versions_prompt_id
    ON _ayb_ai_prompt_versions (prompt_id, version DESC);

-- Trigger: archive old template into versions on every update.
CREATE OR REPLACE FUNCTION _ayb_ai_prompt_version_trigger()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO _ayb_ai_prompt_versions (prompt_id, version, template, variables)
    VALUES (OLD.id, OLD.version, OLD.template, OLD.variables);
    NEW.version := OLD.version + 1;
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS ayb_ai_prompt_version ON _ayb_ai_prompts;
CREATE TRIGGER ayb_ai_prompt_version
    BEFORE UPDATE ON _ayb_ai_prompts
    FOR EACH ROW EXECUTE FUNCTION _ayb_ai_prompt_version_trigger();
