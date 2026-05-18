-- Track whether rows were created by the CRD operator or the REST API.
-- Operator-managed rows (source='crd') are deleted when the CRD instance disappears.
-- API-managed rows (source='api') are never touched by the operator.
ALTER TABLE credentials   ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'api';
ALTER TABLE job_templates ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'api';

-- Global user roles. Default 'admin' preserves the existing single-admin model:
-- existing users and any user created via API start as full admins.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'admin'
        CHECK (role IN ('admin', 'operator', 'viewer'));

-- ── RBAC tables (exclusively operator-managed) ───────────────────────────────

CREATE TABLE IF NOT EXISTS organizations (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       TEXT        UNIQUE NOT NULL,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS teams (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    slug       TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, slug)
);

CREATE TABLE IF NOT EXISTS team_members (
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE IF NOT EXISTS team_permissions (
    id       UUID   PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id  UUID   NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    resource TEXT   NOT NULL CHECK (resource IN ('template', 'credential', 'job')),
    verbs    TEXT[] NOT NULL DEFAULT '{}',
    UNIQUE (team_id, resource)
);
