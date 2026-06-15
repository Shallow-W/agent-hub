-- Ensure all tools registered via ToolRegistry are in tool_definitions.
-- Using ON CONFLICT DO NOTHING since the Go code's ToolRegistry.Register()
-- also upserts at startup. This migration is a safety net.
INSERT INTO tool_definitions (name, label, category, description) VALUES
    ('deploy_artifact', '部署产物', 'deployment', '将 artifact 部署为可公开访问的预览页面'),
    ('deploy_artifact_github', 'GitHub 发布', 'deployment', '将 artifact 永久发布到 GitHub Pages'),
    ('list_platform_skills', '平台 Skills', 'skill', '列出所有平台 Skill 摘要')
ON CONFLICT (name) DO NOTHING;
