CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT        UNIQUE NOT NULL,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE job_templates (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    -- null = use controller's DEFAULT_RUNNER_IMAGE (like AWX EE)
    image       TEXT,
    playbook    TEXT        NOT NULL DEFAULT '/playbooks/sample.yml',
    extra_vars  JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE jobs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id   UUID        REFERENCES job_templates(id) ON DELETE SET NULL,
    status        TEXT        NOT NULL DEFAULT 'pending',  -- pending|running|success|failed
    image_used    TEXT        NOT NULL,
    playbook      TEXT        NOT NULL,
    extra_vars    JSONB       NOT NULL DEFAULT '{}',
    k8s_job_name  TEXT,
    started_at    TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE job_logs (
    id         BIGSERIAL   PRIMARY KEY,
    job_id     UUID        NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    line_num   INTEGER     NOT NULL,
    content    TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_job_logs_job ON job_logs(job_id, line_num);
