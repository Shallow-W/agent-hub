-- 消息软删除（撤回）
ALTER TABLE messages ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE INDEX idx_messages_deleted_at ON messages (deleted_at) WHERE deleted_at IS NOT NULL;

---- DOWN
ALTER TABLE messages DROP COLUMN IF EXISTS deleted_at;
