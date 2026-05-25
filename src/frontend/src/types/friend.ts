export type FriendStatus = 'pending' | 'accepted' | 'rejected' | 'blocked';

export interface Friend {
  id: string;
  user_id: string;
  friend_id: string;
  status: FriendStatus;
  friend_name?: string;
  created_at: string;
  updated_at: string;
}

export interface FriendRequest {
  id: string;
  from_user_id: string;
  to_user_id: string;
  from_username?: string;
  to_username?: string;
  status: FriendStatus;
  created_at: string;
}
