import type { AttachmentPayload } from '@/types/attachment';
import { getAuthHeaders, ApiError } from './client';
import { apiURL } from './runtime';

export async function uploadFile(file: File): Promise<AttachmentPayload> {
  const formData = new FormData();
  formData.append('file', file);

  const res = await fetch(apiURL('/api/upload'), {
    method: 'POST',
    headers: getAuthHeaders(),
    body: formData,
  });

  let json: { code?: number; message?: string; data?: AttachmentPayload };
  try {
    json = await res.json();
  } catch {
    throw new ApiError(res.status, 0, `上传失败：服务器返回了非预期响应 (${res.status})`);
  }
  if (!res.ok || json.code !== 0) {
    throw new ApiError(res.status, json.code ?? 0, json.message || '上传失败');
  }
  const data = json.data!;
  return {
    ...data,
    file_name: data.file_name?.trim() || file.name,
  };
}
