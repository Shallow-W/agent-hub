-- 产物版本血缘：同一"逻辑产物"的多个版本用 root_id 串起来，version 递增。
-- root_id 指向血缘根（v1 的 id）；现有每行各自成为血缘根 v1。
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS root_id UUID;

-- 回填：现有每行 root_id 设为自身 id（各自成为血缘根 v1）。幂等：仅填空值。
UPDATE artifacts SET root_id = id WHERE root_id IS NULL;

-- 回填完成后置非空（幂等：列已是 NOT NULL 时重复执行无副作用）。
ALTER TABLE artifacts ALTER COLUMN root_id SET NOT NULL;

-- 按 root_id + version 查询版本列表 / 取最新版本的支撑索引。
CREATE INDEX IF NOT EXISTS idx_artifacts_root ON artifacts(root_id, version);

---- DOWN
DROP INDEX IF EXISTS idx_artifacts_root;
ALTER TABLE artifacts DROP COLUMN IF EXISTS root_id;
