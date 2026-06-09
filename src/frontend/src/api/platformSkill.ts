import { del, get, post, put } from './client';
import type { PlatformSkill, PlatformSkillRequest } from '@/types/agent';

const BASE = '/api/platform-skills';

export async function getPlatformSkills(): Promise<PlatformSkill[]> {
  const skills = await get<PlatformSkill[] | null>(BASE);
  return skills ?? [];
}

export async function createPlatformSkill(body: PlatformSkillRequest): Promise<PlatformSkill> {
  return post<PlatformSkill>(BASE, body);
}

export async function updatePlatformSkill(id: string, body: PlatformSkillRequest): Promise<PlatformSkill> {
  return put<PlatformSkill>(`${BASE}/${id}`, body);
}

export async function deletePlatformSkill(id: string): Promise<void> {
  return del<void>(`${BASE}/${id}`);
}
