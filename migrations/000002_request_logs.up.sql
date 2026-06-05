-- Migration: 000002_request_logs.up.sql
-- Description: Creates the request_logs partitioned table, initial monthly partitions,
--              and the cost_records table for per-request cost attribution.
-- Note:        request_logs is PARTITION BY RANGE (created_at) — add new monthly
--              partitions via pg_partman in production or manually each month.

CREATE TABLE IF NOT EXISTS request_logs (
    id              UUID          NOT NULL DEFAULT gen_random_uuid(),
    org_id          UUID          NOT NULL,
    trace_id        VARCHAR(64),
    span_id         VARCHAR(32),
    user_id         UUID,
    virtual_acct_id UUID,
    model           VARCHAR(255)   NOT NULL,
    provider        VARCHAR(100)   NOT NULL,
    virtual_model   VARCHAR(255),
    status          VARCHAR(20)    NOT NULL,
    http_status     SMALLINT,
    latency_ms      INTEGER,
    ttft_ms         INTEGER,
    input_tokens    INTEGER        NOT NULL DEFAULT 0,
    output_tokens   INTEGER        NOT NULL DEFAULT 0,
    cost_usd        DECIMAL(12,8)  NOT NULL DEFAULT 0,
    cache_status    VARCHAR(10),
    prompt_fqn      VARCHAR(512),
    skills_used     JSONB          NOT NULL DEFAULT '[]',
    mcps_called     JSONB          NOT NULL DEFAULT '[]',
    guardrails_hit  JSONB          NOT NULL DEFAULT '[]',
    conversation_id VARCHAR(255),
    execution_id    VARCHAR(64),
    metadata        JSONB          NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE IF NOT EXISTS request_logs_2026_06
    PARTITION OF request_logs
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE TABLE IF NOT EXISTS request_logs_2026_07
    PARTITION OF request_logs
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE IF NOT EXISTS cost_records (
    id              UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID           NOT NULL REFERENCES organizations(id),
    request_log_id  UUID           NOT NULL UNIQUE,
    user_id         UUID           REFERENCES users(id),
    model           VARCHAR(255)   NOT NULL,
    input_tokens    INTEGER        NOT NULL DEFAULT 0,
    output_tokens   INTEGER        NOT NULL DEFAULT 0,
    cost_usd        DECIMAL(12,8)  NOT NULL DEFAULT 0,
    period_day      DATE           NOT NULL,
    period_month    VARCHAR(7)     NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_request_logs_org ON request_logs(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cost_records_org_period ON cost_records(org_id, period_day);
