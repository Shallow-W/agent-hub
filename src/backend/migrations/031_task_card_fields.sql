ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS worker_result TEXT;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;
