import { get, post, put, del, getAuthHeaders } from './client';
import type {
  KnowledgeBase,
  GroupKnowledgeBase,
  CreateKnowledgeBaseRequest,
  UpdateKnowledgeBaseRequest,
} from '@/types/knowledge';

const BASE = '/api/knowledge-bases';

/** 获取知识库列表 */
export async function getKnowledgeBases(): Promise<KnowledgeBase[]> {
  return await get<KnowledgeBase[]>(BASE);
}

/** 创建知识库 */
export async function createKnowledgeBase(req: CreateKnowledgeBaseRequest): Promise<KnowledgeBase> {
  return await post<KnowledgeBase>(BASE, req);
}

/** 更新知识库 */
export async function updateKnowledgeBase(
  id: string,
  req: UpdateKnowledgeBaseRequest,
): Promise<KnowledgeBase> {
  return await put<KnowledgeBase>(`${BASE}/${id}`, req);
}

/** 删除知识库 */
export async function deleteKnowledgeBase(id: string): Promise<void> {
  await del(`${BASE}/${id}`);
}

/** 上传文件到知识库 */
export async function uploadKnowledgeFile(kbId: string, file: File): Promise<void> {
  const formData = new FormData();
  formData.append('file', file);

  const res = await fetch(`${BASE}/${kbId}/files`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: formData,
  });

  const json = await res.json().catch(() => ({ message: '上传失败' }));
  if (!res.ok || json.code !== 0) {
    throw new Error(json.message || '上传失败');
  }
}

/** 删除知识库中的文件 */
export async function deleteKnowledgeFile(kbId: string, fileId: string): Promise<void> {
  await del(`${BASE}/${kbId}/files/${fileId}`);
}

/** 获取知识库文件预览/下载 URL */
export function getKnowledgeFileUrl(kbId: string, fileId: string): string {
  return `${BASE}/${kbId}/files/${fileId}/content`;
}

/** 获取群组中可用的知识库列表（自己的全部 + 其他成员的公开 KB） */
export async function getGroupKnowledgeBases(groupId: string): Promise<GroupKnowledgeBase[]> {
  return await get<GroupKnowledgeBase[]>(`${BASE}/group/${groupId}`);
}
