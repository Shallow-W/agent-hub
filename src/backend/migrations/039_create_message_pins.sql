-- 群聊上下文黑板：用户手动 pin 消息作为长期共享上下文
CREATE TABLE IF NOT EXISTS message_pins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (conversation_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_message_pins_conversation
    ON message_pins (conversation_id, enabled, created_at DESC);

---- DOWN
DROP TABLE IF EXISTS message_pins;
