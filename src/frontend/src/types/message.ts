export type MessageRole = 'user' | 'assistant' | 'system';

export interface Message {
  id: string;
  conversation_id: string;
  role: MessageRole;
  content: string;
  artifacts_json: string | null;
  created_at: string;
}

export interface StreamMessage {
  type: 'message.streaming' | 'message.complete' | 'agent.status' | 'error';
  data: {
    conversationId?: string;
    messageId?: string;
    content?: string;
    done?: boolean;
    agentId?: string;
    status?: 'thinking' | 'running' | 'idle';
    code?: string;
    message?: string;
  };
}
