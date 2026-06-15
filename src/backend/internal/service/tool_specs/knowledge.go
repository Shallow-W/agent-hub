package tool_specs

import "github.com/agent-hub/backend/internal/port"

// ── Knowledge tools ──

func ListKnowledgeBases() port.MCPToolSpec {
	return routeSpec{
		name:        "list_knowledge_bases",
		label:       "知识库列表",
		category:    "knowledge",
		description: "列出当前用户的知识库，包含 ID、名称、描述、可见性、文件数量、创建时间等信息",
		inputSchema: noParams(),
		routeInfo:   &port.RouteInfo{Method: "GET", Path: "/mcp/knowledge-bases"},
	}
}

func ListKnowledgeFiles() port.MCPToolSpec {
	return routeSpec{
		name:        "list_knowledge_files",
		label:       "知识库文件列表",
		category:    "knowledge",
		description: "列出指定知识库中的文件，包含文件名、大小、类型、预览文本等信息",
		inputSchema: schema(map[string]map[string]interface{}{
			"knowledge_base_id": strProp("知识库 ID（必填）"),
		}, "knowledge_base_id"),
		routeInfo: &port.RouteInfo{Method: "GET", Path: "/mcp/knowledge-bases/{knowledge_base_id}/files", Required: []string{"knowledge_base_id"}},
	}
}

func SearchKnowledge() port.MCPToolSpec {
	return routeSpec{
		name:        "search_knowledge",
		label:       "知识库搜索",
		category:    "knowledge",
		description: "在指定知识库中按关键词搜索文件，基于文件的 preview_text 字段进行匹配过滤",
		inputSchema: schema(map[string]map[string]interface{}{
			"knowledge_base_id": strProp("知识库 ID（必填）"),
			"keyword":           strProp("搜索关键词（必填）"),
			"limit":             intProp("最多返回结果数（可选，默认 20）"),
		}, "knowledge_base_id", "keyword"),
		routeInfo: &port.RouteInfo{Method: "GET", Path: "/mcp/knowledge-bases/{knowledge_base_id}/search", Required: []string{"knowledge_base_id", "keyword"}, Optional: []string{"limit"}},
	}
}

func ReadKnowledgeFile() port.MCPToolSpec {
	return routeSpec{
		name:        "read_knowledge_file",
		label:       "读取知识库文件",
		category:    "knowledge",
		description: "读取指定知识库文件已抽取的文本内容。适合在搜索命中文件后按 file_id 获取完整可用上下文",
		inputSchema: schema(map[string]map[string]interface{}{
			"knowledge_base_id": strProp("知识库 ID（必填）"),
			"file_id":           strProp("文件 ID（必填）"),
		}, "knowledge_base_id", "file_id"),
		routeInfo: &port.RouteInfo{Method: "GET", Path: "/mcp/knowledge-bases/{knowledge_base_id}/files/{file_id}/text", Required: []string{"knowledge_base_id", "file_id"}},
	}
}
