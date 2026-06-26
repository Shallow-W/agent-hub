export type FileViewMode = 'preview' | 'source';

export function isHtmlPreviewFile(filepath: string): boolean {
  const normalized = filepath.trim().toLowerCase();
  return normalized.endsWith('.html') || normalized.endsWith('.htm');
}

export function defaultFileViewMode(filepath: string): FileViewMode {
  return isHtmlPreviewFile(filepath) ? 'preview' : 'source';
}
