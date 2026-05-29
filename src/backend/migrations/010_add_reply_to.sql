-- 消息回复引用
ALTER TABLE messages ADD COLUMN reply_to UUID REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX idx_messages_reply_to ON messages (reply_to) WHERE reply_to IS NOT NULL;

---- DOWN
ALTER TABLE messages DROP COLUMN IF EXISTS reply_to;
