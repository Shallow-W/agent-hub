ALTER TABLE conversations ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

---- DOWN
ALTER TABLE conversations DROP COLUMN IF EXISTS archived_at;
