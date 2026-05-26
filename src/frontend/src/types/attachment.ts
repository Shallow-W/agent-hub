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

export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
