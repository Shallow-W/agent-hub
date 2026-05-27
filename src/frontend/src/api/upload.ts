import type { AttachmentPayload } from '@/types/attachment';

export async function uploadFile(file: File): Promise<AttachmentPayload> {
  const token = localStorage.getItem('agenthub_token');
  const formData = new FormData();
  formData.append('file', file);

  const res = await fetch('/api/upload', {
    method: 'POST',
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: formData,
  });

  let json: { code?: number; message?: string; data?: AttachmentPayload };
  try {
    json = await res.json();
  } catch {
    throw new Error(`上传失败：服务器返回了非预期响应 (${res.status})`);
  }
  if (!res.ok || json.code !== 0) {
    throw new Error(json.message || '上传失败');
  }
  return json.data!;
}
