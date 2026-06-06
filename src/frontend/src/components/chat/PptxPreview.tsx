import React, { useEffect, useRef, useState } from 'react';
import { init as initPptxPreviewer } from 'pptx-preview';
import { Button } from 'antd';
import { DownloadOutlined, FileExclamationOutlined, LoadingOutlined } from '@ant-design/icons';
import styles from './PptxPreview.module.css';

interface PptxPreviewProps {
  fileUrl: string;
  fileName: string;
  previewUrl?: string;
}

type LoadState = 'loading' | 'pdf' | 'dom' | 'error';

const MAX_SLIDE_WIDTH = 960;

const PptxPreview: React.FC<PptxPreviewProps> = ({ fileUrl, fileName, previewUrl }) => {
  const measureRef = useRef<HTMLDivElement>(null);
  const hostRef = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<LoadState>('loading');
  const [pdfUrl, setPdfUrl] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    let objectUrl: string | null = null;
    let previewer: ReturnType<typeof initPptxPreviewer> | null = null;

    async function tryRenderPDF(): Promise<boolean> {
      if (!previewUrl) return false;
      const resp = await fetch(previewUrl);
      if (!resp.ok) return false;
      const blob = await resp.blob();
      if (blob.type && !blob.type.includes('pdf')) return false;
      objectUrl = URL.createObjectURL(blob);
      if (cancelled) return true;
      setPdfUrl(objectUrl);
      setState('pdf');
      return true;
    }

    async function renderDomPreview() {
      const host = hostRef.current;
      const measure = measureRef.current;
      if (!host || !measure) return;

      const resp = await fetch(fileUrl);
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const buffer = await resp.arrayBuffer();
      if (cancelled || !hostRef.current || !measureRef.current) return;

      const available = measureRef.current.clientWidth;
      const width = Math.min(available > 0 ? available : MAX_SLIDE_WIDTH, MAX_SLIDE_WIDTH);
      previewer = initPptxPreviewer(hostRef.current, {
        width,
        height: Math.round(width * (3 / 4)),
        mode: 'list',
      });
      await previewer.preview(buffer);
      if (cancelled) return;

      const realW = previewer.pptx?.width ?? 0;
      const realH = previewer.pptx?.height ?? 0;
      const wrapper = previewer.wrapper as HTMLElement | undefined;
      if (wrapper) {
        wrapper.style.setProperty('height', 'auto');
        wrapper.style.setProperty('overflow-y', 'visible');
        wrapper.style.setProperty('background', 'transparent');
        if (realW > 0 && realH > 0) {
          const slideH = Math.round(width * (realH / realW));
          wrapper.querySelectorAll<HTMLElement>('.pptx-preview-slide-wrapper').forEach((slide) => {
            slide.style.setProperty('height', `${slideH}px`);
            slide.style.setProperty('width', `${width}px`);
          });
        }
      }
      setState('dom');
    }

    async function render() {
      setState('loading');
      setPdfUrl(null);
      try {
        const renderedPDF = await tryRenderPDF().catch(() => false);
        if (cancelled || renderedPDF) return;
        await renderDomPreview();
      } catch {
        if (!cancelled) setState('error');
      }
    }

    render();

    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
      try {
        previewer?.destroy();
      } catch {
        // Ignore partial-render cleanup errors from pptx-preview.
      }
    };
  }, [fileUrl, previewUrl]);

  return (
    <div ref={measureRef} className={styles.measure}>
      {pdfUrl && state === 'pdf' ? (
        <iframe src={pdfUrl} title={fileName} className={styles.pdfFrame} />
      ) : (
        <div ref={hostRef} className={styles.host} />
      )}
      {state === 'loading' && (
        <div className={styles.overlay}>
          <LoadingOutlined className={styles.spinner} spin />
          <span>正在解析幻灯片…</span>
        </div>
      )}
      {state === 'error' && (
        <div className={styles.overlay}>
          <FileExclamationOutlined className={styles.errorIcon} />
          <span className={styles.errorText}>无法预览，请下载查看。</span>
          <Button
            type="primary"
            icon={<DownloadOutlined />}
            href={fileUrl}
            download={fileName}
            className={styles.downloadBtn}
          >
            下载文件
          </Button>
        </div>
      )}
    </div>
  );
};

export default PptxPreview;
