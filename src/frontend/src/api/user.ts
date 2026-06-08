import { put } from './client';
import type { User } from '@/types/auth';

/** 更新当前用户头像，返回最新用户资料。 */
export async function updateUserAvatar(avatar: string): Promise<User> {
  return put<User>('/api/users/me', { avatar });
}

/** 更新当前用户名，返回最新用户资料。 */
export async function updateUsername(username: string): Promise<User> {
  return put<User>('/api/users/me', { username });
}
