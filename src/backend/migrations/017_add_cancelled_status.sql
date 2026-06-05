-- 添加 cancelled 到任务状态 CHECK 约束
ALTER TABLE workspace_tasks DROP CONSTRAINT chk_workspace_tasks_status;
ALTER TABLE workspace_tasks ADD CONSTRAINT chk_workspace_tasks_status
    CHECK (status IN ('todo', 'in_progress', 'blocked', 'done', 'cancelled'));

---- DOWN
ALTER TABLE workspace_tasks DROP CONSTRAINT chk_workspace_tasks_status;
ALTER TABLE workspace_tasks ADD CONSTRAINT chk_workspace_tasks_status
    CHECK (status IN ('todo', 'in_progress', 'blocked', 'done'));
