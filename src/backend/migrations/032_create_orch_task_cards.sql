-- 032: Orch 编排任务独立卡片表
-- 将 Orch 分派的 worker 任务从通用 workspace_tasks 分离到专用表，
-- 创建时写入完整的发送方/处理方信息，不再依赖 JOIN。

CREATE TABLE IF NOT EXISTS orch_task_cards (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id TEXT NOT NULL,
    orch_task_id TEXT NOT NULL,

    -- 发送方（Orch Agent）
    sender_id TEXT NOT NULL,
    sender_name TEXT NOT NULL,
    sender_avatar TEXT NOT NULL DEFAULT '',

    -- 处理方（Worker Agent）
    worker_id TEXT NOT NULL,
    worker_name TEXT NOT NULL,
    worker_avatar TEXT NOT NULL DEFAULT '',

    -- 任务内容
    task_content TEXT NOT NULL,
    task_summary TEXT NOT NULL,

    -- 回复内容
    worker_result TEXT DEFAULT '',

    -- 状态
    status TEXT NOT NULL DEFAULT 'todo' CHECK (status IN ('todo', 'in_progress', 'done', 'failed')),
    priority TEXT NOT NULL DEFAULT 'medium',
    task_hash TEXT NOT NULL,

    -- 时间
    dispatched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orch_task_cards_conv ON orch_task_cards (conversation_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_orch_task_cards_hash ON orch_task_cards (task_hash);

-- DOWN: DROP TABLE IF EXISTS orch_task_cards;
