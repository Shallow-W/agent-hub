-- daemon 执行任务表：聊天消息投递到指定电脑的指定 CLI
CREATE TABLE IF NOT EXISTS daemon_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    machine_id UUID NOT NULL REFERENCES daemon_machines(id) ON DELETE CASCADE,
    cli_tool VARCHAR(50) NOT NULL,
    prompt TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    result TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    claimed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_daemon_tasks_machine_status
    ON daemon_tasks (machine_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_daemon_tasks_conversation
    ON daemon_tasks (conversation_id, created_at DESC);

---- DOWN
DROP INDEX IF EXISTS idx_daemon_tasks_conversation;
DROP INDEX IF EXISTS idx_daemon_tasks_machine_status;
DROP TABLE IF EXISTS daemon_tasks;
