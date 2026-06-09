-- DB-06: conversations.type CHECK constraint
-- Normalize legacy types and ensure all existing rows conform
UPDATE conversations SET type = 'single' WHERE type = 'private';
UPDATE conversations SET type = 'single' WHERE type NOT IN ('single', 'group');
ALTER TABLE conversations DROP CONSTRAINT IF EXISTS chk_conversations_type;
ALTER TABLE conversations ADD CONSTRAINT chk_conversations_type
  CHECK (type IN ('single', 'group'));

-- DB-07: friends.status CHECK constraint
UPDATE friends SET status = 'pending' WHERE status NOT IN ('pending', 'accepted', 'rejected');
ALTER TABLE friends DROP CONSTRAINT IF EXISTS chk_friends_status;
ALTER TABLE friends ADD CONSTRAINT chk_friends_status
  CHECK (status IN ('pending', 'accepted', 'rejected'));

-- DB-11: conversation_members unique index for idempotency
CREATE UNIQUE INDEX IF NOT EXISTS idx_conv_members_unique
  ON conversation_members (conversation_id, user_id);

---- DOWN
ALTER TABLE conversations DROP CONSTRAINT IF EXISTS chk_conversations_type;
ALTER TABLE friends DROP CONSTRAINT IF EXISTS chk_friends_status;
DROP INDEX IF EXISTS idx_conv_members_unique;
