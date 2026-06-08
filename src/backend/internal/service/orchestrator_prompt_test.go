package service

import (
	"strings"
	"testing"
)

func TestBuildOrchestratorPrompt_Normal(t *testing.T) {
	result := BuildOrchestratorPrompt(
		"项目讨论组",
		[]string{"Alice", "Bob", "Charlie"},
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
	if !strings.Contains(result, "[当前群聊]") {
		t.Error("prompt missing [当前群聊] section")
	}
	if !strings.Contains(result, "[群聊最近动态]") {
		t.Error("prompt missing [群聊最近动态] section")
	}
	if !strings.Contains(result, "[用户消息]") {
		t.Error("prompt missing [用户消息] section")
	}
}

func TestOrchestratorSystemPrompt_AllowsPromptAgentListFallback(t *testing.T) {
	if strings.Contains(OrchestratorSystemPrompt, "list_group_agents") {
		t.Fatal("orchestrator prompt should not mention MCP agent listing")
	}
	if !strings.Contains(OrchestratorSystemPrompt, "prompt 中提供的 Agent 列表") {
		t.Fatal("orchestrator prompt should allow using the provided agent list")
	}
	if !strings.Contains(OrchestratorSystemPrompt, "不要因为无法额外查询群聊成员") {
		t.Fatal("orchestrator prompt should not refuse when extra lookup is unavailable")
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
	result := BuildOrchestratorPrompt(
		"空群",
		[]string{},
		"some summary",
		"some request",
	)

	if !strings.Contains(result, "空群") {
		t.Error("prompt missing title")
	}
	if !strings.Contains(result, "some request") {
		t.Error("prompt missing user message")
	}
}

func TestBuildOrchestratorPrompt_SpecialChars(t *testing.T) {
	result := BuildOrchestratorPrompt(
		"群聊<>&\"'",
		[]string{"Agent-1", "Agent_2", "Agent.3"},
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
	agents := make([]string, 50)
	for i := range agents {
		agents[i] = "Agent" + strings.Repeat("名", 5)
	}

	result := BuildOrchestratorPrompt(longTitle, agents, "summary", longMsg)

	if !strings.Contains(result, longMsg) {
		t.Error("prompt missing long user message")
	}
	if !strings.Contains(result, "Agent"+strings.Repeat("名", 5)) {
		t.Error("prompt missing agent names")
	}
}
