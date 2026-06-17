-- 新增 platform 工具分类
INSERT INTO tool_categories (name, label, color, sort_order) VALUES
    ('platform', '平台内置', '#8c8c8c', 0)
ON CONFLICT (name) DO NOTHING;

-- render_card 是平台基础设施工具，所有 Agent 默认可用，不可禁用。
INSERT INTO tool_definitions (name, label, category, description, is_management) VALUES
    ('render_card', '卡片渲染', 'platform',
     '在聊天中渲染交互式卡片（方案选择、审批确认、任务进度、信息展示）', false)
ON CONFLICT (name) DO NOTHING;

-- 所有内置 toolset 模板自动包含 render_card
UPDATE builtin_toolset_templates
SET tool_names = tool_names || '["render_card"]'::jsonb
WHERE name IN ('basic', 'tasks', 'orchestrator', 'agent_builder', 'agent_manager', 'knowledge')
  AND NOT tool_names @> '["render_card"]';
