export interface AttachmentPayload {
  file_name: string;
  mime_type: string;
  file_size: number;
  file_path: string;
  thumbnail_path: string | null;
  width: number | null;
  height: number | null;
}

export interface MessageAttachment {
  id: string;
  message_id: string;
  file_name: string;
  mime_type: string;
  file_size: number;
  file_path: string;
  thumbnail_path: string | null;
  width: number | null;
  height: number | null;
  created_at: string;
}

export function isImageAttachment(mimeType: string): boolean {
  return mimeType.startsWith('image/');
}

export function isPDFAttachment(mimeType: string): boolean {
  return mimeType === 'application/pdf';
}

/**
 * 判断是否为 PowerPoint 演示文稿（.pptx / .ppt）。
 * mime 优先（pptx / ppt 的官方 mime），但上传链路 mime 可能不准，故用文件名后缀兜底。
 */
export function isPptxAttachment(mimeType: string, fileName: string): boolean {
  const ppMimes = new Set([
    'application/vnd.openxmlformats-officedocument.presentationml.presentation',
    'application/vnd.ms-powerpoint',
  ]);
  if (ppMimes.has(mimeType)) return true;
  return /\.pptx?$/i.test(fileName);
}

export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
