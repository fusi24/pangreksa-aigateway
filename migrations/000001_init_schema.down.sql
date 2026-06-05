-- Migration: 000001_init_schema.down.sql
-- Description: Drops all tables created by 000001_init_schema.up.sql in reverse
--              dependency order (child tables before parent tables).

DROP TABLE IF EXISTS auth_events;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS agent_sessions;
DROP TABLE IF EXISTS rate_limit_rules;
DROP TABLE IF EXISTS budget_rules;
DROP TABLE IF EXISTS guardrail_policies;
DROP TABLE IF EXISTS mcp_servers;
DROP TABLE IF EXISTS skills;
DROP TABLE IF EXISTS prompt_versions;
DROP TABLE IF EXISTS prompt_repositories;
DROP TABLE IF EXISTS virtual_models;
DROP TABLE IF EXISTS provider_accounts;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS virtual_accounts;
DROP TABLE IF EXISTS user_entitlements;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;
