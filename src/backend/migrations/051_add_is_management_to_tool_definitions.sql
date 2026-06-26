-- Add is_management column to tool_definitions. Management tools
-- (create_agent / update_agent / delete_agent) are hidden from the
-- tool catalog unless enable_management_tools is set on the agent.
ALTER TABLE tool_definitions
    ADD COLUMN IF NOT EXISTS is_management BOOLEAN NOT NULL DEFAULT false;

-- Mark the three management tools
UPDATE tool_definitions
SET is_management = true
WHERE name IN ('create_agent', 'update_agent', 'delete_agent');
