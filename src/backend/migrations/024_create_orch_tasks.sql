CREATE TABLE IF NOT EXISTS orch_tasks (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  orch_agent_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'planning',
  dispatch_plan JSONB,
  worker_status JSONB DEFAULT '{}',
  worker_results JSONB DEFAULT '{}',
  summary TEXT,
  original_message TEXT,
  kb_preload TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orch_tasks_conversation ON orch_tasks (conversation_id);
CREATE INDEX IF NOT EXISTS idx_orch_tasks_status ON orch_tasks (status);
