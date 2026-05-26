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

  const json = await res.json();
  if (!res.ok || json.code !== 0) {
    throw new Error(json.message || '上传失败');
  }
  return json.data;
}
