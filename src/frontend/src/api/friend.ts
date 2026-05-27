import { get, post } from './client';
import type { Friend, FriendRequest } from '@/types/friend';
import type { User } from '@/types/auth';

export const sendFriendRequest = (username: string) =>
  post<Friend>('/api/friends/request', { username });

export const acceptFriendRequest = (id: string) =>
  post<void>(`/api/friends/${id}/accept`, {});

export const rejectFriendRequest = (id: string) =>
  post<void>(`/api/friends/${id}/reject`, {});

export const listFriends = () =>
  get<Friend[]>('/api/friends');

export const listPendingRequests = () =>
  get<FriendRequest[]>('/api/friends/pending');

export const searchUsers = (username: string) =>
  get<User[]>(`/api/friends/search?username=${encodeURIComponent(username)}`);
