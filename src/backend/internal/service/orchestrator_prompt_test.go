package service

import (
	"strings"
	"testing"
)

func TestBuildOrchestratorPrompt_Normal(t *testing.T) {
	result := BuildOrchestratorPromptWithAgents(
		"项目讨论组",
		[]OrchestratorAgentDetail{
			{
				Name:        "Alice",
				Role:        "orchestrator",
				Status:      "online",
				Description: "负责需求拆解",
				Tags:        `["planning"]`,
			},
			{Name: "Bob", Role: "worker", Status: "online"},
			{Name: "Charlie", Role: "worker", Status: "offline"},
		},
		"{会话上下文黑板\n{用户 Pin 上下文\n- user: 这是长期约束\n}\n{用户手写上下文\n这是用户手写的背景\n}\n}\n\n",
		"- Alice: 已完成设计\n- Bob: 开始编码",
		"请分析这个需求并分派任务",
	)

	if result == "" {
		t.Fatal("expected non-empty prompt")
	}
	// OrchestratorSystemPrompt is now injected via context_messages as system prompt,
	// not included in the user prompt. Only check group context is present.
	if !strings.Contains(result, "项目讨论组") {
		t.Error("prompt missing conversation title")
	}
	// 包含所有 agent
	for _, name := range []string{"Alice", "Bob", "Charlie"} {
		if !strings.Contains(result, name) {
			t.Errorf("prompt missing agent %q", name)
		}
	}
	// 包含历史消息
	if !strings.Contains(result, "已完成设计") {
		t.Error("prompt missing recent messages")
	}
	// 包含用户消息
	if !strings.Contains(result, "请分析这个需求并分派任务") {
		t.Error("prompt missing user message")
	}
	// 包含结构段
	if !strings.Contains(result, "{当前群聊") {
		t.Error("prompt missing {当前群聊 section")
	}
	if !strings.Contains(result, "{当前群聊 Agent 详情") {
		t.Error("prompt missing {当前群聊 Agent 详情 section")
	}
	if !strings.Contains(result, "简介：负责需求拆解") {
		t.Error("prompt missing backend-provided description")
	}
	if !strings.Contains(result, `标签：["planning"]`) {
		t.Error("prompt missing backend-provided tags")
	}
	if strings.Contains(result, "CLI工具：") {
		t.Error("prompt should not include CLI tool field")
	}
	if strings.Contains(result, "能力：") || strings.Contains(result, `"tools":["read"]`) {
		t.Error("prompt should not include raw capabilities")
	}
	if !strings.Contains(result, "{群聊最近动态") {
		t.Error("prompt missing {群聊最近动态 section")
	}
	if !strings.Contains(result, "{会话上下文黑板") {
		t.Error("prompt missing {会话上下文黑板 section")
	}
	if !strings.Contains(result, "这是长期约束") {
		t.Error("prompt missing pinned blackboard context")
	}
	if !strings.Contains(result, "这是用户手写的背景") {
		t.Error("prompt missing manual blackboard context")
	}
	if !strings.Contains(result, "{用户消息") {
		t.Error("prompt missing {用户消息 section")
	}
	if !strings.Contains(result, "{Orchestrator 指令") {
		t.Error("prompt missing {Orchestrator 指令 section")
	}
}

func TestBuildOrchestratorPromptWithAgents_DoesNotLeakToolPrompts(t *testing.T) {
	result := BuildOrchestratorPromptWithAgents(
		"测试群",
		[]OrchestratorAgentDetail{
			{
				Name:        "员工1",
				Role:        "worker",
				Status:      "online",
				Description: "",
				Tags:        `["coding"]`,
			},
		},
		"",
		"",
		"@员工2 分派任务",
	)

	for _, forbidden := range []string{
		"CLI工具：",
		"能力：",
		"ablation-planner",
		"你可以通过平台提供的管理工具执行以下操作",
		"查看平台上的所有 Agent 列表",
	} {
		if strings.Contains(result, forbidden) {
			t.Fatalf("prompt leaked forbidden text %q", forbidden)
		}
	}
	if !strings.Contains(result, "简介：未配置") {
		t.Error("prompt should show missing description as unconfigured")
	}
	if !strings.Contains(result, `标签：["coding"]`) {
		t.Error("prompt should keep backend-provided tags")
	}
}

func TestBuildOrchestratorPrompt_EmptySummary(t *testing.T) {
	result := BuildOrchestratorPrompt(
		"测试群",
		[]string{"Agent1"},
		"",
		"hello",
	)

	if !strings.Contains(result, "测试群") {
		t.Error("prompt missing title")
	}
	if !strings.Contains(result, "Agent1") {
		t.Error("prompt missing agent list")
	}
	if !strings.Contains(result, "hello") {
		t.Error("prompt missing user message")
	}
}

func TestBuildOrchestratorPrompt_EmptyAgentList(t *testing.T) {
	result := BuildOrchestratorPromptWithAgents(
		"空群",
		[]OrchestratorAgentDetail{},
		"",
		"some summary",
		"some request",
	)

	if !strings.Contains(result, "空群") {
		t.Error("prompt missing title")
	}
	if !strings.Contains(result, "some request") {
		t.Error("prompt missing user message")
	}
	if !strings.Contains(result, "{当前群聊 Agent 详情\n无\n}") {
		t.Error("prompt should show no agent details")
	}
}

func TestBuildOrchestratorPrompt_SpecialChars(t *testing.T) {
	result := BuildOrchestratorPromptWithAgents(
		"群聊<>&\"'",
		[]OrchestratorAgentDetail{
			{Name: "Agent-1"},
			{Name: "Agent_2"},
			{Name: "Agent.3"},
		},
		"",
		"消息含<特殊>字符 & \"引号\"",
		"用户消息含 `backtick` 和 $ 符号",
	)

	if !strings.Contains(result, "Agent-1") {
		t.Error("prompt missing Agent-1")
	}
	if !strings.Contains(result, "Agent_2") {
		t.Error("prompt missing Agent_2")
	}
	if !strings.Contains(result, "Agent.3") {
		t.Error("prompt missing Agent.3")
	}
	if !strings.Contains(result, "消息含<特殊>字符") {
		t.Error("prompt missing special chars in summary")
	}
}

func TestBuildOrchestratorPrompt_LongInput(t *testing.T) {
	longTitle := strings.Repeat("标题", 500)
	longMsg := strings.Repeat("这是一条很长的用户消息。", 200)
	agents := make([]OrchestratorAgentDetail, 50)
	for i := range agents {
		agents[i] = OrchestratorAgentDetail{Name: "Agent" + strings.Repeat("名", 5)}
	}

	result := BuildOrchestratorPromptWithAgents(longTitle, agents, "", "summary", longMsg)

	if !strings.Contains(result, longMsg) {
		t.Error("prompt missing long user message")
	}
	if !strings.Contains(result, "Agent"+strings.Repeat("名", 5)) {
		t.Error("prompt missing agent names")
	}
}

func TestBuildOrchestratorPromptWithAgents_EmptyFieldsUseFallback(t *testing.T) {
	result := BuildOrchestratorPromptWithAgents(
		"测试群",
		[]OrchestratorAgentDetail{{Name: "Codex"}},
		"",
		"",
		"@员工2 分派任务",
	)

	if strings.Contains(result, "擅长") {
		t.Error("prompt should not invent agent description")
	}
	if !strings.Contains(result, "简介：未配置") {
		t.Error("prompt should show missing description as unconfigured")
	}
	if !strings.Contains(result, "标签：未配置") {
		t.Error("prompt should show missing tags as unconfigured")
	}
	if !strings.Contains(result, "只能使用“当前群聊 Agent 详情”里真实存在的 Agent 名称") {
		t.Error("prompt missing anti-hallucination dispatch instruction")
	}
}
