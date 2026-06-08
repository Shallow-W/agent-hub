ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS orch_task_id TEXT;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS worker_name TEXT;

-- Allow NULL user_id for Orch-created tasks (no human owner).
ALTER TABLE workspace_tasks ALTER COLUMN user_id DROP NOT NULL;

-- Partial index for fast lookup of Orch worker cards; only rows with orch_task_id.
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_orch_worker ON workspace_tasks (orch_task_id, worker_name) WHERE orch_task_id IS NOT NULL;

---- DOWN
DROP INDEX IF EXISTS idx_workspace_tasks_orch_worker;
ALTER TABLE workspace_tasks ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE workspace_tasks DROP COLUMN IF EXISTS orch_task_id;
ALTER TABLE workspace_tasks DROP COLUMN IF EXISTS worker_name;
