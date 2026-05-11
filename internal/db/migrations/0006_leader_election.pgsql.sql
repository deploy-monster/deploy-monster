-- Leader election table for HA multi-instance deployments using PostgreSQL
-- advisory locks. Only one instance can hold leadership for a given key at a time.

CREATE TABLE IF NOT EXISTS _leader_election (
    key          TEXT        NOT NULL PRIMARY KEY,
    instance_id  TEXT        NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL
);

-- Index for quick leader lookup by instance ID (used in resign/isleader checks)
CREATE INDEX IF NOT EXISTS idx_leader_election_instance ON _leader_election (instance_id);

-- Index for cleaning up expired leadership rows
CREATE INDEX IF NOT EXISTS idx_leader_election_expires ON _leader_election (expires_at);