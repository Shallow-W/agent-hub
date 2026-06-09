package service

import (
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

func TestBuildWorkerDispatchPlanResolvesNormalizedNames(t *testing.T) {
	tasks := []DispatchTask{
		{AgentName: "codex", Task: "写测试"},
		{AgentName: "ClaudeCode", Task: "做评审"},
		{AgentName: "不存在", Task: "不会执行"},
	}
	agents := []model.ConversationAgent{
		{AgentID: "agent-codex", Name: "Codex"},
		{AgentID: "agent-claude", Name: "Claude Code"},
	}

	plan := BuildWorkerDispatchPlan(tasks, agents)

	if len(plan.Tasks) != 2 {
		t.Fatalf("expected 2 executable tasks, got %d", len(plan.Tasks))
	}
	if plan.Tasks[0].AgentID != "agent-codex" {
		t.Fatalf("expected codex task to resolve to agent-codex, got %q", plan.Tasks[0].AgentID)
	}
	if plan.Tasks[1].AgentID != "agent-claude" {
		t.Fatalf("expected ClaudeCode task to resolve to agent-claude, got %q", plan.Tasks[1].AgentID)
	}
	if len(plan.UnknownTasks) != 1 || plan.UnknownTasks[0].AgentName != "不存在" {
		t.Fatalf("expected one unknown task for 不存在, got %#v", plan.UnknownTasks)
	}
}
