CREATE TABLE credentials (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL UNIQUE,
    type       TEXT        NOT NULL DEFAULT 'pat',
    data       JSONB       NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE job_templates
    ADD COLUMN IF NOT EXISTS credential_id UUID REFERENCES credentials(id) ON DELETE SET NULL;

ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS credential_id UUID;
