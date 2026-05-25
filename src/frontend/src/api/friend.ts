import { get, post } from './client';
import type { Friend, FriendRequest } from '@/types/friend';

export const sendFriendRequest = (username: string) =>
  post<Friend>('/friends/request', { username });

export const acceptFriendRequest = (id: string) =>
  post<Friend>(`/friends/${id}/accept`, {});

export const rejectFriendRequest = (id: string) =>
  post<Friend>(`/friends/${id}/reject`, {});

export const listFriends = () =>
  get<Friend[]>('/friends');

export const listPendingRequests = () =>
  get<FriendRequest[]>('/friends/pending');
