import React, { useEffect, useState, useRef } from 'react';
import { Spin } from 'antd';
import {
  FileOutlined,
  FileImageOutlined,
  FilePdfOutlined,
  FileTextOutlined,
  DownloadOutlined,
} from '@ant-design/icons';
import type { KnowledgeFile } from '@/types/knowledge';
import { getKnowledgeFileText, getKnowledgeFileUrl } from '@/api/knowledge';
import { getAuthHeaders } from '@/api/client';
import styles from './KnowledgeFilePreview.module.css';

interface KnowledgeFilePreviewProps {
  file: KnowledgeFile;
  kbId: string;
}

type PreviewState = 'loading' | 'loaded' | 'error';

function isImageMime(mime: string): boolean {
  return mime.startsWith('image/');
}

function isPDFMime(mime: string): boolean {
  return mime === 'application/pdf';
}

function isTextMime(mime: string): boolean {
  return (
    mime === 'text/plain' ||
    mime === 'text/markdown' ||
    mime === 'text/csv' ||
    mime === 'text/html' ||
    mime === 'application/json'
  );
}

function shouldUseTextPreview(file: KnowledgeFile): boolean {
  return file.preview_type === 'text' && !isImageMime(file.mime_type) && !isPDFMime(file.mime_type);
}

function shouldTryServerTextPreview(file: KnowledgeFile): boolean {
  return !isImageMime(file.mime_type) && !isPDFMime(file.mime_type) && file.preview_type !== 'too_large';
}

/** 通过认证 fetch 获取文件 blob，创建带 token 的 blob URL */
async function fetchFileBlob(kbId: string, file: KnowledgeFile): Promise<{ url: string; blob: Blob } | null> {
  const url = getKnowledgeFileUrl(kbId, file);
  const res = await fetch(url, {
    headers: getAuthHeaders(),
  });
  if (!res.ok) {
    throw new Error(`获取文件失败 (${res.status})`);
  }
  const blob = await res.blob();
  const blobUrl = URL.createObjectURL(blob);
  return { url: blobUrl, blob };
}

const KnowledgeFilePreview: React.FC<KnowledgeFilePreviewProps> = ({ file, kbId }) => {
  const [state, setState] = useState<PreviewState>('loading');
  const [blobUrl, setBlobUrl] = useState<string | null>(null);
  const [textContent, setTextContent] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const blobUrlRef = useRef<string | null>(null);

  // 清理 blob URL
  useEffect(() => {
    return () => {
      if (blobUrlRef.current) {
        URL.revokeObjectURL(blobUrlRef.current);
        blobUrlRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    // 重置状态
    setState('loading');
    setBlobUrl(null);
    setTextContent(null);
    setError(null);

    let cancelled = false;

    (async () => {
      try {
        if (shouldUseTextPreview(file)) {
          const text = file.preview_text || (await getKnowledgeFileText(kbId, file.id)).text;
          if (!cancelled) {
            setTextContent(text);
            setState('loaded');
          }
        } else if (isTextMime(file.mime_type) || shouldTryServerTextPreview(file)) {
          const textResult = await getKnowledgeFileText(kbId, file.id);
          if (!cancelled) {
            setTextContent(textResult.preview_type === 'text' ? textResult.text : null);
            setState('loaded');
          }
        } else if (isImageMime(file.mime_type) || isPDFMime(file.mime_type)) {
          // 图片/PDF：fetch 为 blob
          const result = await fetchFileBlob(kbId, file);
          if (!cancelled && result) {
            blobUrlRef.current = result.url;
            setBlobUrl(result.url);
            setState('loaded');
          }
        } else {
          // 其他文件：只显示元信息
          if (!cancelled) setState('loaded');
        }
      } catch (err) {
        if (!cancelled) {
          setError((err as Error).message);
          setState('error');
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [file.id, file.mime_type, file.preview_text, file.preview_type, kbId]);

  const handleDownload = async () => {
    try {
      const url = getKnowledgeFileUrl(kbId, file);
      const res = await fetch(url, { headers: getAuthHeaders() });
      if (!res.ok) throw new Error(`下载失败 (${res.status})`);
      const blob = await res.blob();
      const blobUrl = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = blobUrl;
      a.download = file.filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(blobUrl);
    } catch {
      // 静默失败，用户可通过右键保存
    }
  };

  const renderPreview = () => {
    if (state === 'loading') {
      return (
        <div className={styles.centerWrap}>
          <Spin />
          <span className={styles.loadingText}>加载预览中…</span>
        </div>
      );
    }

    if (state === 'error') {
      return (
        <div className={styles.centerWrap}>
          <FileOutlined className={styles.errorIcon} />
          <span className={styles.errorText}>{error || '加载失败'}</span>
        </div>
      );
    }

    if (isImageMime(file.mime_type) && blobUrl) {
      return (
        <div className={styles.imageWrap}>
          <img src={blobUrl} alt={file.filename} className={styles.previewImage} />
        </div>
      );
    }

    if (isPDFMime(file.mime_type) && blobUrl) {
      return (
        <div className={styles.pdfWrap}>
          <iframe src={blobUrl} className={styles.pdfFrame} title={file.filename} />
        </div>
      );
    }

    if ((shouldUseTextPreview(file) || isTextMime(file.mime_type)) && textContent !== null) {
      return (
        <div className={styles.textWrap}>
          <pre className={styles.textContent}>{textContent}</pre>
        </div>
      );
    }

    if (file.preview_type === 'too_large') {
      return (
        <div className={styles.centerWrap}>
          <FileOutlined className={styles.unsupportedIcon} />
          <span className={styles.unsupportedText}>文件过大，无法生成文本预览</span>
        </div>
      );
    }

    // 不支持预览的文件类型
    return (
      <div className={styles.centerWrap}>
        <FileOutlined className={styles.unsupportedIcon} />
        <span className={styles.unsupportedText}>此文件类型不支持预览</span>
      </div>
    );
  };

  const fileTypeIcon = isImageMime(file.mime_type) ? (
    <FileImageOutlined />
  ) : isPDFMime(file.mime_type) ? (
    <FilePdfOutlined />
  ) : shouldUseTextPreview(file) || isTextMime(file.mime_type) ? (
    <FileTextOutlined />
  ) : (
    <FileOutlined />
  );

  return (
    <div className={styles.container}>
      {/* 文件信息头部 */}
      <div className={styles.fileHeader}>
        <div className={styles.fileInfo}>
          <span className={styles.fileIcon}>{fileTypeIcon}</span>
          <span className={styles.fileName}>{file.filename}</span>
        </div>
        <button
          className={styles.downloadBtn}
          type="button"
          aria-label="下载文件"
          onClick={handleDownload}
        >
          <DownloadOutlined />
        </button>
      </div>

      {/* 预览区域 */}
      <div className={styles.previewArea}>{renderPreview()}</div>
    </div>
  );
};

export default KnowledgeFilePreview;
