-- 会话上下文黑板：用户手写长期上下文，适用于群聊、单聊和 Agent 单聊
CREATE TABLE IF NOT EXISTS conversation_blackboards (
    conversation_id UUID PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
    manual_context TEXT NOT NULL DEFAULT '',
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

---- DOWN
DROP TABLE IF EXISTS conversation_blackboards;
