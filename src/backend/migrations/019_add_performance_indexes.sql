-- DB-03: last_read_at 索引（GetMember 热查询）
CREATE INDEX IF NOT EXISTS idx_conv_members_last_read_at
    ON conversation_members (conversation_id, user_id, last_read_at);

-- DB-05: archived_at 索引（ListByUserID 过滤归档）
CREATE INDEX IF NOT EXISTS idx_conversations_user_archived
    ON conversations (user_id, archived_at) WHERE archived_at IS NOT NULL;

---- DOWN
DROP INDEX IF EXISTS idx_conv_members_last_read_at;
DROP INDEX IF EXISTS idx_conversations_user_archived;
