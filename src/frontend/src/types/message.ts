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
    reply_to?: string | null;
    reply_to_message?: ReplyToPreview | null;
    agent_id?: string;
    agent_status?: string;
  };
}
