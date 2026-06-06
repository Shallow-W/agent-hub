import type { MessageAttachment } from './attachment';
import type { AttachmentPayload } from './attachment';

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

export interface Message {
  id: string;
  conversation_id: string;
  role: MessageRole;
  content: string;
  artifacts_json: string | null;
  created_at: string;
  sender_id?: string;
  username?: string;
  attachments?: MessageAttachment[];
  artifacts?: Artifact[];
  reply_to?: string | null;
  reply_to_message?: ReplyToPreview | null;
  mentions?: string[];
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
}

export interface StreamMessage {
  type: 'message.streaming' | 'message.complete' | 'agent.status' | 'user.typing_start' | 'user.typing_stop' | 'agent.typing_start' | 'agent.typing_stop' | 'message.recall' | 'error';
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
  };
}
