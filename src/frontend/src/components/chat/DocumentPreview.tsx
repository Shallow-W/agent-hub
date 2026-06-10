import React, { useEffect, useState } from 'react';
import { Button } from 'antd';
import { DownloadOutlined, FileExclamationOutlined, LoadingOutlined } from '@ant-design/icons';
import styles from './DocumentPreview.module.css';

interface DocumentPreviewProps {
  fileUrl: string;
  fileName: string;
  previewUrl: string;
}

type LoadState = 'loading' | 'ready' | 'error';

const DocumentPreview: React.FC<DocumentPreviewProps> = ({ fileUrl, fileName, previewUrl }) => {
  const [state, setState] = useState<LoadState>('loading');
  const [pdfUrl, setPdfUrl] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    let objectUrl: string | null = null;

    async function render() {
      setState('loading');
      setPdfUrl(null);
      try {
        const resp = await fetch(previewUrl);
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const blob = await resp.blob();
        if (blob.type && !blob.type.includes('pdf')) throw new Error(`Unexpected type ${blob.type}`);
        objectUrl = URL.createObjectURL(blob);
        if (cancelled) return;
        setPdfUrl(objectUrl);
        setState('ready');
      } catch {
        if (!cancelled) setState('error');
      }
    }

    render();

    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [previewUrl]);

  return (
    <div className={styles.measure}>
      {pdfUrl && state === 'ready' && (
        <iframe src={pdfUrl} title={fileName} className={styles.pdfFrame} />
      )}
      {state === 'loading' && (
        <div className={styles.overlay}>
          <LoadingOutlined className={styles.spinner} spin />
          <span>正在解析文件...</span>
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

export default DocumentPreview;
