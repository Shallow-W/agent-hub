import React, { useEffect, useState } from 'react';
import type { AttachmentPayload } from '@/types/attachment';
import { formatFileSize, isImageAttachment, isPDFAttachment, isPptxAttachment, isWordAttachment } from '@/types/attachment';
import {
  CloseCircleFilled,
  FileOutlined,
  FilePdfOutlined,
  FilePptOutlined,
  FileWordOutlined,
} from '@ant-design/icons';
import { Spin } from 'antd';
import styles from './AttachmentPreview.module.css';

export interface PendingAttachment {
  uid: string;
  file: File;
  status: 'uploading' | 'done' | 'error';
  payload?: AttachmentPayload;
  error?: string;
}

interface Props {
  items: PendingAttachment[];
  onRemove: (uid: string) => void;
}

export const AttachmentPreview: React.FC<Props> = ({ items, onRemove }) => {
  if (!items.length) return null;

  return (
    <div className={styles.container}>
      {items.map((item) => (
        <div key={item.uid} className={styles.item}>
          {item.status === 'uploading' && (
            <div className={styles.overlay}>
              <Spin size="small" />
            </div>
          )}
          {item.status === 'error' && (
            <div className={`${styles.overlay} ${styles.overlayError}`}>
              <span className={styles.errorText}>失败</span>
            </div>
          )}
          <PreviewContent item={item} />
          <button
            type="button"
            className={styles.removeBtn}
            onClick={() => onRemove(item.uid)}
          >
            <CloseCircleFilled />
          </button>
          <div className={styles.fileName}>{item.file.name}</div>
        </div>
      ))}
    </div>
  );
};

const PreviewContent: React.FC<{ item: PendingAttachment }> = ({ item }) => {
  const [url, setUrl] = useState<string | null>(null);

  useEffect(() => {
    if (!isImageAttachment(item.file.type)) return;
    const blobUrl = URL.createObjectURL(item.file);
    setUrl(blobUrl);
    return () => URL.revokeObjectURL(blobUrl);
  }, [item.file]);

  if (isImageAttachment(item.file.type) && url) {
    return <img src={url} alt={item.file.name} className={styles.thumb} />;
  }

  return (
    <div className={styles.fileIcon}>
      <UploadFileIcon file={item.file} />
      <span className={styles.fileSize}>{formatFileSize(item.file.size)}</span>
    </div>
  );
};

const UploadFileIcon: React.FC<{ file: File }> = ({ file }) => {
  if (isPDFAttachment(file.type)) return <FilePdfOutlined className={styles.pdfIcon} />;
  if (isWordAttachment(file.type, file.name)) return <FileWordOutlined className={styles.wordIcon} />;
  if (isPptxAttachment(file.type, file.name)) return <FilePptOutlined className={styles.pptIcon} />;
  return <FileOutlined className={styles.genericIcon} />;
};
