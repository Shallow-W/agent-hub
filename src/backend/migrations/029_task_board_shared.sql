ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS orch_task_id TEXT;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS worker_name TEXT;

---- DOWN
ALTER TABLE workspace_tasks DROP COLUMN IF EXISTS orch_task_id;
ALTER TABLE workspace_tasks DROP COLUMN IF EXISTS worker_name;
