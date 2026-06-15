-- 会话成员表（群聊支持）
CREATE TABLE IF NOT EXISTS conversation_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_membership UNIQUE (conversation_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_conv_members_conv ON conversation_members (conversation_id);
CREATE INDEX IF NOT EXISTS idx_conv_members_user ON conversation_members (user_id);

---- DOWN
DROP TABLE IF EXISTS conversation_members;
