package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/agent-hub/backend/internal/model"
	"github.com/agent-hub/backend/internal/port"
	"github.com/agent-hub/backend/pkg/ws"
	"github.com/google/uuid"
)

const maxMessageLen = 10000 // 10KB
const maxBlackboardManualContextLen = 8000

// agentReplyInFlight 防止同一源消息触发重复 agent dispatch。
// key = replyTo (source message ID)。
var agentReplyInFlight sync.Map

const recallTimeLimit = 2 * time.Minute // 消息撤回时间限制

// MessageNotifier 消息推送接口（由 Hub 实现）
type MessageNotifier interface {
	PushToConversation(conversationID string, memberIDs []string, message interface{})
	PushCustomEvent(conversationID string, memberIDs []string, eventType string, data interface{})
	IsOnline(userID string) bool
}

// MessageDeliveryState stores transient delivery state outside the source-of-truth message DB.
// History reads must not use this interface; it is only for offline queues and unread counts.
type MessageDeliveryState interface {
	EnqueueOffline(ctx context.Context, userID, conversationID string, msg *model.Message) error
	DequeueOfflineAfter(ctx context.Context, userID, conversationID string, after interface{}) ([]model.Message, error)
	ClearUnread(ctx context.Context, userID, conversationID string) error
	IncrementUnread(ctx context.Context, userID, conversationID string) error
}

// MsgRepo 消息服务所需的仓库接口。
// Deprecated: migrate to repository.MessageStore for canonical interface.
type MsgRepo interface {
	Create(ctx context.Context, conversationID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, senderID *string, mentions []string) (*model.Message, error)
	ListByConversation(ctx context.Context, conversationID string, before interface{}, limit int) ([]model.Message, error)
	MarkConversationRead(ctx context.Context, conversationID, userID string) error
	GetMessagesAfter(ctx context.Context, conversationID string, afterTime interface{}, limit int) ([]model.Message, error)
	GetByID(ctx context.Context, id string) (*model.Message, error)
	GetMessageSender(ctx context.Context, messageID string) (string, error)
	SearchByContent(ctx context.Context, conversationID, keyword string, limit int) ([]model.Message, error)
	SoftDelete(ctx context.Context, messageID string) error
	SaveArtifacts(ctx context.Context, messageID string, artifacts []model.Artifact) error
	UpdateMessageCards(ctx context.Context, messageID, cardsJSON string) error
	PinMessage(ctx context.Context, conversationID, messageID, userID string) (*model.MessagePin, error)
	UnpinMessage(ctx context.Context, conversationID, messageID string) error
	ListPinnedMessages(ctx context.Context, conversationID string, limit int) ([]model.PinnedMessage, error)
	GetConversationBlackboard(ctx context.Context, conversationID string) (*model.ConversationBlackboard, error)
	UpsertConversationBlackboard(ctx context.Context, conversationID, manualContext, userID string) (*model.ConversationBlackboard, error)
	ListReplies(ctx context.Context, messageID string) ([]model.Message, error)
	HideMessage(ctx context.Context, userID, messageID string) error
	UnhideMessage(ctx context.Context, userID, messageID string) error
	GetHiddenMessageIDs(ctx context.Context, userID, conversationID string) (map[string]bool, error)
	CreateStreaming(ctx context.Context, conversationID, role string, senderID *string, replyTo *string, artifactsJSON string) (*model.Message, error)
	FinalizeStreaming(ctx context.Context, messageID, status, content, blocksJSON, artifactsJSON string) error
	ListStreaming(ctx context.Context) ([]model.Message, error)
	MarkStaleStreaming(ctx context.Context, maxAge time.Duration) (int, error)
}

// artifactsFromTaskResult 将 daemon 上行的产物转换为 model.Artifact。
func artifactsFromTaskResult(results []ws.ArtifactResult) []model.Artifact {
	if len(results) == 0 {
		return nil
	}
	out := make([]model.Artifact, 0, len(results))
	for _, r := range results {
		if r.Type == "" {
			continue
		}
		out = append(out, model.Artifact{
			Version:  1,
			Type:     r.Type,
			Language: r.Language,
			Filename: r.Filename,
			Title:    r.Title,
			URL:      r.URL,
			Content:  r.Content,
		})
	}
	return out
}

func artifactsFromTaskResultOrMarkdown(results []ws.ArtifactResult, content string) []model.Artifact {
	if arts := artifactsFromTaskResult(results); len(arts) > 0 {
		return arts
	}
	return artifactsFromMarkdown(content)
}

func hasCodeArtifact(artifacts []model.Artifact) bool {
	for _, a := range artifacts {
		if a.Type == "code" {
			return true
		}
	}
	return false
}

// extractCardsFromContent 从 agent 回复文本里提取所有 ```json {"cards":[...]} ``` fenced block，
// 合并所有 block 的 cards 为单一数组。同时生成 strippedContent：每个 block 替换为 N 个
// `[CARD:<id>]` 占位符（N = block 内卡数，按数组顺序），保留卡片在正文中的位置。
// 找不到 id 字段的卡用自动 UUID。
//
// 行为细则：
//   - 只识别 fenced block（```json 或 ```JSON 开启，``` 闭合），不识别整段 content 为 JSON
//     （避免 agent 整段 JSON 回复被误解析）
//   - 多个 fenced block 全部识别并合并
//   - block 内 JSON 解析失败：静默丢弃该 block，原文中该 block（含 fence 行）原样保留
//   - block 无 cards 字段或 cards 不是数组：静默丢弃，原文中该 block 原样保留
func extractCardsFromContent(content string) (cards []map[string]any, strippedContent string, cardsJSON string) {
	lines := strings.Split(content, "\n")

	// 第一遍：识别所有有效 block 的 (startLine, endLine, cards) 映射，便于第二遍精确替换。
	type blockMatch struct {
		startLine int // ```json 所在行号
		endLine   int // ``` 闭合所在行号
		cards     []map[string]any
	}
	var blocks []blockMatch

	inBlock := false
	blockStart := -1
	var jsonBuf strings.Builder

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if trimmed == "```json" || trimmed == "```JSON" {
				inBlock = true
				blockStart = i
				jsonBuf.Reset()
			}
			continue
		}
		// inBlock == true
		if trimmed == "```" {
			// 闭合 fence——尝试解析这个 block
			var probe struct {
				Cards []map[string]any `json:"cards"`
			}
			if err := json.Unmarshal([]byte(jsonBuf.String()), &probe); err == nil && probe.Cards != nil {
				// 只接受 cards 为非空数组的 block；cards 缺失或为 null/非数组则丢弃
				blocks = append(blocks, blockMatch{
					startLine: blockStart,
					endLine:   i,
					cards:     probe.Cards,
				})
			}
			inBlock = false
			blockStart = -1
			continue
		}
		jsonBuf.WriteString(line)
		jsonBuf.WriteString("\n")
	}

	if len(blocks) == 0 {
		return nil, content, ""
	}

	// 合并所有 block 的 cards，为缺失 id 的卡补 UUID。
	cards = make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		for _, c := range b.cards {
			if c == nil {
				continue
			}
			if _, ok := c["id"]; !ok {
				c["id"] = uuid.NewString()
			}
			cards = append(cards, c)
		}
	}

	if len(cards) == 0 {
		return nil, content, ""
	}

	// 构建 strippedContent：用 [CARD:<id>] 占位符替换每个有效 block（含 fence 行）。
	// 处理嵌套：在第一遍识别时 block 不会嵌套（一个 block 闭合前不会开启新的），所以
	// 行区间不会重叠，可以按 startLine 升序逐个替换。
	var out strings.Builder
	cursor := 0
	for _, b := range blocks {
		// 写入 block 之前的原文行。
		for i := cursor; i < b.startLine; i++ {
			out.WriteString(lines[i])
			out.WriteString("\n")
		}
		// 写入占位符——每张卡一行，按数组顺序。
		for _, c := range b.cards {
			if c == nil {
				continue
			}
			id, _ := c["id"].(string)
			out.WriteString("[CARD:")
			out.WriteString(id)
			out.WriteString("]\n")
		}
		cursor = b.endLine + 1
	}
	// 写入剩余行
	for i := cursor; i < len(lines); i++ {
		out.WriteString(lines[i])
		out.WriteString("\n")
	}

	// strings.Split 在 content 非空时末尾会带一个空串元素，对应的 "\n" 会被上面循环写回；
	// 为避免多余尾部换行，trim 等于原 content 末尾的内容。
	stripped := out.String()
	// 去掉由于循环统一加 "\n" 而多出来的末尾换行（与原 content 的末尾对齐）。
	if strings.HasSuffix(stripped, "\n") {
		stripped = strings.TrimSuffix(stripped, "\n")
		if !strings.HasSuffix(content, "\n") {
			// 原 content 不以换行结尾时，补一次移除（上面 split 会多一个空元素）
		} else {
			stripped += "\n"
		}
	}

	b, err := json.Marshal(cards)
	if err != nil {
		// 极不可能发生（输入已经是 []map[string]any）
		return cards, stripped, ""
	}
	return cards, stripped, string(b)
}

func codeArtifactsFromMarkdown(content string) []model.Artifact {
	parsed := artifactsFromMarkdown(content)
	if len(parsed) == 0 {
		return nil
	}
	codeArtifacts := make([]model.Artifact, 0, len(parsed))
	for _, artifact := range parsed {
		if artifact.Type == "code" {
			codeArtifacts = append(codeArtifacts, artifact)
		}
	}
	return codeArtifacts
}

// ConvRepoForMsg 消息服务需要的对话仓库接口（用于权限校验和成员查询）。
// Deprecated: migrate to repository.ConvStore for canonical interface.
type ConvRepoForMsg interface {
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	UpdateTimestamp(ctx context.Context, id string) error
	GetMember(ctx context.Context, conversationID, userID string) (*model.ConversationMember, error)
	ListMemberIDs(ctx context.Context, conversationID string) ([]string, error)
	ListAgents(ctx context.Context, conversationID, userID string) ([]model.ConversationAgent, error)
}

// ConvRepoForDaemon 是 DaemonHandler 需要的 conv 仓库子集（仅 ListMemberIDs）。
//
// 引入原因（本任务）：handleTaskProgress 原实现 Broadcast 推流式事件给所有在线用户，
// 改用 PushStreamingToConversation 按会话成员推送需要查成员列表。抽独立接口而非
// 直接用 ConvRepoForMsg 让 handler 包依赖面最小（不引入 GetMember / ListAgents 等
// handler 用不到的方法）。
//
// repository.ConvStore 自动满足此接口。
type ConvRepoForDaemon interface {
	ListMemberIDs(ctx context.Context, conversationID string) ([]string, error)
}

// AgentRepoForMsg 消息服务查询 Agent 用于对话接入。
// Deprecated: migrate to repository.AgentStore for canonical interface.
type AgentRepoForMsg interface {
	GetByID(ctx context.Context, id string) (*model.Agent, error)
	IsAgentInConversation(ctx context.Context, conversationID, agentID, userID string) (bool, error)
	CreateDaemonTask(ctx context.Context, userID, conversationID, agentID, machineID, cliTool, prompt, contextMessages string) (*model.DaemonTask, error)
	GetDaemonTask(ctx context.Context, id string) (*model.DaemonTask, error)
}

var (
	ErrMsgConvNotFound      = errors.New("对话不存在")
	ErrMsgConvNoPerm        = errors.New("无权操作此对话")
	ErrMsgTooLong           = errors.New("消息内容过长")
	ErrMsgNotFound          = errors.New("消息不存在")
	ErrMsgNotSender         = errors.New("只能撤回自己的消息")
	ErrMsgRecallExpired     = errors.New("消息已超过2分钟，无法撤回")
	ErrMsgAlreadyDeleted    = errors.New("消息已被撤回")
	ErrMsgEmptyContent      = errors.New("消息内容不能为空")
	ErrMsgReplyNotFound     = errors.New("回复的消息不存在")
	ErrMsgReplyWrongConv    = errors.New("回复的消息不属于当前对话")
	ErrMsgAgentNoPerm       = errors.New("无权使用此 Agent")
	ErrMsgAgentOffline      = errors.New("Agent 未连接电脑，无法执行真实 CLI")
	ErrMsgAgentTimeout      = errors.New("Agent 执行超时")
	ErrMsgBlackboardTooLong = errors.New("黑板内容过长")
)

// MessageService 消息业务逻辑
type MessageService struct {
	msgRepo         MsgRepo
	convRepo        ConvRepoForMsg
	agentRepo       AgentRepoForMsg
	notifier        MessageNotifier
	delivery        MessageDeliveryState
	orchSvc         *OrchestratorService
	daemonHub       port.DaemonDispatcher
	fileURLs        *FileURLBuilder
	taskCardQueue   *TaskCardQueue // 收集 MCP subprocess 工具 emit 的卡片，按 task_id 索引
	streamingBuffer *StreamingBuffer // PR4: 累积 task_id 对应的 StreamingState，task.complete 时落权威 blocks_json
}

// SendMessageResult 发送消息后的用户消息和可选 Agent 回复。
type SendMessageResult struct {
	UserMessage  *model.Message `json:"user_message"`
	AgentMessage *model.Message `json:"agent_message,omitempty"`
}

// NewMessageService 创建消息服务
func NewMessageService(msgRepo MsgRepo, convRepo ConvRepoForMsg, agentRepo AgentRepoForMsg) *MessageService {
	return &MessageService{msgRepo: msgRepo, convRepo: convRepo, agentRepo: agentRepo, fileURLs: NewFileURLBuilder("")}
}

// SetNotifier 注入消息推送器（避免循环依赖，由 main.go 调用）
func (s *MessageService) SetNotifier(n MessageNotifier) {
	s.notifier = n
}

// SetDeliveryState injects transient delivery state storage.
func (s *MessageService) SetDeliveryState(c MessageDeliveryState) {
	s.delivery = c
}

// SetCacher is kept for compatibility with older wiring code.
func (s *MessageService) SetCacher(c MessageDeliveryState) {
	s.SetDeliveryState(c)
}

// SetOrchestratorService 注入 Orchestrator 服务（避免循环依赖，由 main.go 调用）
func (s *MessageService) SetOrchestratorService(orchSvc *OrchestratorService) {
	s.orchSvc = orchSvc
}

// SetDaemonHub 注入 DaemonDispatcher（避免循环依赖，由 main.go 调用）。
//
// P8b: 字段类型由 *ws.DaemonHub 改为 port.DaemonDispatcher，service 层不再
// 直接依赖 pkg/ws 具体实现。*ws.DaemonHub 通过 Go 结构化类型自动满足该接口，
// 调用方（main.go）依然传 *ws.DaemonHub 指针，无需改动。
func (s *MessageService) SetDaemonHub(dh port.DaemonDispatcher) {
	s.daemonHub = dh
}

// SetFileURLBuilder injects the public URL policy for message attachments.
func (s *MessageService) SetFileURLBuilder(builder *FileURLBuilder) {
	if builder == nil {
		builder = NewFileURLBuilder("")
	}
	s.fileURLs = builder
}

// SetTaskCardQueue 注入 MCP subprocess 卡片队列。daemon 主进程在 createAgentReply
// 时会 Drain task_id 对应的卡片，合并到 message.cards_json。
func (s *MessageService) SetTaskCardQueue(q *TaskCardQueue) {
	s.taskCardQueue = q
}

// SetStreamingBuffer 注入流式状态 buffer。
//
// PR4: daemon task.progress 时 handler 把 events 喂给 buffer 累积；
// createAgentReply 在 task.complete 时 GetState 拿到权威 blocks 落库，
// 然后 Delete 释放内存。nil 时 fallback 到空 blocks_json（兼容旧路径）。
func (s *MessageService) SetStreamingBuffer(b *StreamingBuffer) {
	s.streamingBuffer = b
}

// checkMembership 校验用户是否为会话成员（优先查成员表）
func (s *MessageService) checkMembership(ctx context.Context, conv *model.Conversation, userID string) error {
	member, err := s.convRepo.GetMember(ctx, conv.ID, userID)
	if err != nil {
		return fmt.Errorf("check member: %w", err)
	}
	if member != nil {
		return nil
	}
	// Fallback: creator may not yet be in members table
	if conv.UserID == userID {
		return nil
	}
	return ErrMsgConvNoPerm
}

// SendMessage 发送消息：持久化 → 推送 → 缓存
func (s *MessageService) SendMessage(ctx context.Context, convID, userID, role, content, artifactsJSON string, attachments []model.MessageAttachment) (*SendMessageResult, error) {
	return s.SendMessageWithReply(ctx, convID, userID, role, content, artifactsJSON, attachments, nil, "", nil)
}

// SendMessageWithMentions 发送消息（支持 mentions）
func (s *MessageService) SendMessageWithMentions(ctx context.Context, convID, userID, role, content, artifactsJSON string, attachments []model.MessageAttachment, mentions []string) (*SendMessageResult, error) {
	return s.SendMessageWithReply(ctx, convID, userID, role, content, artifactsJSON, attachments, nil, "", mentions)
}

// SendMessageWithReply 发送消息（支持回复引用和 Agent 回复）
func (s *MessageService) SendMessageWithReply(ctx context.Context, convID, userID, role, content, artifactsJSON string, attachments []model.MessageAttachment, replyTo *string, agentID string, mentions []string) (*SendMessageResult, error) {
	if len(content) > maxMessageLen {
		return nil, ErrMsgTooLong
	}
	// 允许纯附件消息（无文字内容），仅当内容为空且无附件时拒绝
	if strings.TrimSpace(content) == "" && len(attachments) == 0 {
		return nil, ErrMsgEmptyContent
	}

	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return nil, err
	}

	// 强制角色约束：客户端只允许发送 user 角色消息，防止伪造 assistant 消息
	role = "user"

	// 校验 reply_to 引用的消息
	if replyTo != nil {
		refMsg, err := s.msgRepo.GetByID(ctx, *replyTo)
		if err != nil || refMsg == nil {
			return nil, ErrMsgReplyNotFound
		}
		if refMsg.DeletedAt != nil {
			return nil, ErrMsgReplyNotFound
		}
		if refMsg.ConversationID != convID {
			return nil, ErrMsgReplyWrongConv
		}
	}

	var senderID *string
	if role == "user" {
		senderID = &userID
	}
	msg, err := s.msgRepo.Create(ctx, convID, role, content, artifactsJSON, attachments, replyTo, senderID, mentions)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	s.enrichMessageFileURLs(msg)

	result := &SendMessageResult{UserMessage: msg}

	// Agent dispatch routing based on conversation type
	switch conv.Type {
	case "agent":
		// Single/agent chat — direct dispatch via agentID
		resolvedAgentID := strings.TrimSpace(agentID)
		if resolvedAgentID == "" {
			resolvedAgentID = s.resolveAgentConversationAgentID(ctx, convID, userID)
		}
		slog.Info("agent chat dispatch resolved", "conversation_id", convID, "agent_id", resolvedAgentID, "provided_agent_id", strings.TrimSpace(agentID) != "")
		if resolvedAgentID != "" {
			go s.asyncAgentReply(convID, userID, resolvedAgentID, content, msg.Attachments, &msg.ID)
		}
	case "group":
		// Group chat — mention routing via Orchestrator
		if s.orchSvc != nil {
			parsedMentions := ParseMentions(content)
			if len(mentions) > 0 || len(parsedMentions) > 0 {
				slog.Info(orchFlowLog, "stage", "message.async_mention_enqueued", "conversation_id", convID, "user_id", userID, "source_message_id", msg.ID, "mention_count", len(parsedMentions))
				go s.asyncMentionDispatch(convID, userID, msg.ID, content, msg.Attachments)
			}
		}
	default:
		// "single" or other types — no agent dispatch
	}

	// 异步推送和缓存（失败不影响消息持久化）
	go s.postPersist(convID, userID, msg)

	return result, nil
}

// postPersist 持久化后的推送和缓存操作
func (s *MessageService) postPersist(conversationID, senderID string, msg *model.Message) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("postPersist recovered from panic", "conversation_id", conversationID, "panic", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 获取会话成员列表
	memberIDs, err := s.convRepo.ListMemberIDs(ctx, conversationID)
	if err != nil {
		slog.Warn("list members failed in postPersist", "conversation_id", conversationID, "error", err)
		memberIDs = []string{senderID}
	}
	if len(memberIDs) == 0 {
		memberIDs = []string{senderID}
	}

	// 推送给所有会话成员
	if s.notifier != nil {
		s.notifier.PushToConversation(conversationID, memberIDs, msg)
	}

	// Record transient delivery state. The persisted message row remains the
	// source of truth for history reads.
	if s.delivery != nil {
		for _, uid := range memberIDs {
			if uid == senderID {
				continue
			}
			if s.notifier != nil && !s.notifier.IsOnline(uid) {
				if err := s.delivery.EnqueueOffline(ctx, uid, conversationID, msg); err != nil {
					slog.Warn("enqueue offline failed", "user_id", uid, "conversation_id", conversationID, "error", err)
				}
			}
			if err := s.delivery.IncrementUnread(ctx, uid, conversationID); err != nil {
				slog.Warn("increment unread failed", "user_id", uid, "conversation_id", conversationID, "error", err)
			}
		}
	}
}

// MarkAsRead 标记会话消息已读
func (s *MessageService) MarkAsRead(ctx context.Context, userID, convID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return err
	}

	if err := s.msgRepo.MarkConversationRead(ctx, convID, userID); err != nil {
		return err
	}

	if s.delivery != nil {
		_ = s.delivery.ClearUnread(ctx, userID, convID)
		// 同时清空离线消息队列，防止 GetUnreadMessages 再次返回已读消息
		_, _ = s.delivery.DequeueOfflineAfter(ctx, userID, convID, "+inf")
	}

	return nil
}

// GetHistory 获取对话消息历史，支持 before 游标分页
func (s *MessageService) GetHistory(ctx context.Context, convID, userID string, before interface{}, limit int) ([]model.Message, error) {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}

	messages, err := s.msgRepo.ListByConversation(ctx, convID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	// 过滤当前用户已隐藏的消息
	messages = s.filterHidden(ctx, messages, convID, userID)
	s.ensureParsedArtifacts(ctx, messages)
	s.enrichMessagesFileURLs(messages)
	return messages, nil
}

// filterHidden 从消息列表中移除当前用户已隐藏的消息。
func (s *MessageService) filterHidden(ctx context.Context, messages []model.Message, convID, userID string) []model.Message {
	if userID == "" || len(messages) == 0 {
		return messages
	}
	hidden, err := s.msgRepo.GetHiddenMessageIDs(ctx, userID, convID)
	if err != nil {
		return messages // 查询失败不阻塞，返回完整列表
	}
	if len(hidden) == 0 {
		return messages
	}
	filtered := messages[:0]
	for _, m := range messages {
		if !hidden[m.ID] {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// HideMessage 对当前用户隐藏消息。
func (s *MessageService) HideMessage(ctx context.Context, userID, messageID string) error {
	return s.msgRepo.HideMessage(ctx, userID, messageID)
}

// UnhideMessage 取消隐藏。
func (s *MessageService) UnhideMessage(ctx context.Context, userID, messageID string) error {
	return s.msgRepo.UnhideMessage(ctx, userID, messageID)
}

// UpdateMessageCards 更新消息的卡片状态（用户交互后调用）。
func (s *MessageService) UpdateMessageCards(ctx context.Context, messageID, userID, cardsJSON string) error {
	return s.msgRepo.UpdateMessageCards(ctx, messageID, cardsJSON)
}

// UpdateMessageCardsAndBroadcast 更新卡片状态并向会话所有在线成员广播更新后的消息，
// 使其他成员也能实时看到新状态（如用户提交 plan 选择后）。
//
// 与 UpdateMessageCards 的区别：后者只 UPDATE DB 列不广播，多成员场景下其他人看不到更新。
// 本方法只走 PushToConversation（message.complete 事件），不触发离线队列/未读计数——
// 卡片更新不是新消息，不应改变未读数。
//
// 校验：cardsJSON 反序列化后过 ValidateCards，过滤掉 type 未知 / 必填缺失的脏卡，
// 避免前端 PATCH 错误格式破坏 DB 完整性 + 防止其他成员渲染时崩溃。
func (s *MessageService) UpdateMessageCardsAndBroadcast(ctx context.Context, messageID, cardsJSON string) error {
	// 反序列化 + 校验，再序列化回写——确保落库的 cards_json 始终是 validated 形态。
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(cardsJSON), &parsed); err != nil {
		return fmt.Errorf("parse cards json: %w", err)
	}
	validated := ValidateCards(parsed)
	validatedJSON, err := json.Marshal(validated)
	if err != nil {
		return fmt.Errorf("marshal validated cards: %w", err)
	}
	validatedStr := string(validatedJSON)

	if err := s.msgRepo.UpdateMessageCards(ctx, messageID, validatedStr); err != nil {
		return fmt.Errorf("update cards: %w", err)
	}
	msg, err := s.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("reload message: %w", err)
	}
	if msg == nil {
		return ErrMsgNotFound
	}
	if err := json.Unmarshal([]byte(validatedStr), &msg.Cards); err != nil {
		slog.Warn("unmarshal cards after update", "message_id", messageID, "error", err)
	}
	msg.CardsJSON = validatedStr

	// 直接通过 notifier 广播 message.complete，避免 postPersist 的未读计数/离线入队副作用。
	if s.notifier != nil {
		memberIDs, err := s.convRepo.ListMemberIDs(ctx, msg.ConversationID)
		if err != nil {
			slog.Warn("list members for cards broadcast failed", "conversation_id", msg.ConversationID, "error", err)
		} else if len(memberIDs) > 0 {
			s.notifier.PushToConversation(msg.ConversationID, memberIDs, msg)
		}
	}
	return nil
}

// GetUnreadMessages 获取离线/未读消息
func (s *MessageService) GetUnreadMessages(ctx context.Context, convID, userID string, limit int) ([]model.Message, error) {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}

	if s.delivery != nil {
		offline, err := s.delivery.DequeueOfflineAfter(ctx, userID, convID, "-inf")
		if err == nil && len(offline) > 0 {
			s.enrichMessagesFileURLs(offline)
			return offline, nil
		}
	}

	// 降级查询：使用 last_read_at 作为起点，而非返回全部消息
	member, _ := s.convRepo.GetMember(ctx, convID, userID)
	var afterTime interface{}
	if member != nil && member.LastReadAt != nil && *member.LastReadAt != "" {
		afterTime = *member.LastReadAt
	}

	messages, err := s.msgRepo.GetMessagesAfter(ctx, convID, afterTime, limit)
	if err != nil {
		return nil, fmt.Errorf("get unread messages: %w", err)
	}
	messages = s.filterHidden(ctx, messages, convID, userID)
	s.enrichMessagesFileURLs(messages)
	return messages, nil
}

// SearchMessages 搜索对话消息
func (s *MessageService) SearchMessages(ctx context.Context, conversationID, userID, keyword string) ([]model.Message, error) {
	conv, err := s.convRepo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return nil, err
	}
	messages, err := s.msgRepo.SearchByContent(ctx, conversationID, keyword, 20)
	if err != nil {
		return nil, err
	}
	s.enrichMessagesFileURLs(messages)
	return messages, nil
}

// GetReplies 获取某条消息的所有回复（验证消息归属和状态后再查询）
func (s *MessageService) GetReplies(ctx context.Context, conversationID, messageID string) ([]model.Message, error) {
	// 校验目标消息存在
	msg, err := s.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMsgNotFound
		}
		return nil, fmt.Errorf("get message: %w", err)
	}
	if msg == nil {
		return nil, ErrMsgNotFound
	}
	// 消息不属于当前对话
	if msg.ConversationID != conversationID {
		return nil, ErrMsgReplyWrongConv
	}
	// 消息已被软删除
	if msg.DeletedAt != nil {
		return nil, ErrMsgNotFound
	}

	replies, err := s.msgRepo.ListReplies(ctx, messageID)
	if err != nil {
		return nil, fmt.Errorf("list replies: %w", err)
	}
	s.enrichMessagesFileURLs(replies)
	return replies, nil
}

// GetMessageByID 取单条消息，校验其属于指定对话且未删除。
// 供需要 message-level 权限校验的 handler（如 UpdateCard）使用——
// 避免 handler 直接访问 repo，权限逻辑收敛在 service 层。
func (s *MessageService) GetMessageByID(ctx context.Context, conversationID, messageID string) (*model.Message, error) {
	msg, err := s.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMsgNotFound
		}
		return nil, fmt.Errorf("get message: %w", err)
	}
	if msg == nil {
		return nil, ErrMsgNotFound
	}
	// 消息不属于当前对话
	if msg.ConversationID != conversationID {
		return nil, ErrMsgReplyWrongConv
	}
	// 消息已被软删除
	if msg.DeletedAt != nil {
		return nil, ErrMsgNotFound
	}
	return msg, nil
}

// CancelStreamingMessage 向 daemon 发送 task.cancel 控制消息，异步中断流式生成。
//
// 设计：
//   - 不等 daemon 响应——返回 202 Accepted，UI 立即 disable 按钮。
//   - daemon 侧（未来实现）收到 task.cancel 后 SIGINT Claude 进程，触发 task.complete with error，
//     后端 watchdog FinalizeStreaming 切到 canceled/error 并广播 message.complete。
//   - 当前 PR3 只负责 backend → daemon 的下发链路；daemon 侧 SIGINT 在另一个 PR 补全。
//
// agent_id 从 artifacts_json 解析；task_id 优先从参数读取（前端从 message.streaming
// payload 拿到 task_id 回传），缺省时只能告警——无 task_id daemon 无法定位流。
func (s *MessageService) CancelStreamingMessage(ctx context.Context, conversationID, messageID, taskID string) error {
	msg, err := s.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMsgNotFound
		}
		return fmt.Errorf("get message: %w", err)
	}
	if msg == nil {
		return ErrMsgNotFound
	}
	if msg.ConversationID != conversationID {
		return ErrMsgReplyWrongConv
	}
	if msg.Status != "" && msg.Status != model.MessageStatusStreaming {
		// 非流式状态——幂等返回 nil，前端显示已是终态。
		slog.Info("cancel streaming: message not in streaming state", "message_id", messageID, "status", msg.Status)
		return nil
	}

	// 从 artifacts_json 解析 agent_id（预创建 streaming message 时写入）。
	var meta struct {
		AgentID string `json:"agent_id"`
	}
	if msg.ArtifactsJSON != "" {
		_ = json.Unmarshal([]byte(msg.ArtifactsJSON), &meta) // 解析失败忽略——下面 agent_id 为空会返回错误
	}
	if meta.AgentID == "" {
		return fmt.Errorf("cancel streaming: agent_id missing in artifacts_json for message %s", messageID)
	}
	agent, err := s.agentRepo.GetByID(ctx, meta.AgentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return ErrAgentNotFound
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return ErrMsgAgentOffline
	}
	if s.daemonHub == nil || !s.daemonHub.IsConnected(*agent.MachineID) {
		return ErrMsgAgentOffline
	}
	if taskID == "" {
		return fmt.Errorf("cancel streaming: task_id required for message %s", messageID)
	}
	// 异步发送 task.cancel——不等 daemon 响应，UI 收到 202 立即 disable。
	if err := s.daemonHub.SendToMachine(*agent.MachineID, ws.WSMessage{
		Type: "task.cancel",
		Data: map[string]interface{}{
			"task_id":         taskID,
			"agent_id":        agent.ID,
			"message_id":      messageID,
			"conversation_id": conversationID,
		},
	}); err != nil {
		return fmt.Errorf("send task.cancel: %w", err)
	}
	return nil
}

func (s *MessageService) enrichMessagesFileURLs(messages []model.Message) {
	for i := range messages {
		s.enrichMessageFileURLs(&messages[i])
	}
}

func (s *MessageService) enrichMessageFileURLs(message *model.Message) {
	if message == nil || s.fileURLs == nil {
		return
	}
	for i := range message.Attachments {
		message.Attachments[i].URL = s.fileURLs.UploadURL(message.Attachments[i].FilePath)
		message.Attachments[i].ThumbnailURL = s.fileURLs.UploadURL(message.Attachments[i].ThumbnailPath)
	}
}

func (s *MessageService) ensureParsedArtifacts(ctx context.Context, messages []model.Message) {
	for i := range messages {
		msg := &messages[i]
		if msg.Role != "assistant" || hasCodeArtifact(msg.Artifacts) || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		arts := codeArtifactsFromMarkdown(msg.Content)
		if len(arts) == 0 {
			continue
		}
		if err := s.msgRepo.SaveArtifacts(ctx, msg.ID, arts); err != nil {
			slog.Warn("backfill message artifacts failed", "message_id", msg.ID, "error", err)
			continue
		}
		msg.Artifacts = append(msg.Artifacts, arts...)
	}
}

// PinMessage pins a message into the shared conversation context blackboard.
func (s *MessageService) PinMessage(ctx context.Context, convID, messageID, userID string) (*model.MessagePin, error) {
	if err := s.ensureMessageContextAccess(ctx, convID, messageID, userID); err != nil {
		return nil, err
	}
	pin, err := s.msgRepo.PinMessage(ctx, convID, messageID, userID)
	if err != nil {
		return nil, fmt.Errorf("pin message: %w", err)
	}
	return pin, nil
}

// UnpinMessage removes a message from the shared conversation context blackboard.
func (s *MessageService) UnpinMessage(ctx context.Context, convID, messageID, userID string) error {
	if err := s.ensureMessageContextAccess(ctx, convID, messageID, userID); err != nil {
		return err
	}
	if err := s.msgRepo.UnpinMessage(ctx, convID, messageID); err != nil {
		return fmt.Errorf("unpin message: %w", err)
	}
	return nil
}

// ListPinnedContext returns the user's readable pinned context entries.
func (s *MessageService) ListPinnedContext(ctx context.Context, convID, userID string) ([]model.PinnedMessage, error) {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return nil, ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return nil, err
	}
	items, err := s.msgRepo.ListPinnedMessages(ctx, convID, 20)
	if err != nil {
		return nil, fmt.Errorf("list pinned context: %w", err)
	}
	if items == nil {
		items = []model.PinnedMessage{}
	}
	return items, nil
}

// GetConversationBlackboard returns the user-authored long-term context for a conversation.
func (s *MessageService) GetConversationBlackboard(ctx context.Context, convID, userID string) (*model.ConversationBlackboard, error) {
	if err := s.ensureConversationAccess(ctx, convID, userID); err != nil {
		return nil, err
	}
	blackboard, err := s.msgRepo.GetConversationBlackboard(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("get conversation blackboard: %w", err)
	}
	if blackboard == nil {
		blackboard = &model.ConversationBlackboard{ConversationID: convID, ManualContext: ""}
	}
	return blackboard, nil
}

// UpdateConversationBlackboard saves user-authored long-term context for a conversation.
func (s *MessageService) UpdateConversationBlackboard(ctx context.Context, convID, userID, manualContext string) (*model.ConversationBlackboard, error) {
	if len([]rune(manualContext)) > maxBlackboardManualContextLen {
		return nil, ErrMsgBlackboardTooLong
	}
	if err := s.ensureConversationAccess(ctx, convID, userID); err != nil {
		return nil, err
	}
	blackboard, err := s.msgRepo.UpsertConversationBlackboard(ctx, convID, manualContext, userID)
	if err != nil {
		return nil, fmt.Errorf("update conversation blackboard: %w", err)
	}
	return blackboard, nil
}

func (s *MessageService) ensureConversationAccess(ctx context.Context, convID, userID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrMsgConvNotFound
	}
	return s.checkMembership(ctx, conv, userID)
}

func (s *MessageService) ensureMessageContextAccess(ctx context.Context, convID, messageID, userID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return err
	}
	msg, err := s.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}
	if msg == nil || msg.DeletedAt != nil {
		return ErrMsgNotFound
	}
	if msg.ConversationID != convID {
		return ErrMsgReplyWrongConv
	}
	return nil
}

// ClearUnread 清除未读计数
func (s *MessageService) ClearUnread(ctx context.Context, userID, convID string) error {
	if s.delivery != nil {
		return s.delivery.ClearUnread(ctx, userID, convID)
	}
	return nil
}

// RecallMessage 撤回消息（软删除）
func (s *MessageService) RecallMessage(ctx context.Context, convID, messageID, userID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	if conv == nil {
		return ErrMsgConvNotFound
	}
	if err := s.checkMembership(ctx, conv, userID); err != nil {
		return err
	}

	// 查询消息
	msg, err := s.msgRepo.GetByID(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}
	if msg == nil {
		return ErrMsgNotFound
	}

	// 检查是否已删除
	if msg.DeletedAt != nil {
		return ErrMsgAlreadyDeleted
	}

	// 检查是否为消息发送者
	senderID, err := s.msgRepo.GetMessageSender(ctx, messageID)
	if err != nil {
		return fmt.Errorf("get message sender: %w", err)
	}
	if senderID != userID {
		return ErrMsgNotSender
	}

	// 检查时间限制
	if time.Since(msg.CreatedAt) > recallTimeLimit {
		return ErrMsgRecallExpired
	}

	// 软删除
	if err := s.msgRepo.SoftDelete(ctx, messageID); err != nil {
		return fmt.Errorf("recall message: %w", err)
	}

	// 撤回成功后异步推送通知给其他成员（排除发送者）
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("recall push recovered from panic", "conversation_id", convID, "panic", r)
			}
		}()

		bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		memberIDs, err := s.convRepo.ListMemberIDs(bgCtx, convID)
		if err != nil {
			slog.Warn("list members for recall push failed", "conversation_id", convID, "error", err)
			return
		}

		// 过滤掉撤回者本人（本地已做乐观更新）
		filtered := make([]string, 0, len(memberIDs))
		for _, uid := range memberIDs {
			if uid != userID {
				filtered = append(filtered, uid)
			}
		}

		if s.notifier != nil && len(filtered) > 0 {
			s.notifier.PushCustomEvent(convID, filtered, "message.recall", map[string]interface{}{
				"message_id":      messageID,
				"conversation_id": convID,
			})
		}
	}()

	return nil
}

// agentHandoff 表示一条 agent 的任务交接单，只收集 agent 的回复摘要。
type agentHandoff struct {
	AgentName   string `json:"agent_name"`
	AgentID     string `json:"agent_id"`
	UserRequest string `json:"user_request"`
	Result      string `json:"result"`
}

// buildAgentHandoffs 只收集其他 agent 的回复摘要，构建精简交接单。
// 遍历最近消息，找到每条 assistant 消息（含 agent_name），向前找触发它的最近一条
// user 消息作为 user_request，结果截断到 500 字符，最多保留 5 条。
func (s *MessageService) buildAgentHandoffs(ctx context.Context, convID string) string {
	const fetchLimit = 40
	const maxHandoffs = 5
	const maxResultLen = 500

	msgs, err := s.msgRepo.ListByConversation(ctx, convID, nil, fetchLimit)
	if err != nil || len(msgs) == 0 {
		return ""
	}
	// ListByConversation 返回倒序（最新在前），需要反转为时间正序
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	handoffs := make([]agentHandoff, 0, maxHandoffs)
	for i, m := range msgs {
		if m.Role != "assistant" || m.ArtifactsJSON == "" {
			continue
		}
		var a struct {
			AgentID   string `json:"agent_id"`
			AgentName string `json:"agent_name"`
		}
		if err := json.Unmarshal([]byte(m.ArtifactsJSON), &a); err != nil || a.AgentName == "" {
			continue
		}

		// 向前找触发该 agent 的最近一条 user 消息
		userRequest := ""
		for j := i - 1; j >= 0; j-- {
			if msgs[j].Role == "user" {
				userRequest = msgs[j].Content
				break
			}
		}

		result := m.Content
		if len([]rune(result)) > maxResultLen {
			runes := []rune(result)
			result = string(runes[:maxResultLen]) + "..."
		}

		handoffs = append(handoffs, agentHandoff{
			AgentName:   a.AgentName,
			AgentID:     a.AgentID,
			UserRequest: userRequest,
			Result:      result,
		})
		if len(handoffs) >= maxHandoffs {
			break
		}
	}

	if len(handoffs) == 0 {
		return ""
	}
	b, err := json.Marshal(handoffs)
	if err != nil {
		return ""
	}
	return string(b)
}

// createAgentReply 生成 Agent 回复消息
func (s *MessageService) createAgentReply(ctx context.Context, convID, userID, agentID, userContent, contextMessages string, replyTo *string) (*model.Message, error) {
	if s.agentRepo == nil {
		return nil, ErrAgentNotFound
	}
	agent, err := s.agentRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	if agent.UserID != nil && *agent.UserID != userID {
		return nil, ErrMsgAgentNoPerm
	}
	ok, err := s.agentRepo.IsAgentInConversation(ctx, convID, agent.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("check conversation agent: %w", err)
	}
	if !ok {
		return nil, ErrMsgAgentNoPerm
	}
	if agent.MachineID == nil || *agent.MachineID == "" {
		return nil, ErrMsgAgentOffline
	}
	if agent.Status == "stopped" {
		return nil, fmt.Errorf("agent %q 已被用户停止", agent.Name)
	}

	task, err := s.agentRepo.CreateDaemonTask(ctx, userID, convID, agent.ID, *agent.MachineID, agent.CLITool, userContent, contextMessages)
	if err != nil {
		return nil, fmt.Errorf("create daemon task: %w", err)
	}

	// D5 ADR：预创建 streaming 状态的 assistant message，让前端在流式期间就能看到
	// message_id 并实时 append delta。task.complete 时 FinalizeStreaming 切到 complete。
	// 失败路径：daemon 崩溃 / 超时 / 用户取消 → watchdog 或显式 UPDATE 切到 error/canceled。
	//
	// 实现细节抽到 streaming_pipeline.go 的 SetupStreamingPipeline / FinalizeStreamingPipeline，
	// 单聊（createAgentReply）与群聊 worker（Dispatcher.dispatchPlanCore）共用同一管线。
	pipelineDeps := StreamingPipelineDeps{
		MsgRepo:         s.msgRepo,
		DaemonHub:       s.daemonHub,
		StreamingBuffer: s.streamingBuffer,
		Notifier:        s.notifier,
		ConvRepo:        s.convRepo,
	}
	var taskAgentIndex TaskAgentIndex
	if dh, ok := s.daemonHub.(*ws.DaemonHub); ok {
		taskAgentIndex = dh
	}
	handle, err := SetupStreamingPipeline(ctx, pipelineDeps, convID, agent.ID, agent.Name, agent.CLITool, task.ID, replyTo, taskAgentIndex)
	if err != nil {
		return nil, err
	}

	// Push via WS and wait for channel-based result
	if s.daemonHub == nil || !s.daemonHub.IsConnected(*agent.MachineID) {
		// daemon 未连接：直接把预创建 message 标 error，避免悬挂。
		if _, ferr := FinalizeStreamingPipeline(ctx, pipelineDeps, handle, FinalizeStreamingPipelineOptions{
			Status: model.MessageStatusError,
		}); ferr != nil {
			slog.Warn("finalize streaming on daemon-disconnect failed", "message_id", handle.MessageID, "error", ferr)
		}
		return nil, fmt.Errorf("agent %q 的 daemon 未通过 WS 连接", agent.Name)
	}
	slog.Info("createAgentReply: BEFORE SendToMachine", "conversation_id", convID, "agent_id", agent.ID, "daemon_task_id", task.ID, "message_id", handle.MessageID, "reply_to", stringValue(replyTo))
	if err := s.daemonHub.SendToMachine(*agent.MachineID, ws.WSMessage{
		Type: "task.dispatch",
		Data: map[string]interface{}{
			"task_id":          task.ID,
			"cli_tool":         agent.CLITool,
			"prompt":           userContent,
			"context_messages": contextMessages,
			"agent_id":         agent.ID,
			"conversation_id":  convID,
			"user_id":          userID,
			"message_id":       handle.MessageID,
		},
	}); err != nil {
		// dispatch 失败也标 error
		if _, ferr := FinalizeStreamingPipeline(ctx, pipelineDeps, handle, FinalizeStreamingPipelineOptions{
			Status: model.MessageStatusError,
		}); ferr != nil {
			slog.Warn("finalize streaming on dispatch-failure failed", "message_id", handle.MessageID, "error", ferr)
		}
		return nil, fmt.Errorf("dispatch to daemon: %w", err)
	}
	slog.Info("createAgentReply: AFTER SendToMachine", "conversation_id", convID, "agent_id", agent.ID, "daemon_task_id", task.ID, "message_id", handle.MessageID)

	ch := s.daemonHub.AwaitTaskResult(task.ID)
	if ch == nil {
		if _, ferr := FinalizeStreamingPipeline(ctx, pipelineDeps, handle, FinalizeStreamingPipelineOptions{
			Status: model.MessageStatusError,
		}); ferr != nil {
			slog.Warn("finalize streaming on no-channel failed", "message_id", handle.MessageID, "error", ferr)
		}
		return nil, fmt.Errorf("daemon not connected for task %s", task.ID)
	}
	defer s.daemonHub.RemoveTaskPromise(task.ID)
	if taskAgentIndex != nil {
		defer taskAgentIndex.DeleteTaskAgent(task.ID)
	}
	defer s.daemonHub.DeleteTaskMessage(task.ID)
	if s.streamingBuffer != nil {
		defer s.streamingBuffer.Delete(task.ID)
	}

	ctx, cancel := context.WithTimeout(ctx, 400*time.Second)
	defer cancel()

	var result *ws.TaskResult
	select {
	case result = <-ch:
	case <-ctx.Done():
		// 超时：标记 error（保留已输出的流式 block）。
		// 同步广播终态，让前端停止显示 streaming cursor / StopButton。
		if _, ferr := FinalizeStreamingPipeline(ctx, pipelineDeps, handle, FinalizeStreamingPipelineOptions{
			Status: model.MessageStatusError,
		}); ferr != nil {
			slog.Warn("finalize streaming on timeout failed", "message_id", handle.MessageID, "error", ferr)
		}
		BroadcastStreamingTerminal(pipelineDeps, handle, model.MessageStatusError)
		return nil, ErrMsgAgentTimeout
	}

	if result.Error != "" {
		// daemon task 失败：标记 error，content 留空（前端可显示 error block + 错误原因）。
		// 同步广播终态，让前端 addMessage 把 status 切到 error 并清理 streamingTaskIds。
		if _, ferr := FinalizeStreamingPipeline(ctx, pipelineDeps, handle, FinalizeStreamingPipelineOptions{
			Status: model.MessageStatusError,
		}); ferr != nil {
			slog.Warn("finalize streaming on task-error failed", "message_id", handle.MessageID, "error", ferr)
		}
		BroadcastStreamingTerminal(pipelineDeps, handle, model.MessageStatusError)
		return nil, fmt.Errorf("daemon task failed: %s", result.Error)
	}

	// 提取 agent 在正文 ```json {"cards":[...]}``` block 里写的卡片，同时把 block 从
	// 用户可见正文里剥离掉（替换为 [CARD:<id>] 占位符）。
	agentCards, strippedContent, _ := extractCardsFromContent(result.Result)

	// 合并 daemon emitted cards：
	//   1) result.Cards —— daemon 主进程通过 WS task.complete 上行的卡片
	//   2) taskCardQueue 里 drain 出的 subprocess 卡片
	//   3) agentCards —— agent 在回复正文写的 ```json{"cards":[...]}``` block
	allCards := append([]map[string]any{}, ValidateCards(result.Cards)...)
	if s.taskCardQueue != nil {
		if subprocessCards := s.taskCardQueue.Drain(task.ID); len(subprocessCards) > 0 {
			allCards = append(allCards, ValidateCards(subprocessCards)...)
		}
	}
	allCards = append(allCards, ValidateCards(agentCards)...)

	// D5 ADR：预创建模式下用 FinalizeStreaming UPDATE 而非 Create INSERT。
	// content = strippedContent（剥离 cards 后的可见文本）
	// blocks_json = streamingBuffer 累积的权威 blocks
	// status = complete
	// 与 dispatcher.finalizeStreamingSuccess 共用同一包级函数 snapshotBlocksJSONFromBuffer。
	blocksJSON := snapshotBlocksJSONFromBuffer(s.streamingBuffer, task.ID)
	msg, err := FinalizeStreamingPipeline(ctx, pipelineDeps, handle, FinalizeStreamingPipelineOptions{
		Status:        model.MessageStatusComplete,
		Content:       strippedContent,
		BlocksJSON:    blocksJSON,
		ArtifactsJSON: handle.ArtifactsJSON,
		Cards:         allCards,
		Artifacts:     artifactsFromTaskResultOrMarkdown(result.Artifacts, strippedContent),
	})
	if err != nil {
		return nil, fmt.Errorf("finalize streaming message: %w", err)
	}

	return msg, nil
}

// broadcastAgentTyping 通过 WebSocket 广播 agent 正在处理任务的状态

// asyncMentionDispatch 异步执行 @mention 路由，不阻塞 HTTP 响应。
func (s *MessageService) asyncMentionDispatch(convID, userID, sourceMessageID, content string, attachments []model.MessageAttachment) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn(orchFlowLog, "stage", "message.async_mention_panic", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID, "panic", r)
		}
	}()

	slog.Info(orchFlowLog, "stage", "message.async_mention_start", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID, "content_len", len(content), "content_preview", orchPreview(content))
	s.broadcastAgentTyping(convID, true)
	defer s.broadcastAgentTyping(convID, false)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	orchResult, err := s.orchSvc.RouteMention(ctx, convID, userID, content, attachments, &sourceMessageID)
	if err != nil {
		slog.Warn(orchFlowLog, "stage", "message.async_mention_route_failed", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID, "error", err)
		s.postAgentFailure(ctx, convID, userID, "Agent 调用失败："+shortAgentError(err), &sourceMessageID)
		return
	}
	if orchResult == nil || len(orchResult.AgentMessages) == 0 {
		slog.Info(orchFlowLog, "stage", "message.async_mention_no_messages", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID)
		return
	}
	slog.Info(orchFlowLog, "stage", "message.async_mention_messages_ready", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID, "message_count", len(orchResult.AgentMessages))
	for _, agentMsg := range orchResult.AgentMessages {
		slog.Info(orchFlowLog, "stage", "message.async_mention_push", "conversation_id", convID, "user_id", userID, "source_message_id", sourceMessageID, "message_id", agentMsg.ID, "reply_to", stringValue(agentMsg.ReplyTo), "content_len", len(agentMsg.Content))
		s.postPersist(convID, userID, agentMsg)
	}
}

// asyncAgentReply 异步执行 agentID 路径回复，不阻塞 HTTP 响应。
// asyncAgentReply 异步执行 agentID 路径回复，不阻塞 HTTP 响应。
func (s *MessageService) asyncAgentReply(convID, userID, agentID, content string, attachments []model.MessageAttachment, replyTo *string) {
	slog.Info("asyncAgentReply ENTER", "conversation_id", convID, "agent_id", agentID, "reply_to", stringValue(replyTo), "goroutine", "started")

	defer func() {
		if r := recover(); r != nil {
			slog.Warn("asyncAgentReply recovered", "panic", r)
		}
	}()

	slog.Info("asyncAgentReply CALL createAgentReply", "conversation_id", convID, "agent_id", agentID)

	s.broadcastAgentTyping(convID, true)
	defer s.broadcastAgentTyping(convID, false)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// 单聊 Agent 不需要群聊风格的 handoff 上下文：
	// Claude Code 通过 --session-id/--resume 自行维护对话历史，
	// 无需服务端额外发送历史摘要。
	//
	// 通过 direct reply chain 构建上下文：[Attachment, Blackboard, KB, AgentConfig]
	// 最终输出顺序：agentConfig + kb + blackboard + attach（与重构前完全一致）。
	// 仅当 orchSvc 可用且 agent 解析成功时才走完整链；否则只走 attachment 段。
	var contextMessages string
	if s.orchSvc != nil {
		agent, err := s.agentRepo.GetByID(ctx, agentID)
		if err == nil && agent != nil {
			contextMessages = s.orchSvc.DirectReplyChain().Build(ctx, ContextInput{
				ConvID:      convID,
				UserID:      userID,
				Agent:       agent,
				Content:     content,
				Attachments: attachments,
			})
		} else {
			// agent 解析失败时仅注入附件段（保持原降级行为，不注入 agent config / blackboard / kb）
			// 直接调纯函数 BuildAttachmentText（与 AttachmentBuilder 共享同一实现）
			contextMessages = BuildAttachmentText(ctx, attachments, s.orchSvc.uploadDir, attachmentTextMaxRunes)
		}
	}

	agentMsg, err := s.createAgentReply(ctx, convID, userID, agentID, content, contextMessages, replyTo)
	if err != nil {
		slog.Warn("agent reply failed", "convID", convID, "agentID", agentID, "error", err)
		s.postAgentFailure(ctx, convID, userID, "Agent 调用失败："+shortAgentError(err), replyTo)
		return
	}
	s.postPersist(convID, userID, agentMsg)
}

func (s *MessageService) postAgentFailure(ctx context.Context, convID, userID, content string, replyTo *string) {
	meta, _ := json.Marshal(map[string]string{"agent_name": "AgentHub"})
	msg, err := s.msgRepo.Create(ctx, convID, "assistant", content, string(meta), nil, replyTo, nil, nil)
	if err != nil {
		slog.Warn("create agent failure message failed", "convID", convID, "error", err)
		return
	}
	s.postPersist(convID, userID, msg)
}

// broadcastStreamingTerminal 已删除：streaming 失败终态广播统一走 streaming_pipeline.go
// 的顶层 BroadcastStreamingTerminal 函数（单聊 createAgentReply 和群聊 Dispatcher 共用）。

func shortAgentError(err error) string {
	if err == nil {
		return "未知错误"
	}
	text := err.Error()
	if strings.Contains(text, "You've hit your usage limit") {
		return "Codex CLI 当前额度已用尽，请到 Codex 设置查看 usage，或等待额度恢复。"
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
		text = strings.TrimSpace(lines[0])
	}
	if len([]rune(text)) > 240 {
		runes := []rune(text)
		text = string(runes[:240]) + "..."
	}
	return text
}

func (s *MessageService) broadcastAgentTyping(convID string, typing bool) {
	if s.notifier == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	memberIDs, err := s.convRepo.ListMemberIDs(ctx, convID)
	if err != nil || len(memberIDs) == 0 {
		return
	}

	eventType := "agent.typing_stop"
	if typing {
		eventType = "agent.typing_start"
	}

	s.notifier.PushCustomEvent(convID, memberIDs, eventType, map[string]string{
		"conversation_id": convID,
	})
}

func (s *MessageService) resolveAgentConversationAgentID(ctx context.Context, convID, userID string) string {
	agents, err := s.convRepo.ListAgents(ctx, convID, userID)
	if err != nil || len(agents) == 0 {
		if err != nil {
			slog.Warn("resolve agent conversation agent failed", "conversation_id", convID, "error", err)
		}
		return ""
	}
	return strings.TrimSpace(agents[0].AgentID)
}

// snapshotBlocksJSON / deleteStreamingBuffer 已删除：
//  - snapshotBlocksJSON 被 dispatcher.go 的包级 snapshotBlocksJSONFromBuffer 取代
//    （createAgentReply 成功路径已改用该包级函数，与 dispatcher 共用）
//  - deleteStreamingBuffer 无调用方：createAgentReply 用 inline s.streamingBuffer.Delete，
//    dispatcher 用 buf.Delete 类型断言
