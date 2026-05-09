-- Add region and size columns to servers so list/provision flows can
-- surface the operator-facing geography and machine class without
-- shoehorning them into labels_json.
-- Version: 0004
-- Dialect: SQLite

ALTER TABLE servers ADD COLUMN region TEXT NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN size TEXT NOT NULL DEFAULT '';
