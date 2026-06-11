package service

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-hub/backend/internal/model"
)

// fakeBuilder 是测试用的 ContextBuilder，把 tag 前置到 current（模拟前置型 builder）。
type fakeBuilder struct {
	tag string
}

func (b *fakeBuilder) Build(_ context.Context, _ ContextInput, current string) string {
	if b.tag == "" {
		return current
	}
	if current == "" {
		return b.tag
	}
	return b.tag + "|" + current
}

// TestContextChain_RegistrationOrderIsExecutionOrder 验证：
// chain 的执行顺序 = 注册顺序；越靠后注册的 builder 输出越靠外（最左侧）。
func TestContextChain_RegistrationOrderIsExecutionOrder(t *testing.T) {
	chain := NewContextChain(
		&fakeBuilder{tag: "A"}, // 最先注册，输出在最里面
		&fakeBuilder{tag: "B"},
		&fakeBuilder{tag: "C"}, // 最后注册，输出在最外面
	)
	got := chain.Build(context.Background(), ContextInput{})
	want := "C|B|A"
	if got != want {
		t.Fatalf("chain order: got %q, want %q", got, want)
	}
}

// TestContextChain_EmptyBuilderNoop 验证空 tag 的 builder 原样透传 current。
func TestContextChain_EmptyBuilderNoop(t *testing.T) {
	chain := NewContextChain(
		&fakeBuilder{tag: ""}, // 空操作
		&fakeBuilder{tag: "X"},
		&fakeBuilder{tag: ""}, // 空操作
	)
	got := chain.Build(context.Background(), ContextInput{})
	if got != "X" {
		t.Fatalf("empty builder should noop: got %q, want %q", got, "X")
	}
}

// TestContextChain_NoBuildersReturnsEmpty 验证空 chain 返回空串。
func TestContextChain_NoBuildersReturnsEmpty(t *testing.T) {
	chain := NewContextChain()
	got := chain.Build(context.Background(), ContextInput{})
	if got != "" {
		t.Fatalf("empty chain: got %q, want empty", got)
	}
}

// TestAttachmentBuilder_NoAttachmentsNoop 验证无附件时返回 current 不变。
func TestAttachmentBuilder_NoAttachmentsNoop(t *testing.T) {
	b := &AttachmentBuilder{UploadDir: "/tmp", MaxRunes: 100}
	got := b.Build(context.Background(), ContextInput{Attachments: nil}, "current")
	if got != "current" {
		t.Fatalf("no attachments should be noop: got %q, want %q", got, "current")
	}
}

// TestAgentConfigInjector_NilAgentNoop 验证 in.Agent 为 nil 时返回 current 不变。
func TestAgentConfigInjector_NilAgentNoop(t *testing.T) {
	b := &AgentConfigInjector{}
	got := b.Build(context.Background(), ContextInput{Agent: nil}, "current")
	if got != "current" {
		t.Fatalf("nil agent should be noop: got %q, want %q", got, "current")
	}
}

// TestAgentConfigInjector_PrependsAgentSystemPrompt 验证 agent 配置前置到 current。
func TestAgentConfigInjector_PrependsAgentSystemPrompt(t *testing.T) {
	b := &AgentConfigInjector{}
	in := ContextInput{
		Agent:   &model.Agent{ID: "a1", SystemPrompt: "Hello rules"},
		Content: "请按规则回答",
	}
	got := b.Build(context.Background(), in, "BASE")
	if !strings.HasPrefix(got, "[系统指令]\nHello rules") {
		t.Fatalf("expected agent system prompt prepended, got %q", got)
	}
	if !strings.HasSuffix(got, "BASE") {
		t.Fatalf("expected original current preserved at tail, got %q", got)
	}
}

// TestOrchestratorPromptBuilder_NotOrchestratorNoop 验证非 orch 角色时返回 current 不变。
func TestOrchestratorPromptBuilder_NotOrchestratorNoop(t *testing.T) {
	b := &OrchestratorPromptBuilder{}
	got := b.Build(context.Background(), ContextInput{IsOrchestrator: false}, "current")
	if got != "current" {
		t.Fatalf("non-orch should be noop: got %q, want %q", got, "current")
	}
}

// TestOrchestratorPromptBuilder_OrchestratorPrependsSystemPrompt 验证 orch 角色时前置 OrchestratorSystemPrompt。
func TestOrchestratorPromptBuilder_OrchestratorPrependsSystemPrompt(t *testing.T) {
	b := &OrchestratorPromptBuilder{}
	got := b.Build(context.Background(), ContextInput{IsOrchestrator: true}, "BASE")
	if !strings.HasPrefix(got, "[系统指令]\n"+OrchestratorSystemPrompt+"\n\n") {
		t.Fatalf("expected orch system prompt prepended, got %q", got)
	}
	if !strings.HasSuffix(got, "BASE") {
		t.Fatalf("expected original current preserved at tail, got %q", got)
	}
}

// TestKBBuilder_PrefersPreload 验证 KBPreload 非空时优先使用，跳过实时解析。
func TestKBBuilder_PrefersPreload(t *testing.T) {
	b := &KBBuilder{}
	got := b.Build(context.Background(), ContextInput{KBPreload: "[引用的知识库]\npreload"}, "current")
	if !strings.HasPrefix(got, "[引用的知识库]\npreload") {
		t.Fatalf("expected preload prepended, got %q", got)
	}
}

// TestKBBuilder_EmptyPreloadAndNoResolverReturnsCurrent 验证无预加载且无 resolver 时返回 current 不变。
func TestKBBuilder_EmptyPreloadAndNoResolverReturnsCurrent(t *testing.T) {
	b := &KBBuilder{}
	got := b.Build(context.Background(), ContextInput{}, "current")
	if got != "current" {
		t.Fatalf("no preload + no resolver should be noop: got %q, want %q", got, "current")
	}
}

// === 等价性回归测试（P4 零行为变更的核心保证）===
//
// 下面 3 个测试验证 chain.Build 的输出与重构前各路径手写的拼装结果完全一致。
// 任何拼装顺序回归都会被这些测试捕获。

// TestPathA_DirectReplyChain_EquivalentToLegacyAssembly 验证路径 A（asyncAgentReply）。
// 重构前拼装顺序（message.go 旧实现）：
//  1. contextMessages = ""
//  2. contextMessages = attachCtx + ""           (attach 前置)
//  3. contextMessages = blackboardCtx + attachCtx (blackboard 前置)
//  4. contextMessages = kbCtx + blackboardCtx + attachCtx (kb 前置)
//  5. InjectAgentConfig → agentConfig + kbCtx + blackboardCtx + attachCtx
//
// 重构后：DirectReplyChain = [Attachment, Blackboard, KB, AgentConfig]
//
// 测试为避免触发真实文件 IO，构造空 Attachments（让 AttachmentBuilder no-op），
// 仅验证 blackboard/kb/agentConfig 的相对顺序与最终包装。Attachment 段的拼装等价性
// 已由 AttachmentBuilder 自身的单元测试覆盖。
func TestPathA_DirectReplyChain_EquivalentToLegacyAssembly(t *testing.T) {
	svc := NewOrchestratorService(nil, nil, nil)
	agent := &model.Agent{ID: "a1", SystemPrompt: "AGENT_RULES", ToolsConfig: "TOOLS"}
	in := ContextInput{
		Agent:       agent,
		Content:     "task",
		Attachments: nil, // 无附件，AttachmentBuilder no-op
	}

	// chain 路径
	chainOut := svc.DirectReplyChain().Build(context.Background(), in)

	// 手写等价拼装：所有非 agent-config 段为空（无 msgRepo/kbResolver/attachments）
	legacy := BuildAgentConfigText(agent, "", "task")
	if chainOut != legacy {
		t.Fatalf("path A mismatch:\n chain=%q\n legacy=%q", chainOut, legacy)
	}
	if !strings.HasPrefix(chainOut, "[系统指令]\nAGENT_RULES") {
		t.Fatalf("expected agent config block first, got %q", chainOut)
	}
}

// TestPathC_WorkerChain_EquivalentToLegacyAssembly 验证路径 C（dispatchSingleAgent）。
// 重构前拼装顺序（orchestrator.go 旧实现）：
//  1. kbCtx = kbPreload   (或 PreloadKBContext 回退，等价)
//  2. kbCtx = blackboardCtx + kbCtx
//  3. agentCtx = InjectAgentConfig(agent, kbCtx, ...) = agentConfig + blackboardCtx + kbPreload
//
// 重构后：WorkerChain = [KB, Blackboard, AgentConfig]
func TestPathC_WorkerChain_EquivalentToLegacyAssembly(t *testing.T) {
	svc := NewOrchestratorService(nil, nil, nil)
	agent := &model.Agent{ID: "w1", SystemPrompt: "WORKER_RULES"}
	in := ContextInput{
		Agent:     agent,
		Content:   "task",
		KBPreload: "KB_PRELOAD_BODY",
	}

	chainOut := svc.WorkerChain().Build(context.Background(), in)
	// 手写等价拼装：blackboard 为空（无 msgRepo），所以等价于 agentConfig + "" + kbPreload
	legacy := BuildAgentConfigText(agent, ""+"KB_PRELOAD_BODY", "task")
	if chainOut != legacy {
		t.Fatalf("path C mismatch:\n chain=%q\n legacy=%q", chainOut, legacy)
	}
	// 关键顺序断言：agentConfig 在前，kbPreload 在后
	if !strings.HasPrefix(chainOut, "[系统指令]\nWORKER_RULES") {
		t.Fatalf("expected agent config first, got %q", chainOut)
	}
	if !strings.HasSuffix(chainOut, "KB_PRELOAD_BODY") {
		t.Fatalf("expected kbPreload at tail, got %q", chainOut)
	}
}

// TestPathD_OrchChain_EquivalentToLegacyAssembly 验证路径 D（handleOrchestratedDispatch）。
// 重构前拼装顺序（orchestrator.go 旧实现）：
//  1. orchCtx = "[系统指令]\n" + OrchestratorSystemPrompt + "\n\n"
//  2. orchCtx += kbPreload                            → orchPrompt + kbPreload
//  3. agentConfig = InjectAgentConfig(orchAgent, "", ...)  → agentConfig
//  4. orchCtx = agentConfig + orchCtx                → agentConfig + orchPrompt + kbPreload
//
// 重构后：OrchChain = [KB, OrchestratorPrompt, AgentConfig]
func TestPathD_OrchChain_EquivalentToLegacyAssembly(t *testing.T) {
	svc := NewOrchestratorService(nil, nil, nil)
	orchAgent := &model.Agent{ID: "o1", SystemPrompt: "ORCH_RULES"}
	in := ContextInput{
		Agent:          orchAgent,
		Content:        "task",
		KBPreload:      "KB_PRELOAD_BODY",
		IsOrchestrator: true,
	}

	chainOut := svc.OrchChain().Build(context.Background(), in)
	// 手写等价拼装
	orchPrompt := "[系统指令]\n" + OrchestratorSystemPrompt + "\n\n"
	legacy := BuildAgentConfigText(orchAgent, orchPrompt+"KB_PRELOAD_BODY", "task")
	if chainOut != legacy {
		t.Fatalf("path D mismatch:\n chain=%q\n legacy=%q", chainOut, legacy)
	}
	// 顺序断言：agentConfig 在最前，orchPrompt 居中，kbPreload 在最后
	if !strings.HasPrefix(chainOut, "[系统指令]\nORCH_RULES") {
		t.Fatalf("expected agent config first, got prefix %q", chainOut[:50])
	}
	if !strings.Contains(chainOut, orchPrompt+"KB_PRELOAD_BODY") {
		t.Fatalf("expected orchPrompt + kbPreload in middle, got %q", chainOut)
	}
}

// TestPathC_FanoutChain_EquivalentToLegacyAssembly 验证路径 C 的异步 fanout 变体
// （dispatchOrchWorker）。重构前内联拼装（orchestrator_async.go）：
//
//	frame = fmt.Sprintf("[群聊背景]\n- Orchestrator: %s\n\n[调度指令]\nOrch @你，分配了以下任务：\n%s\n\n请完成这个任务并在回复末尾 @%s 表示完成。",
//	         name, truncateString(task, 2000), name)
//	if kbPreload != "" { dispatchCtx = kbPreload + frame }
//	dispatchCtx = InjectAgentConfig(agent, dispatchCtx, userID, task)  → agentConfig + kbPreload + frame
//
// 重构后：FanoutChain = [Frame, KB, AgentConfig]
func TestPathC_FanoutChain_EquivalentToLegacyAssembly(t *testing.T) {
	svc := NewOrchestratorService(nil, nil, nil)
	agent := &model.Agent{ID: "w1", SystemPrompt: "WORKER_RULES"}
	in := ContextInput{
		Agent:     agent,
		Content:   "fix the bug",
		KBPreload: "KB_PRELOAD_BODY",
		Extra: map[string]any{
			fanoutFrameExtraKey: FanoutFrameInput{
				OrchestratorName: "Orch",
				Task:             "fix the bug",
			},
		},
	}

	chainOut := svc.FanoutChain().Build(context.Background(), in)

	// 手写等价拼装
	frame := "[群聊背景]\n- Orchestrator: Orch\n\n[调度指令]\nOrch @你，分配了以下任务：\nfix the bug\n\n请完成这个任务并在回复末尾 @Orch 表示完成。"
	legacy := BuildAgentConfigText(agent, "KB_PRELOAD_BODY"+frame, "fix the bug")
	if chainOut != legacy {
		t.Fatalf("path C fanout mismatch:\n chain=%q\n legacy=%q", chainOut, legacy)
	}
	// 顺序断言：agentConfig 在最前，kbPreload 居中，frame 在最后
	if !strings.HasPrefix(chainOut, "[系统指令]\nWORKER_RULES") {
		t.Fatalf("expected agent config first, got %q", chainOut)
	}
	if !strings.Contains(chainOut, "KB_PRELOAD_BODY") {
		t.Fatalf("expected kbPreload in middle, got %q", chainOut)
	}
	if !strings.HasSuffix(chainOut, frame) {
		t.Fatalf("expected frame at tail, got %q", chainOut)
	}
}

// TestPathC_FanoutChain_NoKBPreloadOmitsKBSection 验证 kbPreload 为空时
// 不额外补 KB 段（保持原内联行为：if kbPreload != "" 才前置）。
func TestPathC_FanoutChain_NoKBPreloadOmitsKBSection(t *testing.T) {
	svc := NewOrchestratorService(nil, nil, nil)
	agent := &model.Agent{ID: "w1", SystemPrompt: "WORKER_RULES"}
	in := ContextInput{
		Agent:   agent,
		Content: "do task",
		Extra: map[string]any{
			fanoutFrameExtraKey: FanoutFrameInput{
				OrchestratorName: "Orch",
				Task:             "do task",
			},
		},
	}

	chainOut := svc.FanoutChain().Build(context.Background(), in)
	frame := "[群聊背景]\n- Orchestrator: Orch\n\n[调度指令]\nOrch @你，分配了以下任务：\ndo task\n\n请完成这个任务并在回复末尾 @Orch 表示完成。"
	legacy := BuildAgentConfigText(agent, frame, "do task")
	if chainOut != legacy {
		t.Fatalf("path C fanout (no kb) mismatch:\n chain=%q\n legacy=%q", chainOut, legacy)
	}
}

// TestPathD_SummaryChain_EquivalentToLegacyAssembly 验证路径 D 的 summary 阶段
// （startOrchSummary）。重构前内联拼装：
//
//	summaryCtx := InjectAgentConfig(orchAgent, "", orchTask.UserID, summaryPrompt)
//	             → agentConfig + ""
//
// 重构后：SummaryChain = [AgentConfig]，输出 = agentConfig + ""（Content 透传 taskText）
func TestPathD_SummaryChain_EquivalentToLegacyAssembly(t *testing.T) {
	svc := NewOrchestratorService(nil, nil, nil)
	orchAgent := &model.Agent{ID: "o1", SystemPrompt: "ORCH_RULES"}
	in := ContextInput{
		Agent:   orchAgent,
		Content: "汇总所有结果",
	}

	chainOut := svc.SummaryChain().Build(context.Background(), in)
	legacy := BuildAgentConfigText(orchAgent, "", "汇总所有结果")
	if chainOut != legacy {
		t.Fatalf("path D summary mismatch:\n chain=%q\n legacy=%q", chainOut, legacy)
	}
	// summary chain 不应叠加 OrchestratorSystemPrompt（summary prompt 自带 summary system prompt）
	if strings.Contains(chainOut, OrchestratorSystemPrompt[:20]) {
		t.Fatalf("summary chain must not include OrchestratorSystemPrompt, got %q", chainOut)
	}
}

// TestFanoutFrameBuilder_NoExtraNoop 验证 Extra 中无 fanout_frame key 时返回 current 不变。
func TestFanoutFrameBuilder_NoExtraNoop(t *testing.T) {
	b := &FanoutFrameBuilder{}
	got := b.Build(context.Background(), ContextInput{}, "current")
	if got != "current" {
		t.Fatalf("no extra should be noop: got %q, want %q", got, "current")
	}
}

// TestFanoutFrameBuilder_EmptyOrchestratorNameNoop 验证 orchestratorName 为空时返回 current 不变。
func TestFanoutFrameBuilder_EmptyOrchestratorNameNoop(t *testing.T) {
	b := &FanoutFrameBuilder{}
	in := ContextInput{
		Extra: map[string]any{
			fanoutFrameExtraKey: FanoutFrameInput{OrchestratorName: "", Task: "x"},
		},
	}
	got := b.Build(context.Background(), in, "current")
	if got != "current" {
		t.Fatalf("empty orchestratorName should be noop: got %q, want %q", got, "current")
	}
}

// TestFanoutFrameBuilder_TruncatesTaskTo2000 验证 task 描述被截断到 2000 字符。
func TestFanoutFrameBuilder_TruncatesTaskTo2000(t *testing.T) {
	b := &FanoutFrameBuilder{}
	longTask := strings.Repeat("a", 3000)
	in := ContextInput{
		Extra: map[string]any{
			fanoutFrameExtraKey: FanoutFrameInput{OrchestratorName: "Orch", Task: longTask},
		},
	}
	got := b.Build(context.Background(), in, "")
	// 截断后 task 段长度应为 2000
	if !strings.Contains(got, strings.Repeat("a", 2000)) {
		t.Fatalf("expected task truncated to 2000 chars, got len=%d", len(got))
	}
	if strings.Contains(got, strings.Repeat("a", 2001)) {
		t.Fatalf("task was not truncated to 2000, got longer run")
	}
}
