-- Per-user message hides: 用户"删除"消息只是对自己隐藏，其他用户仍可见。
-- 设计为独立表而非 messages 列，支持多用户各自隐藏同一消息。
-- 未来可扩展为 per-user 消息状态（稍后处理/归档/标记重要等）。

CREATE TABLE IF NOT EXISTS message_hides (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    hidden_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_message_hides_user ON message_hides(user_id);
CREATE INDEX IF NOT EXISTS idx_message_hides_message ON message_hides(message_id);
