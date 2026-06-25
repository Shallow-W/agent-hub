package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/port"
	"github.com/agent-hub/backend/pkg/ws"
)

// ErrDaemonNotConnected 表示 daemon 未通过 WS 连接导致派发失败。
//
// P9 引入：DispatchPlan 迁移前 3 处 "daemon not connected" 错误格式不一致
// （dispatcher.go / orchestrator.go 返回 fmt.Errorf 文本，orchestrator_artifact.go
// 返回 ErrArtifactEditNoAgent）。迁移后 Dispatcher 统一返回此 sentinel，
// 调用方（如 runDaemonEdit）通过 errors.Is 映射回业务 sentinel（如 ErrArtifactEditNoAgent）
// 以保留零行为变更。
var ErrDaemonNotConnected = errors.New("daemon not connected")

// DaemonTaskCreator 是 Dispatcher 创建 daemon task 所需的 agent 仓库子集。
//
// P8a 后 OrchestratorService 持有 canonical repository.AgentStore；Dispatcher
// 仍保留此窄接口（仅 CreateDaemonTask 单方法），把「派发」与「查询」职责分离。
// repository.AgentStore 自动满足 DaemonTaskCreator（结构化接口）。
type DaemonTaskCreator interface {
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error)
}

// MessagePersister 是 Dispatcher 落库 assistant 消息所需的 msg 仓库子集。
//
// P8a 后 OrchestratorService 持有 canonical repository.MessageStore；Dispatcher
// 仍保留此窄接口（Create + SaveArtifacts 两方法），隔离 Dispatcher 的最小依赖面。
// repository.MessageStore 自动满足 MessagePersister。
//
// 群聊流式接入（本任务）后 Dispatcher 同时使用 StreamingMsgRepo（来自 streaming_pipeline.go）
// 落 streaming placeholder；MessagePersister 保留给 legacy defaultMessageHandler
// 的 fallback 路径，避免破坏 runDaemonEdit 这种不落 message 的 ResultHandler。
type MessagePersister interface {
	Create(ctx context.Context, conversationID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, senderID *string, mentions []string) (*model.Message, error)
	SaveArtifacts(ctx context.Context, messageID string, artifacts []model.Artifact) error
}

// DispatcherDeps 是 Dispatcher 的依赖集合。
//
// P7 之前 Dispatcher 通过 d.svc 反向依赖 OrchestratorService 来访问这些字段；
// P7 把它们显式提到 DispatcherDeps 中，解掉反向依赖，使得 Dispatcher 可独立测试与构造。
// P8b 把 DaemonHub 从 concrete *ws.DaemonHub 改为 port.DaemonDispatcher 端口接口。
//
// 群聊流式接入（本任务）新增字段：
//   - StreamingPipeline：包含 StreamingMsgRepo / StreamingBuffer / Notifier / ConvRepo。
//     当 StreamingPipeline.MsgRepo 为 nil 时，Dispatcher 回退到非流式行为
//     （Create 而非 CreateStreaming+FinalizeStreaming），保持向后兼容。
//   - TaskAgentIndex：让 *ws.DaemonHub 通过 TaskAgentIndex 接口注入 task→agentName
//     映射；nil 时不注册（仅影响 message.streaming payload 的 agent_name 字段）。
//   - TaskCardQueue：drain MCP subprocess 卡片，与 createAgentReply 一致。可空。
type DispatcherDeps struct {
	AgentRepo    DaemonTaskCreator
	DaemonHub    port.DaemonDispatcher
	MsgRepo      MessagePersister
	UploadDir    string // 用于 artifactsFromMarkdown fallback（目前未直接使用，预留）
	Streaming    *StreamingPipelineDeps
	TaskAgent    TaskAgentIndex
	TaskCardQueue *TaskCardQueue
}

// Dispatcher 封装「为单个 agent 创建 daemon task + WS dispatch + 等待结果 + 落库消息」
// 这一坨派发机制。它对应重构前 orchestrator.go 中的 dispatchWithHooks 私有方法。
//
// 历史路径：dispatchSingleAgent / dispatchOrchWorker / handleOrchestratedDispatch
// 各自直接调 s.dispatchAndWait。P6 把派发动作抽成独立类型并把各路径的业务
// 包装逻辑（权限校验 / OrchTaskCard 生命周期 / CAS guard / summary 落库）拆成 hook：
//   - PreDispatch：daemon task 创建前调用（权限 / CAS / 存在性校验）。返回 error 中止。
//   - OnTaskCreated：daemon task 创建成功后、WS 推送前调用（OrchTaskCard 起点）。
//   - OnMessagePersisted：ResultHandler 返回非 nil message 后调用（OrchTaskCard 完结 / summary 触发）。
//   - OnFailed：派发失败时调用（daemon 未连接 / fn error 等）。
//
// P7 进一步：Dispatcher 不再持有 *OrchestratorService 反向引用，所有依赖通过
// DispatcherDeps 注入；dispatchWithHooks 的核心实现迁到 Dispatcher.dispatchCore。
//
// P9 进一步：Dispatch 拆成两层：
//   - DispatchPlan：核心入口，接受 DispatchPlan（Input + PromptBuilder + ResultHandler），
//     让调用方可以注入「prompt 升级」与「task 产物处理」策略。两条原本内联的路径
//     （handleOrchestratedDispatch / runDaemonEdit）改走 DispatchPlan 直接注入自定义
//     PromptBuilder / ResultHandler，消除 3 处重复的 WSMessage 拼装 + 错误码分歧。
//   - Dispatch：DispatchPlan 的薄壳，用 defaultPromptBuilder + defaultMessageHandler
//     装配，等价于 P9 之前的行为（worker / summary / 直接 reply 仍走这条）。
//
// 注意：Dispatcher 不负责上下文构建（用 chain）也不负责并发护栏（用 AgentQueue）。
// 它只做「拿到 prompt + contextMessages 后把任务送到 daemon 并等待结果」这一件事。
// 并发护栏由调用方在调用 Dispatcher.Dispatch / DispatchPlan 前/后用 AgentQueue.Run 包裹。
type Dispatcher struct {
	deps   DispatcherDeps
	handles sync.Map // taskID -> *StreamingHandle（in-flight streaming placeholder bookkeeping）
}

// NewDispatcher 构造一个独立 Dispatcher，依赖通过 deps 显式注入。
//
// P7 前：NewDispatcher(svc *OrchestratorService)，Dispatcher 反向依赖 svc。
// P7 后：调用方负责组装 DispatcherDeps；OrchestratorService 在构造时把自身字段
// 装入 DispatcherDeps 后传给 NewDispatcher。
func NewDispatcher(deps DispatcherDeps) *Dispatcher {
	return &Dispatcher{deps: deps}
}

// SetStreamingDeps 在 OrchestratorService 默认装配完 Dispatcher 后由 main 调用，
// 注入流式管线共享依赖（与 MessageService 共用同一组 msgRepo / daemonHub / buffer /
// notifier / convRepo / taskCardQueue）。让群聊 worker dispatch 走与单聊 createAgentReply
// 一致的流式管线。
//
// taskAgentIndex 一般传 *ws.DaemonHub（满足 TaskAgentIndex 接口）；nil 时跳过
// task→agentName 映射注册（仅影响 message.streaming payload 的 agent_name 字段）。
func (d *Dispatcher) SetStreamingDeps(streaming StreamingPipelineDeps, taskAgentIndex TaskAgentIndex, taskCardQueue *TaskCardQueue) {
	d.deps.Streaming = &streaming
	d.deps.TaskAgent = taskAgentIndex
	d.deps.TaskCardQueue = taskCardQueue
}

// DispatchInput 是 Dispatcher.Dispatch 的输入。
type DispatchInput struct {
	ConvID          string
	UserID          string
	Agent           *model.Agent
	Prompt          string  // 任务指令（daemon 发给 agent 的 prompt 字段）
	ContextMessages string  // 上下文（context_messages 字段）
	ReplyTo         *string // 关联的消息 ID（reply_to 字段）
}

// DispatchResult 是 Dispatcher.Dispatch 的输出。
type DispatchResult struct {
	Message *model.Message // 落库的 assistant 消息
}

// DispatchHooks 是 Dispatch 的可选业务包装回调。
// 所有字段都是可选（nil-safe）；零值 DispatchHooks（全 nil）等价于
// 直接调用 dispatchCore，不引入任何额外行为。
//
// 调用顺序（与重构前 dispatchWithHooks 内部时序对齐）：
//
//	PreDispatch(input)                       ← Dispatch 入口（创建 daemon task 之前）
//	  ↳ error → 直接 return，不调用后续 hook
//	CreateDaemonTask
//	OnTaskCreated(task)                      ← daemon task 创建成功后、WS 推送前
//	IsConnected / RegisterTaskPromise / SendToMachine / waitDaemonTask / ResultHandler
//	OnMessagePersisted(msg)                  ← ResultHandler 返回非 nil msg 后
//	  ↳ 任一中间步骤 error → OnFailed(input, err)
type DispatchHooks struct {
	// PreDispatch 在 daemon task 创建前调用，用于权限校验、CAS guard、存在性检查。
	// 返回非 nil error 立即中止派发，并直接透传给 Dispatch 调用方。
	PreDispatch func(ctx context.Context, input DispatchInput) error

	// OnTaskCreated 在 daemon task 创建成功后、WS 推送前调用。
	// 用于 OrchTaskCard 生命周期起点（创建 todo 卡片）。
	OnTaskCreated func(ctx context.Context, task *model.DaemonTask)

	// OnMessagePersisted 在 ResultHandler 返回非 nil message 后调用。
	// 用于 OrchTaskCard 完结、summary 落库、postPersistAsync 等后续触发。
	//
	// P9 之前此 hook 名为 OnMessagePersisted，含义是「Dispatcher 内部落了 message 后」。
	// P9 后 DispatchPlan 把「落 message」动作委托给 ResultHandler（可被替换为不落 message
	// 的实现，例如 runDaemonEdit），但 hook 的语义保持不变：只要 ResultHandler 返回了
	// 非 nil msg，OnMessagePersisted 就会被调用。
	OnMessagePersisted func(ctx context.Context, msg *model.Message)

	// OnFailed 在派发失败时调用（daemon 未连接 / WS 失败 / 等待失败 / fn error 等）。
	// 用于 OrchTaskCard 标记失败 / OrchTask 状态回退。
	OnFailed func(ctx context.Context, input DispatchInput, err error)
}

// DispatchPlan 描述一次派发的「输入」和「产物处理策略」。
//
// P9 引入：Dispatcher 之前的契约是「创建 daemon task → WS 推 → 等 → 落 assistant
// message」一条龙，但 handleOrchestratedDispatch 与 runDaemonEdit 两条内联路径
// 无法套用：
//   - handleOrchestratedDispatch：daemon 返回后不立刻落 message，先
//     ParseOrchestratorOutputForAgents 决定 fanout 还是直答；落 message 时
//     artifacts 来自 agentMetadata(orchAgent) 而非默认的 agentMetadata(input.Agent)。
//   - runDaemonEdit：daemon 返回后完全不落 message，只返回结果文本给调用方
//     做 CreateVersion。
//
// DispatchPlan 把「prompt 升级」与「task 产物处理」从核心派发流程中剥离成两个
// 可注入的回调，使得这两条路径也能走 Dispatcher.DispatchPlan 享受 DispatchHooks
// （生命周期 / metric / 限流）。
//
// 零值策略（PromptBuilder / ResultHandler 为 nil）由 defaultPromptBuilder /
// defaultMessageHandler 兜底，等价于 P9 之前 Dispatch 的行为。
type DispatchPlan struct {
	// Input 是派发的输入（convID / userID / agent / prompt / contextMessages / replyTo）。
	Input DispatchInput

	// PromptBuilder 把 input 升级成最终 prompt。
	// nil 时使用 defaultPromptBuilder（直接返回 input.Prompt）。
	//
	// 适合 handleOrchestratedDispatch 这种「调用方在 closure 里构建好 fullPrompt，
	// 再注入到 plan.Input.Prompt」的场景（此时默认 builder 即可）。
	// 也适合需要在 task 创建前对 prompt 做最终变换（如注入动态参数）的场景。
	PromptBuilder func(ctx context.Context, in DispatchInput) (string, error)

	// ResultHandler 处理 daemon 返回的 task；返回 (msg, error)。
	//   - (msg, nil)：落 msg，并触发 OnMessagePersisted（worker / orch 直答走这条）
	//   - (nil, nil)：不落 msg（runDaemonEdit 走这条，调用方从 closure 读 task 副本）
	//   - (_, err)：视为失败，触发 OnFailed（与 daemon 派发失败等价）
	//
	// nil 时使用 defaultMessageHandler：CreateMessage(agentMetadata(input.Agent)) +
	// persistArtifacts(task.Artifacts)，与 P9 之前 dispatchCore 行为完全等价。
	ResultHandler func(ctx context.Context, task *model.DaemonTask) (*model.Message, error)

	// StreamingRequested 控制 dispatchPlanCore 是否预创建 streaming placeholder
	// 并把 message_id 注入 dispatch payload。
	//
	// 群聊流式接入（本任务）引入。语义：
	//   - true：调用方走 streaming 管线（预创建 placeholder + FinalizeStreaming 切终态）。
	//     Dispatch() 薄壳默认置 true（worker / 群聊 @mention 走这条，与单聊 createAgentReply
	//     行为对齐）。
	//   - false（零值）：跳过 streaming。runDaemonEdit（不落 message）/ handleOrchestratedDispatch
	//     （自己落 dispatch message）走这条——这些调用方明确处理产物，创建 placeholder 反而会孤立。
	//
	// 仍要求 DispatcherDeps.Streaming.MsgRepo 非 nil（main.go SetStreamingDeps 注入）。
	// 若 StreamingDeps 未注入，本字段被忽略（兼容旧测试装配）。
	StreamingRequested bool
}

// DispatchPlanResult 是 DispatchPlan 的输出。
//
// Task 总是非 nil（DispatchPlan 必然创建了 daemon task，调用方可读 daemon raw
// output / artifacts，例如 runDaemonEdit 从 task.Artifacts 提取 code 产物）。
// Message 可为 nil（ResultHandler 返回 nil 时，例如 runDaemonEdit 不落 message）。
type DispatchPlanResult struct {
	Task    *model.DaemonTask
	Message *model.Message
}

// defaultPromptBuilder 是 DispatchPlan.PromptBuilder 的默认实现：直接返回 input.Prompt。
//
// P9 引入：让「调用方在 closure 里构建好 prompt 后注入到 input.Prompt」的场景
// （如 handleOrchestratedDispatch 把 fullPrompt 写入 input.Prompt）可以零配置走
// DispatchPlan。
func defaultPromptBuilder(_ context.Context, in DispatchInput) (string, error) {
	return in.Prompt, nil
}

// defaultMessageHandler 是 DispatchPlan.ResultHandler 的默认实现。
//
// 群聊流式接入（本任务）：
//   - 若 dispatcher 装配了 StreamingPipeline（d.deps.Streaming 非 nil 且 MsgRepo 满足
//     StreamingMsgRepo），用 FinalizeStreamingPipeline 切 streaming placeholder 到 complete，
//     写 blocks_json / cards / artifacts。与 createAgentReply 1322-1387 行为一致。
//   - 否则回退到 P9 之前行为：Create + persistArtifacts。保留以兼容 streaming 未注入的
//     构造（如 runDaemonEdit / 单元测试）。
func (d *Dispatcher) defaultMessageHandler(ctx context.Context, in DispatchInput, task *model.DaemonTask) (*model.Message, error) {
	if handle, ok := d.streamingHandleForTask(task.ID); ok {
		return d.finalizeStreamingSuccess(ctx, *handle, in, task)
	}
	// fallback：legacy Create + persistArtifacts
	artifacts := agentMetadata(in.Agent)
	msg, err := d.deps.MsgRepo.Create(ctx, in.ConvID, "assistant", task.Result, artifacts, nil, in.ReplyTo, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create agent reply: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "agent.message_created", "conversation_id", in.ConvID, "agent_id", in.Agent.ID, "agent_name", in.Agent.Name, "message_id", msg.ID, "reply_to", stringValue(in.ReplyTo), "content_len", len(msg.Content))
	d.persistArtifacts(ctx, msg, task.Artifacts)
	return msg, nil
}

// streamingHandleForTask 从 dispatcher 内部 in-flight handle 表查询 taskID 对应的
// StreamingHandle。handle 由 dispatchPlanCore 在 setup 时存入，finalize 后清除。
// 第二返回值 false 表示该 task 走 legacy Create 路径（无 streaming）。
func (d *Dispatcher) streamingHandleForTask(taskID string) (*StreamingHandle, bool) {
	if taskID == "" {
		return nil, false
	}
	v, ok := d.handles.Load(taskID)
	if !ok {
		return nil, false
	}
	return v.(*StreamingHandle), true
}

// finalizeStreamingSuccess 在 task 成功完成时调用：
//  - 抽取 agent 写在正文里的 cards
//  - drain MCP subprocess 卡片队列
//  - snapshot streamingBuffer 累积的 blocks_json
//  - FinalizeStreamingPipeline 写 status=complete + content + blocks_json + cards + artifacts
//  - 清理 in-flight handle
func (d *Dispatcher) finalizeStreamingSuccess(ctx context.Context, handle StreamingHandle, in DispatchInput, task *model.DaemonTask) (*model.Message, error) {
	pipelineDeps := d.streamingDeps()
	defer d.handles.Delete(handle.TaskID)

	agentCards, strippedContent, _ := extractCardsFromContent(task.Result)
	allCards := append([]map[string]any{}, ValidateCards(nil)...)
	if d.deps.TaskCardQueue != nil {
		if subprocessCards := d.deps.TaskCardQueue.Drain(handle.TaskID); len(subprocessCards) > 0 {
			allCards = append(allCards, ValidateCards(subprocessCards)...)
		}
	}
	allCards = append(allCards, ValidateCards(agentCards)...)

	var blocksJSON string
	if pipelineDeps.StreamingBuffer != nil {
		// pipelineDeps.StreamingBuffer 是 *StreamingBuffer；通过类型断言拿 snapshotBlocksJSON
		if buf, ok := pipelineDeps.StreamingBuffer.(*StreamingBuffer); ok {
			blocksJSON = snapshotBlocksJSONFromBuffer(buf, handle.TaskID)
		}
	}

	msg, err := FinalizeStreamingPipeline(ctx, pipelineDeps, &handle, FinalizeStreamingPipelineOptions{
		Status:        model.MessageStatusComplete,
		Content:       strippedContent,
		BlocksJSON:    blocksJSON,
		ArtifactsJSON: handle.ArtifactsJSON,
		Cards:         allCards,
		Artifacts:     mergeArtifactsForStreaming(task.Artifacts, strippedContent),
	})
	if err != nil {
		return nil, fmt.Errorf("finalize streaming message: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "agent.message_created", "conversation_id", in.ConvID, "agent_id", in.Agent.ID, "agent_name", in.Agent.Name, "message_id", msg.ID, "reply_to", stringValue(in.ReplyTo), "content_len", len(msg.Content))
	return msg, nil
}

// streamingDeps 返回当前 dispatcher 装配的 StreamingPipelineDeps（零值时所有字段为 nil，
// 调用方需各自做 nil 防护）。
func (d *Dispatcher) streamingDeps() StreamingPipelineDeps {
	if d.deps.Streaming == nil {
		return StreamingPipelineDeps{}
	}
	return *d.deps.Streaming
}

// snapshotBlocksJSONFromBuffer 从 *StreamingBuffer 取 taskID 累积的 blocks JSON。
// 单聊（createAgentReply）与群聊（Dispatcher）共用此包级函数。
//
// 切分：取出 blocks 后调用 SplitTextBlocksByCardFences，把 text block 里的
// ```agenthub {"cards":[...]}``` fenced block 提升为独立 card kind block。这样
// 卡片成为 first-class block（与 text/thinking 平级），前端 BlockRegistry 直接渲染，
// 无需依赖 content placeholder + cards_json 的双表示路径。
func snapshotBlocksJSONFromBuffer(buf *StreamingBuffer, taskID string) string {
	if buf == nil || taskID == "" {
		return ""
	}
	state, ok := buf.GetState(taskID)
	if !ok || len(state.Blocks) == 0 {
		return ""
	}
	// 切分 text block 里的 fenced card blocks，让卡片成为 first-class block。
	splitBlocks := SplitTextBlocksByCardFences(state.Blocks)
	b, err := json.Marshal(splitBlocks)
	if err != nil {
		return ""
	}
	return string(b)
}

// mergeArtifactsForStreaming 是 dispatcher 流式成功路径的 artifact 合并函数。
// 与 createAgentReply 的 artifactsFromTaskResultOrMarkdown 对齐，但 task.Artifacts
// 已是 []model.Artifact（dispatcher 的 DaemonTask 在 waitDaemonTask 中由
// artifactsFromTaskResult 转换过），所以只做 markdown fallback——空时从正文抽取。
func mergeArtifactsForStreaming(arts []model.Artifact, content string) []model.Artifact {
	if len(arts) > 0 {
		if !hasCodeArtifact(arts) {
			arts = append(arts, codeArtifactsFromMarkdown(content)...)
		}
		return arts
	}
	return artifactsFromMarkdown(content)
}

// Dispatch 把一个任务送到 daemon 并等待结果，落库为 assistant 消息后返回。
//
// P9 后 Dispatch 是 DispatchPlan 的薄壳：用 defaultPromptBuilder + defaultMessageHandler
// 装配 DispatchPlan，零行为变更。两条内联路径（handleOrchestratedDispatch /
// runDaemonEdit）改走 DispatchPlan 直接注入自定义 PromptBuilder / ResultHandler。
//
// hooks 零值（全 nil）时与原 dispatchAndWait 行为完全一致；非 nil 字段会在
// 对应时序点被调用（详见 DispatchHooks 文档）。每个字段调用前都做 nil 检查。
//
// 错误语义（与原 dispatchWithHooks 完全一致）：
//   - PreDispatch 返回 error → 直接透传，不进入派发流程
//   - daemon task 创建失败 → fmt.Errorf("create daemon task: %w", ...)
//   - daemon 未连接 → fmt.Errorf("agent %q 的 daemon 未通过 WS 连接: %w", agent.Name, ErrDaemonNotConnected)
//   - WS 发送失败 → fmt.Errorf("dispatch to daemon: %w", ...)
//   - 等待失败 / 超时 → 原错误透传
//   - task.Status == "failed" → fmt.Errorf("daemon task failed: %s", task.Error)
//   - 消息落库失败 → fmt.Errorf("create agent reply: %w", ...)
//
// 任一错误路径上 OnFailed 都会被调用一次（若非 nil），便于调用方统一处理失败。
func (d *Dispatcher) Dispatch(ctx context.Context, in DispatchInput, hooks DispatchHooks) (*DispatchResult, error) {
	plan := DispatchPlan{
		Input:               in,
		PromptBuilder:       defaultPromptBuilder,
		StreamingRequested:  true, // worker / 群聊 @mention 路径：走 streaming 管线（与 createAgentReply 对齐）
		ResultHandler: func(ctx context.Context, task *model.DaemonTask) (*model.Message, error) {
			return d.defaultMessageHandler(ctx, in, task)
		},
	}
	res, err := d.DispatchPlan(ctx, plan, hooks)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return &DispatchResult{Message: res.Message}, nil
}

// DispatchPlan 把一个 DispatchPlan 送到 daemon 并等待结果，按 plan.ResultHandler
// 处理产物（落 message / 不落 message / 触发 fanout）。
//
// 时序（与重构前 dispatchCore 完全对齐，仅 PromptBuilder / ResultHandler 可被替换）：
//
//	PreDispatch(plan.Input)                  ← hooks.PreDispatch 非 nil 时调用
//	  ↳ error → OnFailed(input, err)，直接 return
//	prompt := plan.PromptBuilder(plan.Input) ← 默认 defaultPromptBuilder 直接返回 input.Prompt
//	CreateDaemonTask(prompt)
//	  ↳ err → OnFailed(input, "create daemon task")，return
//	OnTaskCreated(task)                      ← hooks.OnTaskCreated 非 nil 时调用
//	IsConnected / RegisterTaskPromise / SendToMachine / waitDaemonTask
//	  ↳ 任一 err → OnFailed(input, err)，return
//	msg := plan.ResultHandler(task)          ← 默认 defaultMessageHandler 落 message
//	  ↳ err → OnFailed(input, err)，return
//	OnMessagePersisted(msg)                  ← msg 非 nil 且 hooks.OnMessagePersisted 非 nil 时调用
//
// 返回 *DispatchPlanResult 总是携带 task（调用方可读 daemon raw output）；
// Message 在 ResultHandler 返回 nil 时为 nil（如 runDaemonEdit 不落 message）。
func (d *Dispatcher) DispatchPlan(ctx context.Context, plan DispatchPlan, hooks DispatchHooks) (*DispatchPlanResult, error) {
	in := plan.Input

	if hooks.PreDispatch != nil {
		if err := hooks.PreDispatch(ctx, in); err != nil {
			if hooks.OnFailed != nil {
				hooks.OnFailed(ctx, in, err)
			}
			return nil, err
		}
	}

	promptBuilder := plan.PromptBuilder
	if promptBuilder == nil {
		promptBuilder = defaultPromptBuilder
	}
	resultHandler := plan.ResultHandler

	task, msg, err := d.dispatchPlanCore(ctx, in, promptBuilder, resultHandler, hooks.OnTaskCreated, plan.StreamingRequested)
	if err != nil {
		if hooks.OnFailed != nil {
			hooks.OnFailed(ctx, in, err)
		}
		return nil, err
	}

	// msg 可能为 nil（ResultHandler 返回 nil，例如 runDaemonEdit）。
	// 仅当 msg 非 nil 时触发 OnMessagePersisted。
	if hooks.OnMessagePersisted != nil && msg != nil {
		hooks.OnMessagePersisted(ctx, msg)
	}
	return &DispatchPlanResult{Task: task, Message: msg}, nil
}

// dispatchCore 是 dispatchWithHooks 的核心实现迁移：CreateDaemonTask → onTaskCreated →
// IsConnected/RegisterTaskPromise/SendToMachine → waitDaemonTask → ResultHandler。
//
// P9 后核心实现支持 PromptBuilder / ResultHandler 注入；task 与 message 分别
// 通过返回值传出（避免污染 model.DaemonTask 领域模型）。
//
// 与原 OrchestratorService.dispatchWithHooks 时序完全一致（零行为变更）：
//
//	CreateDaemonTask(promptFromBuilder)
//	  ↳ err → return ("create daemon task")
//	onTaskCreated(task)               ← 若回调非 nil
//	IsConnected / RegisterTaskPromise / SendToMachine / waitDaemonTask / ResultHandler
//
// onTaskCreated 为 nil 时与原 dispatchWithHooks 行为完全等价。
//
// resultHandler 为 nil 时使用 defaultMessageHandler（落 message + persistArtifacts）。
func (d *Dispatcher) dispatchPlanCore(
	ctx context.Context,
	in DispatchInput,
	promptBuilder func(context.Context, DispatchInput) (string, error),
	resultHandler func(context.Context, *model.DaemonTask) (*model.Message, error),
	onTaskCreated func(context.Context, *model.DaemonTask),
	streamingRequested bool,
) (*model.DaemonTask, *model.Message, error) {
	convID, userID := in.ConvID, in.UserID
	agent, contextMessages, replyTo := in.Agent, in.ContextMessages, in.ReplyTo

	prompt := in.Prompt
	if promptBuilder != nil {
		p, err := promptBuilder(ctx, in)
		if err != nil {
			return nil, nil, fmt.Errorf("build dispatch prompt: %w", err)
		}
		prompt = p
	}

	task, err := d.deps.AgentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, prompt, contextMessages)
	if err != nil {
		return nil, nil, fmt.Errorf("create daemon task: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "agent.dispatch_task_created", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "machine_id", *agent.MachineID, "cli_tool", agent.CLITool, "prompt_len", len(prompt), "context_len", len(contextMessages), "reply_to", stringValue(replyTo), "prompt_preview", orchPreview(prompt))

	// OrchTaskCard 起点回调：拿到 daemon task ID 后立即登记（WS 推送前）。
	if onTaskCreated != nil {
		onTaskCreated(ctx, task)
	}

	// 群聊流式接入（本任务）：预创建 streaming placeholder + 注册 task 映射。
	// 流式仅在「调用方显式声明走 streaming 路径」时启用：
	//   - Dispatch() 薄壳把 StreamingRequested 置 true → 启用流式（worker / 群聊 @mention 走这条）
	//   - DispatchPlan() 注入自定义 ResultHandler 的调用方（runDaemonEdit /
	//     handleOrchestratedDispatch）默认 StreamingRequested=false → 不启用流式，
	//     避免创建孤立的 streaming placeholder（无人 FinalizeStreaming → 仅 watchdog 兜底），
	//     且与调用方自己的产物落库形成双消息。
	//
	// 同时要求 StreamingDeps.MsgRepo 非 nil（main.go 通过 SetStreamingDeps 注入）。
	// 测试若直接构造 DispatcherDeps{} 不注入 Streaming，也保持向后兼容（无流式）。
	pipelineDeps := d.streamingDeps()
	streamingEnabled := pipelineDeps.MsgRepo != nil && streamingRequested
	var handle *StreamingHandle
	if streamingEnabled {
		handle, err = SetupStreamingPipeline(ctx, pipelineDeps, convID, agent.ID, agent.Name, agent.CLITool, task.ID, replyTo, d.deps.TaskAgent)
		if err != nil {
			return task, nil, err
		}
		d.handles.Store(task.ID, handle)
		// defer 清理（对齐 createAgentReply:1269-1279）
		defer d.handles.Delete(task.ID)
		if d.deps.DaemonHub != nil {
			defer d.deps.DaemonHub.RemoveTaskPromise(task.ID)
			defer d.deps.DaemonHub.DeleteTaskMessage(task.ID)
		}
		if d.deps.TaskAgent != nil {
			defer d.deps.TaskAgent.DeleteTaskAgent(task.ID)
		}
		if pipelineDeps.StreamingBuffer != nil {
			if buf, ok := pipelineDeps.StreamingBuffer.(*StreamingBuffer); ok {
				defer buf.Delete(task.ID)
			}
		}
	}

	finalizeErr := func(status string) {
		if !streamingEnabled || handle == nil {
			return
		}
		if _, ferr := FinalizeStreamingPipeline(ctx, pipelineDeps, handle, FinalizeStreamingPipelineOptions{Status: status}); ferr != nil {
			slog.Warn("dispatcher: finalize streaming on error failed", "message_id", handle.MessageID, "error", ferr)
		}
		BroadcastStreamingTerminal(pipelineDeps, handle, status)
	}

	if d.deps.DaemonHub == nil || !d.deps.DaemonHub.IsConnected(*agent.MachineID) {
		slog.Warn(orchFlowLog, "stage", "agent.daemon_not_connected", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "machine_id", *agent.MachineID)
		finalizeErr(model.MessageStatusError)
		return task, nil, fmt.Errorf("agent %q 的 daemon 未通过 WS 连接: %w", agent.Name, ErrDaemonNotConnected)
	}
	d.deps.DaemonHub.RegisterTaskPromise(task.ID)
	dispatchData := map[string]interface{}{
		"task_id":          task.ID,
		"cli_tool":         agent.CLITool,
		"prompt":           prompt,
		"context_messages": contextMessages,
		"agent_id":         agent.ID,
		"conversation_id":  convID,
		"user_id":          userID,
	}
	if handle != nil {
		dispatchData["message_id"] = handle.MessageID
	}
	if err := d.deps.DaemonHub.SendToMachine(*agent.MachineID, ws.WSMessage{
		Type: "task.dispatch",
		Data: dispatchData,
	}); err != nil {
		finalizeErr(model.MessageStatusError)
		return task, nil, fmt.Errorf("dispatch to daemon: %w", err)
	}
	slog.Info(orchFlowLog, "stage", "agent.dispatch_sent", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID)

	preWaitTask := task
	task, err = d.waitDaemonTask(ctx, task.ID)
	if err != nil {
		slog.Warn(orchFlowLog, "stage", "agent.dispatch_wait_failed", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", preWaitTask.ID, "error", err)
		finalizeErr(model.MessageStatusError)
		// 注意：waitDaemonTask 出错时返回的 task 可能为 nil（IsConnected 返回 false
		// / channel nil 等场景）。这里取原 task（preWaitTask，预 CreateDaemonTask 的实例）
		// 避免 nil 解引用；调用方读 task 时容忍无 Result/Artifacts 字段（与重构前行为一致）。
		if task == nil {
			task = preWaitTask
		}
		return task, nil, err
	}
	slog.Info(orchFlowLog, "stage", "agent.dispatch_completed", "conversation_id", convID, "agent_id", agent.ID, "agent_name", agent.Name, "daemon_task_id", task.ID, "status", task.Status, "result_len", len(task.Result), "artifact_count", len(task.Artifacts), "result_preview", orchPreview(task.Result))
	if task.Status == "failed" {
		finalizeErr(model.MessageStatusError)
		return task, nil, fmt.Errorf("daemon task failed: %s", task.Error)
	}

	// ResultHandler 注入点：调用方可自定义如何处理 task。
	// nil → defaultMessageHandler：落 message with agentMetadata(agent)。
	if resultHandler == nil {
		msg, err := d.defaultMessageHandler(ctx, in, task)
		if err != nil {
			return task, nil, err
		}
		return task, msg, nil
	}

	msg, err := resultHandler(ctx, task)
	if err != nil {
		return task, nil, err
	}
	return task, msg, nil
}

// waitDaemonTask 通过 DaemonHub 等待 daemon task 完成（channel-based 通知）。
//
// 迁移自 OrchestratorService.waitDaemonTask（零行为变更）：
//   - DaemonHub 为 nil → fmt.Errorf("daemon hub not available")
//   - AwaitTaskResult 返回 nil → fmt.Errorf("daemon not connected for task %s", taskID)
//   - 400s 超时 → ErrMsgAgentTimeout
func (d *Dispatcher) waitDaemonTask(ctx context.Context, taskID string) (*model.DaemonTask, error) {
	if d.deps.DaemonHub == nil {
		return nil, fmt.Errorf("daemon hub not available")
	}

	ch := d.deps.DaemonHub.AwaitTaskResult(taskID)
	if ch == nil {
		return nil, fmt.Errorf("daemon not connected for task %s", taskID)
	}
	defer d.deps.DaemonHub.RemoveTaskPromise(taskID)

	ctx, cancel := context.WithTimeout(ctx, 400*time.Second)
	defer cancel()

	select {
	case result := <-ch:
		task := &model.DaemonTask{
			ID:        result.TaskID,
			Status:    "completed",
			Result:    result.Result,
			Artifacts: artifactsFromTaskResult(result.Artifacts),
		}
		if result.Error != "" {
			task.Status = "failed"
			task.Error = result.Error
		}
		return task, nil
	case <-ctx.Done():
		return nil, ErrMsgAgentTimeout
	}
}

// persistArtifacts 把 daemon 返回的 artifacts（或从 markdown 抽取）落库并回填 msg.Artifacts。
//
// 迁移自 OrchestratorService.persistArtifacts（零行为变更）。
func (d *Dispatcher) persistArtifacts(ctx context.Context, msg *model.Message, artifacts []model.Artifact) {
	if msg == nil {
		return
	}
	if len(artifacts) == 0 {
		artifacts = artifactsFromMarkdown(msg.Content)
	} else if !hasCodeArtifact(artifacts) {
		artifacts = append(artifacts, codeArtifactsFromMarkdown(msg.Content)...)
	}
	if len(artifacts) == 0 {
		return
	}
	if err := d.deps.MsgRepo.SaveArtifacts(ctx, msg.ID, artifacts); err != nil {
		slog.Warn("save orchestrator artifacts failed", "message_id", msg.ID, "error", err)
		return
	}
	msg.Artifacts = artifacts
}

// DispatchMany 把同一份输入并发派发给多个 agent（fan-out）。
// 当前实现是简单的并发循环；调用方若需要串行化同一 agent 的派发，
// 应在调用前用 AgentQueue.Run 包裹。
//
// 返回的 results 与 agents 一一对应；任一 agent 派发失败时对应位置的 error 非 nil，
// 但不会中断其他 agent 的派发（fan-out 语义）。hooks 对每个 input 都生效。
func (d *Dispatcher) DispatchMany(ctx context.Context, inputs []DispatchInput, hooks DispatchHooks) ([]*DispatchResult, []error) {
	results := make([]*DispatchResult, len(inputs))
	errs := make([]error, len(inputs))
	if len(inputs) == 0 {
		return results, errs
	}

	// 并发 fan-out：每个 agent 独立 goroutine
	type outcome struct {
		idx int
		res *DispatchResult
		err error
	}
	ch := make(chan outcome, len(inputs))
	for i, in := range inputs {
		go func(idx int, din DispatchInput) {
			res, err := d.Dispatch(ctx, din, hooks)
			ch <- outcome{idx: idx, res: res, err: err}
		}(i, in)
	}

	for i := 0; i < len(inputs); i++ {
		o := <-ch
		results[o.idx] = o.res
		errs[o.idx] = o.err
	}
	return results, errs
}

// FormatDispatchError 统一格式化派发错误，便于上层 slog。
// 当前仅 fmt.Errorf 包装，保留作为未来统一错误码的扩展点。
func FormatDispatchError(agentName string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("dispatch to %q failed: %w", agentName, err)
}
