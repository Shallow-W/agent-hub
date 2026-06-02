package service

import (
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

// ---------------------------------------------------------------------------
// ParseMentions
// ---------------------------------------------------------------------------

func TestParseMentionsBasic(t *testing.T) {
	text := "@Alice 请分析数据 @Bob 写测试报告"
	results := ParseMentions(text)

	if len(results) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(results))
	}
	if results[0].AgentName != "Alice" {
		t.Errorf("first agent = %q, want Alice", results[0].AgentName)
	}
	if results[0].Task != "请分析数据" {
		t.Errorf("first task = %q", results[0].Task)
	}
	if results[1].AgentName != "Bob" {
		t.Errorf("second agent = %q, want Bob", results[1].AgentName)
	}
	if results[1].Task != "写测试报告" {
		t.Errorf("second task = %q", results[1].Task)
	}
}

func TestParseMentionsSingleMention(t *testing.T) {
	text := "@Agent 请完成任务"
	results := ParseMentions(text)

	if len(results) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(results))
	}
	if results[0].AgentName != "Agent" {
		t.Errorf("agent = %q, want Agent", results[0].AgentName)
	}
	if results[0].Task != "请完成任务" {
		t.Errorf("task = %q", results[0].Task)
	}
}

func TestParseMentionsNoMentions(t *testing.T) {
	results := ParseMentions("这是一条普通消息，没有任何提及")
	if results != nil {
		t.Fatalf("expected nil, got %v", results)
	}
}

func TestParseMentionsEmptyString(t *testing.T) {
	results := ParseMentions("")
	if results != nil {
		t.Fatalf("expected nil, got %v", results)
	}
}

func TestParseMentionsConsecutive(t *testing.T) {
	text := "@Alice@Bob 协作完成"
	results := ParseMentions(text)

	if len(results) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(results))
	}
	// Between @Alice and @Bob there's no text
	if results[0].Task != "" {
		t.Errorf("first task = %q, want empty", results[0].Task)
	}
	if results[1].Task != "协作完成" {
		t.Errorf("second task = %q", results[1].Task)
	}
}

func TestParseMentionAtEndOfText(t *testing.T) {
	text := "请处理这个任务 @Agent"
	results := ParseMentions(text)

	if len(results) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(results))
	}
	if results[0].AgentName != "Agent" {
		t.Errorf("agent = %q, want Agent", results[0].AgentName)
	}
	if results[0].Task != "" {
		t.Errorf("task = %q, want empty (mention at end)", results[0].Task)
	}
}

func TestParseMentionsCJKNames(t *testing.T) {
	text := "@分析器 请分析 @编码器.1 编写代码"
	results := ParseMentions(text)

	if len(results) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(results))
	}
	if results[0].AgentName != "分析器" {
		t.Errorf("first agent = %q, want 分析器", results[0].AgentName)
	}
	if results[1].AgentName != "编码器.1" {
		t.Errorf("second agent = %q, want 编码器.1", results[1].AgentName)
	}
}

func TestParseMentionsHyphenUnderscoreDot(t *testing.T) {
	text := "@my-agent_v2.0 开始工作"
	results := ParseMentions(text)

	if len(results) != 1 {
		t.Fatalf("expected 1 mention, got %d", len(results))
	}
	if results[0].AgentName != "my-agent_v2.0" {
		t.Errorf("agent = %q, want my-agent_v2.0", results[0].AgentName)
	}
}

func TestParseMentionsWhitespaceTrimmed(t *testing.T) {
	text := "@Alice   完成任务   \n\n@Bob  写文档  "
	results := ParseMentions(text)

	if len(results) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(results))
	}
	if results[0].Task != "完成任务" {
		t.Errorf("first task = %q, want 完成任务", results[0].Task)
	}
	if results[1].Task != "写文档" {
		t.Errorf("second task = %q, want 写文档", results[1].Task)
	}
}

// ---------------------------------------------------------------------------
// ParseOrchestratorOutput
// ---------------------------------------------------------------------------

func TestParseOrchOutputBasic(t *testing.T) {
	text := "好的，我来分配任务：\n@Alice 分析数据\n@Bob 写报告"
	dispatch := ParseOrchestratorOutput(text)

	if dispatch == nil {
		t.Fatal("expected non-nil dispatch")
	}
	if dispatch.Preamble != "好的，我来分配任务：" {
		t.Errorf("preamble = %q", dispatch.Preamble)
	}
	if len(dispatch.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(dispatch.Tasks))
	}
	if dispatch.Tasks[0].AgentName != "Alice" {
		t.Errorf("task[0] agent = %q", dispatch.Tasks[0].AgentName)
	}
	if dispatch.Tasks[1].AgentName != "Bob" {
		t.Errorf("task[1] agent = %q", dispatch.Tasks[1].AgentName)
	}
}

func TestParseOrchOutputNilOnNoMentions(t *testing.T) {
	dispatch := ParseOrchestratorOutput("这是一条普通消息")
	if dispatch != nil {
		t.Fatal("expected nil for text without @mentions")
	}
}

func TestParseOrchOutputSequential(t *testing.T) {
	text := "分配任务：\n@Alice 先分析数据\n→ @Bob 等Alice完成后写报告"
	dispatch := ParseOrchestratorOutput(text)

	if dispatch == nil {
		t.Fatal("expected non-nil dispatch")
	}
	if len(dispatch.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(dispatch.Tasks))
	}
	if dispatch.Tasks[0].Sequential {
		t.Error("first task should not be sequential")
	}
	if !dispatch.Tasks[1].Sequential {
		t.Error("second task should be sequential")
	}
}

func TestParseOrchOutputDependsOn(t *testing.T) {
	text := "@Alice 分析数据\n@Bob 根据 @Alice 的结果写报告"
	dispatch := ParseOrchestratorOutput(text)

	if dispatch == nil {
		t.Fatal("expected non-nil dispatch")
	}
	if len(dispatch.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(dispatch.Tasks))
	}
	if dispatch.Tasks[1].DependsOn != "Alice" {
		t.Errorf("DependsOn = %q, want Alice", dispatch.Tasks[1].DependsOn)
	}
	// Embedded @reference is stripped from task text
	if dispatch.Tasks[1].Task != "根据 的结果写报告" {
		t.Errorf("Task = %q, want 根据 的结果写报告", dispatch.Tasks[1].Task)
	}
}

func TestParseOrchOutputDependsOnWithoutKeyword(t *testing.T) {
	text := "@Alice 分析数据\n@Bob 参考 @Alice 的分析来写报告"
	dispatch := ParseOrchestratorOutput(text)

	if dispatch == nil {
		t.Fatal("expected non-nil dispatch")
	}
	// Even without "根据", a bare @Agent reference in task text counts as DependsOn
	if dispatch.Tasks[1].DependsOn != "Alice" {
		t.Errorf("DependsOn = %q, want Alice", dispatch.Tasks[1].DependsOn)
	}
	if dispatch.Tasks[1].Task != "参考 的分析来写报告" {
		t.Errorf("Task = %q", dispatch.Tasks[1].Task)
	}
}

func TestParseOrchOutputSingleMention(t *testing.T) {
	text := "@Agent 完成所有工作"
	dispatch := ParseOrchestratorOutput(text)

	if dispatch == nil {
		t.Fatal("expected non-nil dispatch")
	}
	if dispatch.Preamble != "" {
		t.Errorf("preamble = %q, want empty", dispatch.Preamble)
	}
	if len(dispatch.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(dispatch.Tasks))
	}
	if dispatch.Tasks[0].AgentName != "Agent" {
		t.Errorf("agent = %q", dispatch.Tasks[0].AgentName)
	}
}

func TestParseOrchOutputSequentialWithSpaces(t *testing.T) {
	text := "@Alice 分析\n  →  @Bob 写报告"
	dispatch := ParseOrchestratorOutput(text)

	if dispatch == nil {
		t.Fatal("expected non-nil dispatch")
	}
	if !dispatch.Tasks[1].Sequential {
		t.Error("second task should be sequential with leading spaces before →")
	}
}

// ---------------------------------------------------------------------------
// FindMentionedAgentID
// ---------------------------------------------------------------------------

func TestFindMentionedAgentIDBasic(t *testing.T) {
	mentions := []MentionResult{
		{AgentName: "Alice", Task: "分析数据"},
		{AgentName: "Bob", Task: "写报告"},
	}
	agents := []model.ConversationAgent{
		{AgentID: "agent-1", Name: "Alice"},
		{AgentID: "agent-2", Name: "Bob"},
		{AgentID: "agent-3", Name: "Charlie"},
	}

	result := FindMentionedAgentID(mentions, agents)
	if len(result) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(result))
	}
	if result["Alice"] != "agent-1" {
		t.Errorf("Alice = %q, want agent-1", result["Alice"])
	}
	if result["Bob"] != "agent-2" {
		t.Errorf("Bob = %q, want agent-2", result["Bob"])
	}
}

func TestFindMentionedAgentIDSkipUnknown(t *testing.T) {
	mentions := []MentionResult{
		{AgentName: "Alice", Task: "分析"},
		{AgentName: "UnknownUser", Task: "查看"},
	}
	agents := []model.ConversationAgent{
		{AgentID: "agent-1", Name: "Alice"},
	}

	result := FindMentionedAgentID(mentions, agents)
	if len(result) != 1 {
		t.Fatalf("expected 1 match (UnknownUser skipped), got %d", len(result))
	}
	if _, ok := result["UnknownUser"]; ok {
		t.Error("UnknownUser should not be in result")
	}
}

func TestFindMentionedAgentIDEmpty(t *testing.T) {
	result := FindMentionedAgentID(nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(result))
	}
}

func TestFindMentionedAgentIDNoMatch(t *testing.T) {
	mentions := []MentionResult{
		{AgentName: "Ghost", Task: "什么都不做"},
	}
	agents := []model.ConversationAgent{
		{AgentID: "agent-1", Name: "Alice"},
	}

	result := FindMentionedAgentID(mentions, agents)
	if len(result) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(result))
	}
}
