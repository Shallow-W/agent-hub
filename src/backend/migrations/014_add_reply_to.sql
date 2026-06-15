-- 消息回复引用
ALTER TABLE messages ADD COLUMN IF NOT EXISTS reply_to UUID REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_messages_reply_to ON messages (reply_to) WHERE reply_to IS NOT NULL;

---- DOWN
ALTER TABLE messages DROP COLUMN IF EXISTS reply_to;
