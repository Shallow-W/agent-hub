import React, { Suspense, lazy, useState } from 'react';
import type { ReactNode } from 'react';
import type { MessageAttachment } from '@/types/attachment';
import {
  formatFileSize,
  isImageAttachment,
  isPDFAttachment,
  isPptxAttachment,
  isWordAttachment,
} from '@/types/attachment';
import {
  DownloadOutlined,
  EyeOutlined,
  FileOutlined,
  FilePdfOutlined,
  FilePptOutlined,
  FileWordOutlined,
  FullscreenExitOutlined,
  FullscreenOutlined,
} from '@ant-design/icons';
import { Button, Modal, Tooltip } from 'antd';
import styles from './MessageAttachmentView.module.css';

// 预览库体积较大，只有用户打开弹窗时再加载，避免拉高首屏成本。
const PptxPreview = lazy(() => import('./PptxPreview'));
const DocumentPreview = lazy(() => import('./DocumentPreview'));

function toUrlPath(p: string): string {
  const normalized = p.replace(/\\/g, '/');
  return normalized.startsWith('/') ? normalized : `/${normalized}`;
}

function authUrl(path: string): string {
  const token = localStorage.getItem('agenthub_token');
  const sep = path.includes('?') ? '&' : '?';
  return token ? `${path}${sep}token=${encodeURIComponent(token)}` : path;
}

function attachmentFileUrl(attachment: MessageAttachment): string {
  return attachment.url || `/api${toUrlPath(attachment.file_path)}`;
}

function attachmentThumbUrl(attachment: MessageAttachment): string {
  if (attachment.thumbnail_url) return attachment.thumbnail_url;
  if (attachment.thumbnail_path) return `/api${toUrlPath(attachment.thumbnail_path)}`;
  return attachmentFileUrl(attachment);
}

function attachmentAPIPrefix(attachment: MessageAttachment): string {
  const url = attachment.url;
  if (!url) return '';
  const marker = '/api/uploads/';
  const idx = url.indexOf(marker);
  return idx > 0 ? url.slice(0, idx) : '';
}

function attachmentName(attachment: Pick<MessageAttachment, 'file_name' | 'file_path'>): string {
  const name = attachment.file_name?.trim();
  if (name) return name;
  const normalized = attachment.file_path.replace(/\\/g, '/');
  return decodeURIComponent(normalized.split('/').pop() || '未命名文件');
}

function previewRelativePath(attachment: MessageAttachment): string {
  return toUrlPath(attachment.file_path).replace(/^\/?uploads\//, '');
}

function pptPreviewUrl(attachment: MessageAttachment): string {
  return authUrl(`${attachmentAPIPrefix(attachment)}/api/ppt-preview/${previewRelativePath(attachment)}`);
}

function documentPreviewUrl(attachment: MessageAttachment): string {
  return authUrl(`${attachmentAPIPrefix(attachment)}/api/file-preview/${previewRelativePath(attachment)}`);
}

interface Props {
  attachments: MessageAttachment[];
}

export const MessageAttachmentView: React.FC<Props> = ({ attachments }) => {
  if (!attachments.length) return null;

  return (
    <div className={styles.container}>
      {attachments.map((att) => {
        const typeName = att.file_name || att.file_path;
        if (isImageAttachment(att.mime_type)) return <ImageAttachment key={att.id} attachment={att} />;
        if (isPDFAttachment(att.mime_type)) return <PDFAttachment key={att.id} attachment={att} />;
        if (isWordAttachment(att.mime_type, typeName)) return <WordAttachment key={att.id} attachment={att} />;
        if (isPptxAttachment(att.mime_type, typeName)) return <PptxAttachment key={att.id} attachment={att} />;
        return <GenericFileAttachment key={att.id} attachment={att} />;
      })}
    </div>
  );
};

const ImageAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => {
  const filePath = attachmentFileUrl(attachment);
  const fileName = attachmentName(attachment);
  const thumbSrc = attachmentThumbUrl(attachment);

  return (
    <a
      href={authUrl(filePath)}
      target="_blank"
      rel="noopener noreferrer"
      download={fileName}
      className={styles.imageLink}
    >
      <img
        src={authUrl(thumbSrc)}
        alt={fileName}
        className={styles.imageThumb}
        loading="lazy"
      />
    </a>
  );
};

const PDFAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => {
  const fileUrl = authUrl(attachmentFileUrl(attachment));
  const fileName = attachmentName(attachment);
  const previewUrl = documentPreviewUrl(attachment);

  return (
    <PreviewableFileAttachment
      attachment={attachment}
      fileName={fileName}
      fileUrl={fileUrl}
      icon={<FilePdfOutlined className={styles.pdfIcon} />}
      titleIcon={<FilePdfOutlined className={styles.pdfIcon} />}
      previewTitle="预览 PDF"
      previewUrl={previewUrl}
      renderPreview={() => (
        <DocumentPreview fileUrl={fileUrl} fileName={fileName} previewUrl={previewUrl} />
      )}
    />
  );
};

const WordAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => {
  const fileUrl = authUrl(attachmentFileUrl(attachment));
  const fileName = attachmentName(attachment);
  const previewUrl = documentPreviewUrl(attachment);

  return (
    <PreviewableFileAttachment
      attachment={attachment}
      fileName={fileName}
      fileUrl={fileUrl}
      icon={<FileWordOutlined className={styles.wordIcon} />}
      titleIcon={<FileWordOutlined className={styles.wordIcon} />}
      previewTitle="预览 Word"
      previewUrl={previewUrl}
      renderPreview={() => (
        <DocumentPreview fileUrl={fileUrl} fileName={fileName} previewUrl={previewUrl} />
      )}
    />
  );
};

const PptxAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => {
  const fileUrl = authUrl(attachmentFileUrl(attachment));
  const fileName = attachmentName(attachment);
  const previewUrl = pptPreviewUrl(attachment);

  return (
    <PreviewableFileAttachment
      attachment={attachment}
      fileName={fileName}
      fileUrl={fileUrl}
      icon={<FilePptOutlined className={styles.pptIcon} />}
      titleIcon={<FilePptOutlined className={styles.pptIcon} />}
      previewTitle="预览幻灯片"
      previewUrl={previewUrl}
      renderPreview={() => (
        <PptxPreview fileUrl={fileUrl} fileName={fileName} previewUrl={previewUrl} />
      )}
    />
  );
};

interface PreviewableFileAttachmentProps {
  attachment: MessageAttachment;
  fileName: string;
  fileUrl: string;
  icon: ReactNode;
  titleIcon: ReactNode;
  previewTitle: string;
  previewUrl: string;
  renderPreview: () => ReactNode;
}

const PreviewableFileAttachment: React.FC<PreviewableFileAttachmentProps> = ({
  attachment,
  fileName,
  fileUrl,
  icon,
  titleIcon,
  previewTitle,
  renderPreview,
}) => {
  const [open, setOpen] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const modalSizeClass = expanded ? styles.previewModalExpanded : styles.previewModalCompact;

  return (
    <>
      <div className={styles.previewCard}>
        {icon}
        <div className={styles.fileInfo}>
          <span className={styles.fileName} title={fileName}>{fileName}</span>
          <span className={styles.fileSize}>{formatFileSize(attachment.file_size)}</span>
        </div>
        <div className={styles.previewActions}>
          <button
            type="button"
            className={styles.previewButton}
            title={previewTitle}
            onClick={() => setOpen(true)}
          >
            <EyeOutlined />
            <span>预览</span>
          </button>
          <a
            href={fileUrl}
            download={fileName}
            className={styles.downloadButton}
            title="下载文件"
          >
            <DownloadOutlined />
          </a>
        </div>
      </div>
      <Modal
        open={open}
        onCancel={() => {
          setOpen(false);
          setExpanded(false);
        }}
        footer={null}
        width={expanded ? '94vw' : 'min(76vw, 980px)'}
        style={{ top: expanded ? 16 : 48, maxWidth: 'none' }}
        className={`${styles.previewModal} ${modalSizeClass}`}
        title={
          <div className={styles.modalTitleBar}>
            <span className={styles.modalTitle}>
              {titleIcon}
              <span className={styles.modalTitleName}>{fileName}</span>
            </span>
            <Tooltip title={expanded ? '还原' : '全屏'}>
              <Button
                type="text"
                size="small"
                icon={expanded ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
                aria-label={expanded ? '还原预览大小' : '全屏预览'}
                className={styles.modalSizeButton}
                onClick={() => setExpanded((value) => !value)}
              />
            </Tooltip>
          </div>
        }
        destroyOnHidden
      >
        <div className={styles.previewModalBody}>
        {open && (
          <Suspense fallback={<div className={styles.previewModalLoading}>加载预览组件...</div>}>
            {renderPreview()}
          </Suspense>
        )}
        </div>
      </Modal>
    </>
  );
};

const GenericFileAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => {
  const fileName = attachmentName(attachment);

  return (
    <a
      href={authUrl(attachmentFileUrl(attachment))}
      download={fileName}
      className={styles.fileCard}
    >
      <FileOutlined className={styles.genericIcon} />
      <div className={styles.fileInfo}>
        <span className={styles.fileName} title={fileName}>
          {fileName}
        </span>
        <span className={styles.fileSize}>{formatFileSize(attachment.file_size)}</span>
      </div>
      <DownloadOutlined className={styles.downloadIcon} />
    </a>
  );
};
