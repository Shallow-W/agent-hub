/** 知识库可见性 */
export type KnowledgeVisibility = 'private' | 'public';

/** 知识库中的文件 */
export interface KnowledgeFile {
  id: string;
  filename: string;
  size: number;
  mime_type: string;
  uploaded_at: string;
}

/** 知识库 */
export interface KnowledgeBase {
  id: string;
  name: string;
  description?: string;
  visibility: KnowledgeVisibility;
  files: KnowledgeFile[];
  file_count: number;
  created_at: string;
  updated_at: string;
}

/** 群组中可用的知识库（含 owner 信息） */
export interface GroupKnowledgeBase {
  id: string;
  name: string;
  description?: string;
  visibility: KnowledgeVisibility;
  username: string;
  file_count: number;
  created_at: string;
  updated_at: string;
}
export interface CreateKnowledgeBaseRequest {
  name: string;
  description?: string;
  visibility?: KnowledgeVisibility;
}

/** 更新知识库请求 */
export interface UpdateKnowledgeBaseRequest {
  name?: string;
  description?: string;
  visibility?: KnowledgeVisibility;
}
