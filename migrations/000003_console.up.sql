-- Migration: 000003_console.up.sql
-- Description: Creates the daemon_registrations and console_sessions tables
--              required by the Pangreksa AI Gateway Console backend.

-- ─────────────────────────────────────────────────────────────────────────────
-- DAEMON_REGISTRATIONS
-- Heartbeat registry for connected gateway daemons.
-- Each daemon upserts its row on every heartbeat call.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS daemon_registrations (
    gateway_id      VARCHAR(255)  PRIMARY KEY,
    org_id          UUID          NOT NULL REFERENCES organizations(id),
    version         VARCHAR(50),
    config_version  VARCHAR(64),
    uptime_seconds  BIGINT        NOT NULL DEFAULT 0,
    provider_ports  JSONB         NOT NULL DEFAULT '{}',
    otel_endpoint   VARCHAR(512),
    last_seen_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    registered_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_daemon_reg_org ON daemon_registrations(org_id, last_seen_at DESC);

-- ─────────────────────────────────────────────────────────────────────────────
-- CONSOLE_SESSIONS
-- Refresh token sessions for the web console.
-- Each login mints one row; logout or expiry removes it.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS console_sessions (
    id                  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash  VARCHAR(255) NOT NULL UNIQUE,
    ip_address          INET,
    user_agent          TEXT,
    expires_at          TIMESTAMPTZ  NOT NULL,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_console_sessions_user    ON console_sessions(user_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_console_sessions_expires ON console_sessions(expires_at);
