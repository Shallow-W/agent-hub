-- tool_definitions.description 改为 TEXT 以容纳较长的工具说明。
-- 原 VARCHAR(256) 在长描述（如 deploy_artifact）下不够用。
ALTER TABLE tool_definitions ALTER COLUMN description TYPE TEXT;
