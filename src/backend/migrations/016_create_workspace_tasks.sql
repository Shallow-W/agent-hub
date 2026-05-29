-- 任务看板表：先支持独立任务，后续可通过 conversation_id 绑定到群聊。
CREATE TABLE IF NOT EXISTS workspace_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id UUID REFERENCES conversations(id) ON DELETE SET NULL,
    assignee_id UUID REFERENCES users(id) ON DELETE SET NULL,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    title VARCHAR(120) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'todo',
    priority VARCHAR(16) NOT NULL DEFAULT 'medium',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_workspace_tasks_status CHECK (status IN ('todo', 'in_progress', 'blocked', 'done')),
    CONSTRAINT chk_workspace_tasks_priority CHECK (priority IN ('low', 'medium', 'high'))
);

CREATE INDEX IF NOT EXISTS idx_workspace_tasks_user_status
    ON workspace_tasks (user_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_conversation
    ON workspace_tasks (conversation_id, updated_at DESC);

---- DOWN
DROP INDEX IF EXISTS idx_workspace_tasks_conversation;
DROP INDEX IF EXISTS idx_workspace_tasks_user_status;
DROP TABLE IF EXISTS workspace_tasks;
