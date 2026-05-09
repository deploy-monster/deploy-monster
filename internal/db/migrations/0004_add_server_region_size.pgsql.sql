-- Add region and size columns to servers (Postgres variant of 0004).

ALTER TABLE servers ADD COLUMN region TEXT NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN size TEXT NOT NULL DEFAULT '';
