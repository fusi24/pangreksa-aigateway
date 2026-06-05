-- Migration: 000003_console.down.sql
-- Description: Drops the console_sessions and daemon_registrations tables
--              in reverse dependency order.

DROP TABLE IF EXISTS console_sessions;
DROP TABLE IF EXISTS daemon_registrations;
