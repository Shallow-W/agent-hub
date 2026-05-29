import React from 'react';
import type { MessageAttachment } from '@/types/attachment';
import { isImageAttachment, isPDFAttachment, formatFileSize } from '@/types/attachment';
import { FilePdfOutlined, FileOutlined } from '@ant-design/icons';
import styles from './MessageAttachmentView.module.css';

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
        ) : (
          <GenericFileAttachment key={att.id} attachment={att} />
        ),
      )}
    </div>
  );
};

const ImageAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => {
  const thumbSrc = attachment.thumbnail_path
    ? `/api${attachment.thumbnail_path}`
    : `/api${attachment.file_path}`;

  return (
    <a
      href={`/api${attachment.file_path}`}
      target="_blank"
      rel="noopener noreferrer"
      className={styles.imageLink}
    >
      <img
        src={thumbSrc}
        alt={attachment.file_name}
        className={styles.imageThumb}
        loading="lazy"
      />
    </a>
  );
};

const PDFAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => (
  <a
    href={`/api${attachment.file_path}`}
    target="_blank"
    rel="noopener noreferrer"
    className={styles.pdfCard}
  >
    <FilePdfOutlined className={styles.pdfIcon} />
    <div className={styles.pdfInfo}>
      <span className={styles.pdfName}>{attachment.file_name}</span>
      <span className={styles.pdfSize}>{formatFileSize(attachment.file_size)}</span>
    </div>
  </a>
);

const GenericFileAttachment: React.FC<{ attachment: MessageAttachment }> = ({ attachment }) => (
  <a
    href={`/api${attachment.file_path}`}
    target="_blank"
    rel="noopener noreferrer"
    className={styles.pdfCard}
  >
    <FileOutlined className={styles.pdfIcon} />
    <div className={styles.pdfInfo}>
      <span className={styles.pdfName}>{attachment.file_name}</span>
      <span className={styles.pdfSize}>{formatFileSize(attachment.file_size)}</span>
    </div>
  </a>
);
