import type { KnowledgeBase, KnowledgeFile } from '@/types/knowledge';

export type KnowledgePreviewModel =
  | { kind: 'image'; icon: 'image'; url: string }
  | { kind: 'pdf'; icon: 'pdf'; url: string }
  | { kind: 'text'; icon: 'text'; text: string }
  | { kind: 'too_large'; icon: 'file' }
  | { kind: 'unsupported'; icon: 'file' };

export function isImageMime(mime?: string): boolean;
export function isPDFMime(mime?: string): boolean;
export function isTextMime(mime?: string): boolean;
export function shouldUseTextPreview(file: KnowledgeFile): boolean;
export function shouldTryServerTextPreview(file: KnowledgeFile): boolean;
export function buildKnowledgePreviewModel(args: {
  file: KnowledgeFile;
  blobUrl?: string | null;
  textContent?: string | null;
}): KnowledgePreviewModel;
export function syncSelectedKnowledgeFile(args: {
  selectedFile: KnowledgeFile | null;
  selectedKbId: string | null;
  knowledgeBases: Pick<KnowledgeBase, 'id' | 'files'>[];
}): { selectedFile: KnowledgeFile | null; selectedKbId: string | null };
