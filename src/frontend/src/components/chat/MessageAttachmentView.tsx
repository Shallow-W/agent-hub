import React, { Suspense, lazy, useState } from 'react';
import type { MessageAttachment } from '@/types/attachment';
import { isImageAttachment, isPDFAttachment, isPptxAttachment, formatFileSize } from '@/types/attachment';
import {
  FilePdfOutlined,
  FileOutlined,
  FilePptOutlined,
  DownloadOutlined,
  EyeOutlined,
} from '@ant-design/icons';
import { Modal } from 'antd';
import styles from './MessageAttachmentView.module.css';

// pptx-preview 较重，仅在用户点击“预览幻灯片”打开 Modal 时动态加载，避免进入首屏 bundle。
const PptxPreview = lazy(() => import('./PptxPreview'));

/** 将路径标准化为 URL 安全的正斜杠格式 */
function toUrlPath(p: string): string {
  const normalized = p.replace(/\\/g, '/');
  return normalized.startsWith('/') ? normalized : `/${normalized}`;
}

/** 构建带鉴权 token 的 API URL（用于 <img>/<a> 等无法带 header 的场景） */
function authUrl(path: string): string {
  const token = localStorage.getItem('agenthub_token');
  const sep = path.includes('?') ? '&' : '?';
  return token ? `${path}${sep}token=${encodeURIComponent(token)}` : path;
}

interface Props {
  attachments: MessageAttachment[];
}

export const MessageAttachmentView: React.FC<Props> = ({ attachments }) => {
  if (!attachments.length) return null;

  return (
    <div className={styles.container}>
      {attachments.map((att) =>
        isImageAttachment(att.mime_type) ? (
          <ImageAttachment key={att.id} attachment={att} />
        ) : isPDFAttachment(att.mime_type) ? (
          <PDFAttachment key={att.id} attachment={att} />
        ) : isPptxAttachment(att.mime_type, att.file_name) ? (
          <PptxAttachment key={att.id} attachment={att} />
        ) : (
          <GenericFileAttachment key={att.id} attachment={att} />
        ),
      )}
    </div>
  );
};

const ImageAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => {
  const filePath = `/api${toUrlPath(attachment.file_path)}`;
  const thumbSrc = attachment.thumbnail_path
    ? `/api${toUrlPath(attachment.thumbnail_path)}`
    : filePath;

  return (
    <a
      href={authUrl(filePath)}
      target="_blank"
      rel="noopener noreferrer"
      download={attachment.file_name}
      className={styles.imageLink}
    >
      <img
        src={authUrl(thumbSrc)}
        alt={attachment.file_name}
        className={styles.imageThumb}
        loading="lazy"
      />
    </a>
  );
};

const PDFAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => (
  <a
    href={authUrl(`/api${toUrlPath(attachment.file_path)}`)}
    target="_blank"
    rel="noopener noreferrer"
    download={attachment.file_name}
    className={styles.pdfCard}
  >
    <FilePdfOutlined className={styles.pdfIcon} />
    <div className={styles.pdfInfo}>
      <span className={styles.pdfName}>{attachment.file_name}</span>
      <span className={styles.pdfSize}>{formatFileSize(attachment.file_size)}</span>
    </div>
  </a>
);

const PptxAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => {
  const [open, setOpen] = useState(false);
  const fileUrl = authUrl(`/api${toUrlPath(attachment.file_path)}`);

  return (
    <>
      <div className={styles.pptCard}>
        <FilePptOutlined className={styles.pptIcon} />
        <div className={styles.pdfInfo}>
          <span className={styles.pdfName}>{attachment.file_name}</span>
          <span className={styles.pdfSize}>{formatFileSize(attachment.file_size)}</span>
        </div>
        <div className={styles.pptActions}>
          <button
            type="button"
            className={styles.pptPreviewBtn}
            title="预览幻灯片"
            onClick={() => setOpen(true)}
          >
            <EyeOutlined />
            <span>预览</span>
          </button>
          <a
            href={fileUrl}
            download={attachment.file_name}
            className={styles.pptDownload}
            title="下载文件"
          >
            <DownloadOutlined />
          </a>
        </div>
      </div>
      <Modal
        open={open}
        onCancel={() => setOpen(false)}
        footer={null}
        width="94vw"
        style={{ top: 16, maxWidth: 'none' }}
        title={
          <span className={styles.modalTitle}>
            <FilePptOutlined className={styles.pptIcon} />
            <span className={styles.modalTitleName}>{attachment.file_name}</span>
          </span>
        }
        destroyOnClose
      >
        {open && (
          <Suspense fallback={<div className={styles.pptModalLoading}>加载预览组件…</div>}>
            <PptxPreview fileUrl={fileUrl} fileName={attachment.file_name} />
          </Suspense>
        )}
      </Modal>
    </>
  );
};

const GenericFileAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => (
  <a
    href={authUrl(`/api${toUrlPath(attachment.file_path)}`)}
    download={attachment.file_name}
    className={styles.pdfCard}
  >
    <FileOutlined className={styles.pdfIcon} />
    <div className={styles.pdfInfo}>
      <span className={styles.pdfName}>{attachment.file_name}</span>
      <span className={styles.pdfSize}>{formatFileSize(attachment.file_size)}</span>
    </div>
    <DownloadOutlined className={styles.downloadIcon} />
  </a>
);
