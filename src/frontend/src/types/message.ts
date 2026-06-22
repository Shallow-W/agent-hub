import type { MessageAttachment } from './attachment';
import type { AttachmentPayload } from './attachment';
import type { Deployment } from './deployment';

export type MessageRole = 'user' | 'assistant' | 'system';

export interface ReplyToPreview {
  id: string;
  content: string;
  sender_id?: string;
  username?: string;
  deleted_at?: string | null;
}

/**
 * 结构化产物：从 Agent 回复中提取的代码 / 网页等一等对象。
 * 字段名严格对齐后端 model.Artifact 与 daemon 上行 JSON（三层对齐真源）。
 * `artifacts_json` 字段不动（只放 agent meta），产物经独立 artifacts 表随消息下发。
 */
export type ArtifactType = 'code' | 'webpage' | 'document' | 'file';

export interface Artifact {
  id?: string;
  message_id?: string;
  /** 版本血缘根（v1 的 id），同一逻辑产物各版本共享；用于版本历史/Diff */
  root_id?: string;
  version: number;
  type: ArtifactType;
  language?: string;
  filename?: string;
  title?: string;
  url?: string;
  content?: string;
  created_at?: string;
}

/** 流式消息 block 类型——与后端 daemon AgentEvent kind 对齐。 */
export type BlockKind = 'text' | 'thinking' | 'tool_use' | 'tool_result' | 'error';

/** 单个累积 block（同 kind 连续 delta 聚合成一个 block）。 */
export interface MessageBlock {
  /** delta event 在 message 内的序号，用于 React key 与排序 */
  index: number;
  kind: BlockKind;
  /** 累积的内容：text/thinking/tool_use 入参/tool_result 输出/error 消息 */
  text: string;
  /** kind=tool_use 的工具名（如 "Read"） */
  tool_name?: string;
  /** kind=tool_use 的 tool_use_id（与 tool_result 对齐用，可选） */
  tool_use_id?: string;
  /** kind=tool_result / error 时为 true */
  is_error?: boolean;
}

/** 消息生命周期状态——空值视为 complete（向后兼容旧消息）。 */
export type MessageStatus = 'streaming' | 'complete' | 'error' | 'canceled';

export interface Message {
  id: string;
  conversation_id: string;
  role: MessageRole;
  content: string;
  artifacts_json: string | null;
  created_at: string;
  sender_id?: string;
  username?: string;
  pinned?: boolean;
  attachments?: MessageAttachment[];
  artifacts?: Artifact[];
  reply_to?: string | null;
  reply_to_message?: ReplyToPreview | null;
  mentions?: string[];
  cards_json?: string;
  cards?: import('./card').InteractiveCard[];
  /** 流式累积 / 持久化的 block 列表。存在时优先于 content 渲染。 */
  blocks_json?: string;
  blocks?: MessageBlock[];
  /** streaming / complete / error / canceled；空值视为 complete */
  status?: MessageStatus;
  /** 流式期间的 daemon task_id——StopButton 取消时回传后端。终态后可清理。 */
  task_id?: string;
}

export interface PinnedMessage {
  id: string;
  conversation_id: string;
  message_id: string;
  role: MessageRole;
  content: string;
  artifacts_json?: string | null;
  sender_id?: string;
  username?: string;
  message_created_at: string;
  pinned_by: string;
  pinned_by_name?: string;
  pinned_at: string;
}

export interface ConversationBlackboard {
  conversation_id: string;
  manual_context: string;
  updated_by?: string | null;
  updated_at: string;
}

export type OptimisticStatus = 'sending' | 'failed';

export interface OptimisticMessage extends Message {
  optimistic: true;
  optimisticStatus: OptimisticStatus;
  /** Original attachment payloads used when sending, for retry */
  pendingAttachments?: AttachmentPayload[];
  pendingAgentId?: string;
}

export type DisplayMessage = Message | OptimisticMessage;

export interface SendMessageResult {
  user_message: Message;
  agent_message?: Message;
}

export interface MessageArtifacts {
  agent_id?: string;
  agent_name?: string;
  cli_tool?: string;
  /** 聊天「部署」指令回执：存在时前端在该消息内联渲染部署状态卡片 */
  deployment?: Deployment;
}

/** daemon 上报的单个流式事件——字段对齐后端 stream-json AgentEvent 透传。 */
export interface AgentEvent {
  type: 'text' | 'thinking' | 'tool_use' | 'tool_result' | 'turn_end' | 'error';
  /** text/thinking 内容片段 */
  content?: string;
  /** tool_use：工具名（首次 delta 非空，后续 input_json_delta 为空字符串） */
  tool?: string;
  /** tool_use：工具调用的唯一 id（可与 tool_result 对齐） */
  tool_use_id?: string;
  /** tool_result：工具输出 */
  output?: string;
  /** tool_result/error：是否为错误 */
  isError?: boolean;
  /** turn_end：最终汇总文本（可选） */
  result?: string;
  /** error：错误原因 */
  message?: string;
}

export interface StreamMessage {
  type: 'message.streaming' | 'message.complete' | 'agent.status' | 'user.typing_start' | 'user.typing_stop' | 'agent.typing_start' | 'agent.typing_stop' | 'message.recall' | 'task.changed' | 'conversation.role_changed' | 'error';
  data: {
    conversationId?: string;
    conversation_id?: string;
    messageId?: string;
    message_id?: string;
    id?: string;
    content?: string;
    role?: MessageRole;
    artifacts_json?: string | null;
    created_at?: string;
    sender_id?: string;
    username?: string;
    done?: boolean;
    agentId?: string;
    status?: 'thinking' | 'running' | 'idle';
    code?: string;
    message?: string;
    userId?: string;
    attachments?: MessageAttachment[];
    artifacts?: Artifact[];
    reply_to?: string | null;
    reply_to_message?: ReplyToPreview | null;
    agent_id?: string;
    agent_status?: string;
    /** 交互式卡片——message.complete 推送时携带，免去刷新页面即可渲染 */
    cards_json?: string;
    cards?: import('./card').InteractiveCard[];
    /** conversation.role_changed 事件：触发变更的用户 ID */
    actor_id?: string;
    /** conversation.role_changed 事件：被降级的旧 Orchestrator Agent ID（可选） */
    demoted_agent_id?: string;
    /** message.streaming：本批次 daemon 累积的 AgentEvent[]（透传，前端按 kind 渲染） */
    deltas?: AgentEvent[];
    /** message.streaming：关联的 task_id（用于 cancel 等场景） */
    task_id?: string;
  };
}
