ALTER TABLE agents ADD COLUMN IF NOT EXISTS enable_management_tools BOOLEAN NOT NULL DEFAULT false;

---- DOWN
ALTER TABLE agents DROP COLUMN IF EXISTS enable_management_tools;
