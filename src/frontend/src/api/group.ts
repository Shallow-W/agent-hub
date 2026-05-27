import { post, get, del } from './client';
import type { Conversation } from '@/types/conversation';
import type { GroupMember } from '@/types/group';

export const createGroup = (data: { name: string; member_ids: string[] }) =>
  post<Conversation>('/api/groups', data);

export const addGroupMember = (groupId: string, data: { user_id: string; role: string }) =>
  post<void>(`/api/groups/${groupId}/members`, data);

export const removeGroupMember = (groupId: string, userId: string) =>
  del<void>(`/api/groups/${groupId}/members/${userId}`);

export const getGroupMembers = (groupId: string) =>
  get<GroupMember[]>(`/api/groups/${groupId}/members`);

export const leaveGroup = (groupId: string) =>
  post<void>(`/api/groups/${groupId}/leave`);
