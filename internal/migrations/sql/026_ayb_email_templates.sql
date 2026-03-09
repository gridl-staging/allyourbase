-- 026: Custom email templates table
-- Stores user-defined overrides for transactional email templates.
-- System defaults are embedded in the Go binary; this table holds only custom overrides.

CREATE TABLE _ayb_email_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_key    VARCHAR(255) NOT NULL,
    subject_template TEXT NOT NULL,
    html_template   TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Each key can have at most one custom override.
    CONSTRAINT uq_ayb_email_templates_key UNIQUE (template_key),

    -- Key format: dot-separated lowercase segments (e.g. auth.password_reset, app.club_invite).
    CONSTRAINT chk_ayb_email_templates_key_format
        CHECK (template_key ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9_]*)+$'),

    -- Size limits to prevent abuse.
    CONSTRAINT chk_ayb_email_templates_html_size
        CHECK (length(html_template) <= 256000),
    CONSTRAINT chk_ayb_email_templates_subject_size
        CHECK (length(subject_template) <= 1000)
);
