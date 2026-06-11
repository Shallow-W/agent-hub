import type { ConversationAgentRole } from '@/types/role';

export type ConversationType = 'single' | 'group' | 'agent';

export interface Conversation {
  id: string;
  user_id: string;
  type: ConversationType;
  title: string;
  pinned: boolean;
  created_at: string;
  updated_at: string;
  peer_id?: string;
  peer_name?: string;
  last_message?: string;
  member_count?: number;
  avatar?: string;
  archived_at?: string | null;
}

export interface ConversationAgent {
  id: string;
  conversation_id: string;
  agent_id: string;
  added_by: string;
  role: ConversationAgentRole;
  joined_at: string;
  name: string;
  type: string;
  cli_tool: string;
  avatar: string;
  source: string;
  status: string;
  version: string;
  machine_id?: string;
  machine_name: string;
  last_seen_at?: string;
  capabilities_json: string;
  custom_skills?: string;
}
