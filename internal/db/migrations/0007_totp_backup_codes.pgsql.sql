ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_backup_codes_json TEXT DEFAULT '[]';
