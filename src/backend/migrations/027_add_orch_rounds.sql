-- Multi-round orchestration: add round tracking to orch_tasks
ALTER TABLE orch_tasks ADD COLUMN IF NOT EXISTS round INT NOT NULL DEFAULT 0;
ALTER TABLE orch_tasks ADD COLUMN IF NOT EXISTS round_history JSONB DEFAULT '[]';
