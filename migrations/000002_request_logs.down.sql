-- Migration: 000002_request_logs.down.sql
-- Description: Drops cost_records and the request_logs partitioned table (and all
--              child partitions) created by 000002_request_logs.up.sql.

DROP TABLE IF EXISTS cost_records;
DROP TABLE IF EXISTS request_logs_2026_07;
DROP TABLE IF EXISTS request_logs_2026_06;
DROP TABLE IF EXISTS request_logs;
