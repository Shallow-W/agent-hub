package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Deployment tools ──

func DeployArtifact() port.MCPToolSpec {
	return routeSpec{
		name:        "deploy_artifact",
		label:       "部署产物",
		category:    "deployment",
		description: "将当前会话中的 artifact（代码/网页/文档）部署为可公开访问的预览页面。通过内网穿透(tunnel)生成临时公网 URL。不指定 artifact_name 时部署最新 artifact。注意：webpage 类型的 artifact 需要包含完整的 HTML 内容（content 字段）才能正确部署预览，仅包含 localhost URL 的产物无法通过公网访问。",
		inputSchema: schema(map[string]map[string]interface{}{
			"artifact_name": strProp("要部署的 artifact 名称（匹配 filename 或 title），不指定则部署最新"),
		}),
		routeInfo: &port.RouteInfo{Method: "POST", Path: "/api/deployments/deploy", Optional: []string{"artifact_name"}},
	}
}

func DeployArtifactGitHub() port.MCPToolSpec {
	return routeSpec{
		name:        "deploy_artifact_github",
		label:       "GitHub 发布",
		category:    "deployment",
		description: "将 artifact 永久发布到 GitHub Pages。需要后端配置 GitHub Token。不指定 artifact_name 时部署最新 artifact。",
		inputSchema: schema(map[string]map[string]interface{}{
			"artifact_name": strProp("要发布的 artifact 名称（匹配 filename 或 title），不指定则发布最新"),
		}),
		routeInfo: &port.RouteInfo{Method: "POST", Path: "/api/deployments/deploy", Optional: []string{"artifact_name"}},
	}
}
