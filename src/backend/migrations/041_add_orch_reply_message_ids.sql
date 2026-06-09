ALTER TABLE orch_tasks ADD COLUMN IF NOT EXISTS source_message_id UUID REFERENCES messages(id) ON DELETE SET NULL;
ALTER TABLE orch_tasks ADD COLUMN IF NOT EXISTS dispatch_message_id UUID REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_orch_tasks_source_message ON orch_tasks (source_message_id) WHERE source_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_orch_tasks_dispatch_message ON orch_tasks (dispatch_message_id) WHERE dispatch_message_id IS NOT NULL;
