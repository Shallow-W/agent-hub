-- 产物表：Agent 回复中提取的结构化产物（代码、网页等），关联消息并预留版本字段
CREATE TABLE IF NOT EXISTS artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    version INTEGER NOT NULL DEFAULT 1,
    type VARCHAR(20) NOT NULL,
    language VARCHAR(50),
    filename VARCHAR(500),
    title VARCHAR(500),
    url VARCHAR(2000),
    content TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- type 取值: 'code'(代码块), 'webpage'(网页 URL 或 HTML 文档)

CREATE INDEX IF NOT EXISTS idx_artifacts_message ON artifacts(message_id);

---- DOWN
DROP TABLE IF EXISTS artifacts;
