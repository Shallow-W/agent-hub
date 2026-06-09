-- Add task_hash column for content-based deduplication of Orch worker tasks.
-- Replaces the (orch_task_id, worker_name) pair as the dedup key, so multi-round
-- dispatches with different task descriptions create separate cards instead of
-- overwriting the previous round's card.

ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS task_hash TEXT;

-- Partial index for fast hash lookup; only rows that carry a hash.
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_task_hash
    ON workspace_tasks (task_hash) WHERE task_hash IS NOT NULL;

---- DOWN
DROP INDEX IF EXISTS idx_workspace_tasks_task_hash;
ALTER TABLE workspace_tasks DROP COLUMN IF EXISTS task_hash;
