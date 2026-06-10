import { del, get, post, put } from './client';

const BASE = '/api/user-templates';

export interface UserTemplate {
  id: string;
  user_id: string;
  type: 'tools' | 'skills';
  name: string;
  content: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export async function listUserTemplates(type: 'tools' | 'skills'): Promise<UserTemplate[]> {
  const list = await get<UserTemplate[] | null>(`${BASE}?type=${type}`);
  return list ?? [];
}

export async function createUserTemplate(body: {
  type: 'tools' | 'skills';
  name: string;
  content: Record<string, unknown>;
}): Promise<UserTemplate> {
  return post<UserTemplate>(BASE, body);
}

export async function updateUserTemplate(
  id: string,
  body: {
    type: 'tools' | 'skills';
    name: string;
    content: Record<string, unknown>;
  },
): Promise<UserTemplate> {
  return put<UserTemplate>(`${BASE}/${id}`, body);
}

export async function deleteUserTemplate(id: string): Promise<void> {
  return del<void>(`${BASE}/${id}`);
}
