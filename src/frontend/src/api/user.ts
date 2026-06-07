import { put } from './client';
import type { User } from '@/types/auth';

/** 更新当前用户头像，返回最新用户资料。 */
export async function updateUserAvatar(avatar: string): Promise<User> {
  return put<User>('/api/users/me', { avatar });
}
