package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Knowledge tools ──

func ListKnowledgeBases() port.MCPToolSpec {
	return newRouteSpec(
		"list_knowledge_bases",
		"知识库列表",
		"knowledge",
		"列出当前用户的知识库，包含 ID、名称、描述、可见性、文件数量、创建时间等信息",
		noParams(),
		&port.RouteInfo{Method: "GET", Path: "/mcp/knowledge-bases"},
	)
}

func ListKnowledgeFiles() port.MCPToolSpec {
	return newRouteSpec(
		"list_knowledge_files",
		"知识库文件列表",
		"knowledge",
		"列出指定知识库中的文件，包含文件名、大小、类型、预览文本等信息",
		schema(map[string]map[string]interface{}{
			"knowledge_base_id": strProp("知识库 ID（必填）"),
		}, "knowledge_base_id"),
		&port.RouteInfo{Method: "GET", Path: "/mcp/knowledge-bases/{knowledge_base_id}/files", Required: []string{"knowledge_base_id"}},
	)
}

func SearchKnowledge() port.MCPToolSpec {
	return newRouteSpec(
		"search_knowledge",
		"知识库搜索",
		"knowledge",
		"在指定知识库中按关键词搜索文件，基于文件的 preview_text 字段进行匹配过滤",
		schema(map[string]map[string]interface{}{
			"knowledge_base_id": strProp("知识库 ID（必填）"),
			"keyword":           strProp("搜索关键词（必填）"),
			"limit":             intProp("最多返回结果数（可选，默认 20）"),
		}, "knowledge_base_id", "keyword"),
		&port.RouteInfo{Method: "GET", Path: "/mcp/knowledge-bases/{knowledge_base_id}/search", Required: []string{"knowledge_base_id", "keyword"}, Optional: []string{"limit"}},
	)
}

func ReadKnowledgeFile() port.MCPToolSpec {
	return newRouteSpec(
		"read_knowledge_file",
		"读取知识库文件",
		"knowledge",
		"读取指定知识库文件已抽取的文本内容。适合在搜索命中文件后按 file_id 获取完整可用上下文",
		schema(map[string]map[string]interface{}{
			"knowledge_base_id": strProp("知识库 ID（必填）"),
			"file_id":           strProp("文件 ID（必填）"),
		}, "knowledge_base_id", "file_id"),
		&port.RouteInfo{Method: "GET", Path: "/mcp/knowledge-bases/{knowledge_base_id}/files/{file_id}/text", Required: []string{"knowledge_base_id", "file_id"}},
	)
}
