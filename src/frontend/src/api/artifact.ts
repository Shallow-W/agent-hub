import { get, post } from './client';
import type { Artifact, ArtifactType } from '@/types/message';

/** 创建产物新版本的请求体，字段对齐后端 CreateVersionRequest。 */
export interface CreateArtifactVersionPayload {
  content: string;
  type?: ArtifactType;
  language?: string;
  filename?: string;
  title?: string;
  url?: string;
}

/** 列出某血缘根的全部版本（按 version 升序）。 */
export async function listArtifactVersions(rootId: string): Promise<Artifact[]> {
  return get<Artifact[]>(`/api/artifacts/${rootId}/versions`);
}

/** 为某血缘根创建新版本，返回新版本产物。 */
export async function createArtifactVersion(
  rootId: string,
  payload: CreateArtifactVersionPayload,
): Promise<Artifact> {
  return post<Artifact>(`/api/artifacts/${rootId}/versions`, payload);
}
