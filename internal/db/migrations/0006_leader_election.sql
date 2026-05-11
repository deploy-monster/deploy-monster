-- Leader election table for HA multi-instance deployments.
-- Only used when running multiple DeployMonster instances with PostgreSQL.
-- SQLite deployments are always single-instance so this table is not needed.

-- This migration is intentionally empty for SQLite. Leader election
-- requires PostgreSQL advisory locks which are not available in SQLite.
-- The PostgresLeaderElector is only instantiated when using PostgresDB.