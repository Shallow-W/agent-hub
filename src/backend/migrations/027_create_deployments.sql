-- 部署表：将一个 artifact（按血缘根）落盘为可访问站点 / 可打包下载的产物
CREATE TABLE IF NOT EXISTS deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_root_id UUID NOT NULL,
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    mode VARCHAR(20) NOT NULL DEFAULT 'preview',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    url VARCHAR(2000),
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- mode 取值: 'preview'(可访问站点 + 可下载)
-- status 取值: 'pending' | 'success' | 'failed'

CREATE INDEX IF NOT EXISTS idx_deployments_root ON deployments(artifact_root_id);
CREATE INDEX IF NOT EXISTS idx_deployments_conversation ON deployments(conversation_id);

---- DOWN
DROP TABLE IF EXISTS deployments;
