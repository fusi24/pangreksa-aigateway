-- Migration: 000001_init_schema.up.sql
-- Description: Creates the full Pangreksa AI Gateway Engine initial schema.
--              Includes all tables defined in SRS Section 10.4.
-- Author:      AI Gateway Engine — infrastructure scaffold
-- Note:        audit_log is INSERT-only; UPDATE and DELETE are revoked from PUBLIC.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ─────────────────────────────────────────────────────────────────────────────
-- ORGANIZATIONS
-- Top-level tenant. All data is scoped per org.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS organizations (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL,
    slug       VARCHAR(100) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ─────────────────────────────────────────────────────────────────────────────
-- USERS
-- Human users: local auth, Entra ID, or Active Directory.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id                  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email               VARCHAR(320) NOT NULL,
    password_hash       VARCHAR(255),
    provider            VARCHAR(50)  NOT NULL DEFAULT 'local',
    status              VARCHAR(20)  NOT NULL DEFAULT 'active',
    mfa_enabled         BOOLEAN      NOT NULL DEFAULT FALSE,
    failed_login_count  INT          NOT NULL DEFAULT 0,
    locked_until        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, email)
);

CREATE INDEX IF NOT EXISTS idx_users_org_email ON users(org_id, email);

-- ─────────────────────────────────────────────────────────────────────────────
-- USER_ENTITLEMENTS
-- Per-user resolved access rights cached at L1 (sync.Map), L2 (Redis), L3 (DB).
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS user_entitlements (
    id               UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID          NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    org_id           UUID          NOT NULL REFERENCES organizations(id),
    allowed_prompts  JSONB         NOT NULL DEFAULT '[]',
    allowed_skills   JSONB         NOT NULL DEFAULT '[]',
    allowed_mcps     JSONB         NOT NULL DEFAULT '[]',
    permissions      JSONB         NOT NULL DEFAULT '[]',
    budget_limit_usd DECIMAL(12,4) NOT NULL DEFAULT 0,
    rate_limit_rpm   INT           NOT NULL DEFAULT 60,
    rate_limit_tpm   INT           NOT NULL DEFAULT 100000,
    data_scope       VARCHAR(20)   NOT NULL DEFAULT 'own_data',
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

-- ─────────────────────────────────────────────────────────────────────────────
-- VIRTUAL_ACCOUNTS
-- Service identities that hold VAT (Virtual Account Token) bearer keys.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS virtual_accounts (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    token_hash  VARCHAR(255) NOT NULL UNIQUE,
    auto_rotate BOOLEAN      NOT NULL DEFAULT FALSE,
    status      VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ─────────────────────────────────────────────────────────────────────────────
-- API_KEYS
-- PAT (Personal Access Token) and VAT hashes. At most one of user_id / va_id is set.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS api_keys (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID         REFERENCES users(id) ON DELETE CASCADE,
    va_id      UUID         REFERENCES virtual_accounts(id) ON DELETE CASCADE,
    org_id     UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    name       VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT api_keys_owner_check CHECK (
        (user_id IS NOT NULL)::INT + (va_id IS NOT NULL)::INT = 1
    )
);

CREATE INDEX IF NOT EXISTS idx_api_keys_token_hash ON api_keys(token_hash);

-- ─────────────────────────────────────────────────────────────────────────────
-- ROLES
-- Named bundles of permissions; can be system-defined or org-custom.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS roles (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name           VARCHAR(100) NOT NULL,
    is_system_role BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, name)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- PERMISSIONS
-- Fine-grained permission tokens (e.g. gateway.mcp.read).
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS permissions (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    token       VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ─────────────────────────────────────────────────────────────────────────────
-- USER_ROLES  (many-to-many: users <-> roles)
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS user_roles (
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id     UUID        NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    assigned_by UUID        REFERENCES users(id),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- ROLE_PERMISSIONS  (many-to-many: roles <-> permissions)
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- PROVIDER_ACCOUNTS
-- LLM provider credentials. api_key_enc is AES-256-GCM encrypted.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS provider_accounts (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider_name VARCHAR(100) NOT NULL,
    api_key_enc   TEXT         NOT NULL,
    base_url      VARCHAR(512),
    region        VARCHAR(100),
    status        VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, provider_name)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- VIRTUAL_MODELS
-- Logical LLM abstraction that routes to one or more real provider models.
-- routing_config is a JSONB object with provider weights and fallback rules.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS virtual_models (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name           VARCHAR(255) NOT NULL,
    routing_config JSONB        NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, name)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- PROMPT_REPOSITORIES
-- Namespaces for versioned prompt templates.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS prompt_repositories (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name       VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, name)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- PROMPT_VERSIONS
-- Immutable versioned prompt content. Content must never be updated after creation.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS prompt_versions (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    repo_id     UUID         NOT NULL REFERENCES prompt_repositories(id) ON DELETE CASCADE,
    prompt_name VARCHAR(255) NOT NULL,
    version     INTEGER      NOT NULL,
    content     TEXT         NOT NULL,
    config      JSONB        NOT NULL DEFAULT '{}',
    created_by  UUID         REFERENCES users(id),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (repo_id, prompt_name, version)
);

CREATE INDEX IF NOT EXISTS idx_prompt_versions_repo ON prompt_versions(repo_id, prompt_name);

-- ─────────────────────────────────────────────────────────────────────────────
-- SKILLS
-- SKILL.md catalog entries. description is <= 200 chars, shown upfront to LLM.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS skills (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    description VARCHAR(200) NOT NULL,
    version     INTEGER      NOT NULL DEFAULT 1,
    content     TEXT         NOT NULL,
    frontmatter JSONB        NOT NULL DEFAULT '{}',
    preload     BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, name, version)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- MCP_SERVERS
-- Registered MCP servers with tool definitions and whitelist.
-- credentials_enc is AES-256-GCM encrypted when present.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS mcp_servers (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    url             VARCHAR(512) NOT NULL,
    auth_type       VARCHAR(50)  NOT NULL DEFAULT 'none',
    credentials_enc TEXT,
    tools           JSONB        NOT NULL DEFAULT '[]',
    status          VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, name)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- GUARDRAIL_POLICIES
-- Content inspection, PII, injection, and code safety rules per org.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS guardrail_policies (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    type        VARCHAR(50)  NOT NULL,
    hook        VARCHAR(50)  NOT NULL,
    mode        VARCHAR(20)  NOT NULL,
    enforcement VARCHAR(20)  NOT NULL DEFAULT 'enforce',
    config      JSONB        NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ─────────────────────────────────────────────────────────────────────────────
-- BUDGET_RULES
-- Ordered cost budget enforcement rules. priority ASC = first evaluated.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS budget_rules (
    id        UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id    UUID    NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    priority  INTEGER NOT NULL DEFAULT 100,
    rule_yaml TEXT    NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_budget_rules_org_priority ON budget_rules(org_id, priority ASC);

-- ─────────────────────────────────────────────────────────────────────────────
-- RATE_LIMIT_RULES
-- Ordered sliding-window rate limit rules. priority ASC = first evaluated.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS rate_limit_rules (
    id         UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID    NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    priority   INTEGER NOT NULL DEFAULT 100,
    rule_yaml  TEXT    NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_rate_limit_rules_org_priority ON rate_limit_rules(org_id, priority ASC);

-- ─────────────────────────────────────────────────────────────────────────────
-- AGENT_SESSIONS
-- Short-lived agent execution state (TTL 24h). Stored in Redis for hot path;
-- persisted here for audit and recovery.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS agent_sessions (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID         NOT NULL REFERENCES organizations(id),
    user_id        UUID         REFERENCES users(id),
    execution_id   VARCHAR(64)  NOT NULL UNIQUE,
    response_id    VARCHAR(255),
    state          JSONB        NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    ttl_expires_at TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_sessions_execution ON agent_sessions(execution_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_ttl ON agent_sessions(ttl_expires_at);

-- ─────────────────────────────────────────────────────────────────────────────
-- AUDIT_LOG
-- Immutable mutation log. INSERT-only: UPDATE and DELETE are revoked from PUBLIC.
-- Retention: 7 years (archive to cold storage; never hard-delete).
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS audit_log (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID         NOT NULL,
    user_id       UUID,
    ip_address    INET,
    user_agent    TEXT,
    event_type    VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100),
    resource_id   UUID,
    old_value     JSONB,
    new_value     JSONB,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

REVOKE UPDATE, DELETE ON audit_log FROM PUBLIC;
CREATE INDEX IF NOT EXISTS idx_audit_log_org ON audit_log(org_id, created_at DESC);

-- ─────────────────────────────────────────────────────────────────────────────
-- AUTH_EVENTS
-- Authentication event log: logins, failures, MFA, lockouts.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS auth_events (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID         NOT NULL REFERENCES organizations(id),
    user_id        UUID         REFERENCES users(id),
    provider       VARCHAR(50)  NOT NULL DEFAULT 'local',
    event_type     VARCHAR(50)  NOT NULL,
    success        BOOLEAN      NOT NULL DEFAULT FALSE,
    failure_reason TEXT,
    ip_address     INET,
    user_agent     TEXT,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auth_events_user ON auth_events(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_auth_events_org ON auth_events(org_id, created_at DESC);
