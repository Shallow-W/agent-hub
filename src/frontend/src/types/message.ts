export type MessageRole = 'user' | 'assistant' | 'system';

export interface Message {
  id: string;
  conversation_id: string;
  role: MessageRole;
  content: string;
  artifacts_json: string | null;
  created_at: string;
}

export type OptimisticStatus = 'sending' | 'failed';

export interface OptimisticMessage extends Message {
  optimistic: true;
  optimisticStatus: OptimisticStatus;
}

export type DisplayMessage = Message | OptimisticMessage;

export interface StreamMessage {
  type: 'message.streaming' | 'message.complete' | 'agent.status' | 'user.typing_start' | 'user.typing_stop' | 'error';
  data: {
    conversationId?: string;
    conversation_id?: string;
    messageId?: string;
    id?: string;
    content?: string;
    role?: MessageRole;
    artifacts_json?: string | null;
    created_at?: string;
    done?: boolean;
    agentId?: string;
    status?: 'thinking' | 'running' | 'idle';
    code?: string;
    message?: string;
    userId?: string;
  };
}
