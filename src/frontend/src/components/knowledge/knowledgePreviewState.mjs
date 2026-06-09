export function isImageMime(mime = '') {
  return mime.startsWith('image/');
}

export function isPDFMime(mime = '') {
  return mime === 'application/pdf';
}

export function isTextMime(mime = '') {
  return (
    mime === 'text/plain' ||
    mime === 'text/markdown' ||
    mime === 'text/csv' ||
    mime === 'text/html' ||
    mime === 'application/json'
  );
}

export function shouldUseTextPreview(file) {
  return file.preview_type === 'text' && !isImageMime(file.mime_type) && !isPDFMime(file.mime_type);
}

export function shouldTryServerTextPreview(file) {
  return !isImageMime(file.mime_type) && !isPDFMime(file.mime_type) && file.preview_type !== 'too_large';
}

export function buildKnowledgePreviewModel({ file, blobUrl = null, textContent = null }) {
  if (isImageMime(file.mime_type) && blobUrl) {
    return { kind: 'image', icon: 'image', url: blobUrl };
  }
  if (isPDFMime(file.mime_type) && blobUrl) {
    return { kind: 'pdf', icon: 'pdf', url: blobUrl };
  }
  if (textContent !== null && (shouldUseTextPreview(file) || isTextMime(file.mime_type) || shouldTryServerTextPreview(file))) {
    return { kind: 'text', icon: 'text', text: textContent };
  }
  if (file.preview_type === 'too_large') {
    return { kind: 'too_large', icon: 'file' };
  }
  return { kind: 'unsupported', icon: 'file' };
}

export function syncSelectedKnowledgeFile({ selectedFile, selectedKbId, knowledgeBases }) {
  if (!selectedFile || !selectedKbId) {
    return { selectedFile: null, selectedKbId: null };
  }
  const kb = knowledgeBases.find((item) => item.id === selectedKbId);
  const file = kb?.files?.find((item) => item.id === selectedFile.id) ?? null;
  if (!file) {
    return { selectedFile: null, selectedKbId: null };
  }
  return { selectedFile: file, selectedKbId };
}
