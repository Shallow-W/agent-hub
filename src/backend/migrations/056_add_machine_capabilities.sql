-- daemon 机器能力清单（docker 等），JSON 数组字符串。
-- daemon 上报 capabilities，后端据此选合适的 machine 执行 docker 部署。
ALTER TABLE daemon_machines ADD COLUMN IF NOT EXISTS capabilities TEXT NOT NULL DEFAULT '[]';
