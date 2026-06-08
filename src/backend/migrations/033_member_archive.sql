-- Add archived_at to conversation_members for per-user archive support
ALTER TABLE conversation_members ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
