import React from 'react';
import type { AttachmentPayload } from '@/types/attachment';
import { isImageAttachment, formatFileSize } from '@/types/attachment';
import { CloseCircleFilled, FilePdfOutlined } from '@ant-design/icons';
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
  if (isImageAttachment(item.file.type)) {
    const url = URL.createObjectURL(item.file);
    return <img src={url} alt={item.file.name} className={styles.thumb} onLoad={() => URL.revokeObjectURL(url)} />;
  }

  return (
    <div className={styles.fileIcon}>
      <FilePdfOutlined style={{ fontSize: 24, color: '#cf1322' }} />
      <span className={styles.fileSize}>{formatFileSize(item.file.size)}</span>
    </div>
  );
};
