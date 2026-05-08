-- Down migration for 0004: drop the region and size columns.
-- SQLite added native ALTER TABLE DROP COLUMN in 3.35; we target 3.40+.

ALTER TABLE servers DROP COLUMN region;
ALTER TABLE servers DROP COLUMN size;
