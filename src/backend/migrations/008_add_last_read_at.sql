ALTER TABLE conversation_members ADD COLUMN IF NOT EXISTS last_read_at TIMESTAMPTZ;

---- DOWN
ALTER TABLE conversation_members DROP COLUMN IF EXISTS last_read_at;
