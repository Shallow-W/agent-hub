export type ConversationType = 'single' | 'group';

export interface Conversation {
  id: string;
  user_id: string;
  type: ConversationType;
  title: string;
  pinned: boolean;
  created_at: string;
  updated_at: string;
}
