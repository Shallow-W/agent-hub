import { post, get, del, put } from './client';
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

export interface GroupInfo {
  conversation: {
    id: string;
    title: string;
    type: string;
    created_at: string;
    updated_at: string;
    member_count?: number;
  };
  members: GroupMember[];
}

export const getGroupInfo = (groupId: string) =>
  get<GroupInfo>(`/api/groups/${groupId}`);

export const leaveGroup = (groupId: string) =>
  post<void>(`/api/groups/${groupId}/leave`);

export const dissolveGroup = (groupId: string) =>
  post<void>(`/api/groups/${groupId}/dissolve`);

export const changeMemberRole = (groupId: string, memberId: string, role: string) =>
  put<void>(`/api/groups/${groupId}/members/${memberId}/role`, { role });
