import React from 'react';
import { Avatar, Typography, Spin, Button, Tooltip } from 'antd';
import { UserOutlined, ReloadOutlined, CloseOutlined, MessageOutlined } from '@ant-design/icons';
import type { Message } from '@/types/message';
import type { OptimisticStatus } from '@/types/message';
import { MessageAttachmentView } from './MessageAttachmentView';
import styles from './MessageBubble.module.css';

const { Text } = Typography;

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function renderMarkdown(text: string): string {
  // 1. Extract code blocks and replace with placeholders
  const codeBlocks: string[] = [];
  let result = text.replace(/```(\w*)\n?([\s\S]*?)```/g, (_match, lang, code) => {
    const idx = codeBlocks.length;
    codeBlocks.push(
      `<pre class="${styles.codeBlock}"><code>${escapeHtml(code.replace(/\n$/, ''))}</code></pre>`,
    );
    if (lang) {
      codeBlocks[idx] =
        `<div class="${styles.codeBlockWrapper}"><div class="${styles.codeHeader}"><span>${escapeHtml(lang)}</span></div>${codeBlocks[idx]}</div>`;
    } else {
      codeBlocks[idx] =
        `<div class="${styles.codeBlockWrapper}">${codeBlocks[idx]}</div>`;
    }
    return `\x00CODEBLOCK${idx}\x00`;
  });

  // 2. Extract inline code and replace with placeholders
  const inlineCodes: string[] = [];
  result = result.replace(/`([^`]+)`/g, (_match, code) => {
    const idx = inlineCodes.length;
    inlineCodes.push(`<code style="background:#e8e8e8;padding:1px 4px;border-radius:3px;font-size:13px;">${escapeHtml(code)}</code>`);
    return `\x00INLINE${idx}\x00`;
  });

  // 3. Escape remaining HTML (outside code)
  result = escapeHtml(result);

  // 4. Apply inline markdown rules
  result = result.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
  result = result.replace(/\*(.+?)\*/g, '<em>$1</em>');
  result = result.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_match, text, href) => {
    const safeHref = /^https?:\/\//i.test(href) || /^mailto:/i.test(href) ? href : '#';
    return `<a href="${safeHref}" target="_blank" rel="noopener noreferrer">${text}</a>`;
  });

  // 5. Line breaks
  result = result.replace(/\n/g, '<br/>');

  // 6. Restore placeholders
  result = result.replace(/\x00CODEBLOCK(\d+)\x00/g, (_m, idx: string) => codeBlocks[Number(idx)] ?? '');
  result = result.replace(/\x00INLINE(\d+)\x00/g, (_m, idx: string) => inlineCodes[Number(idx)] ?? '');

  return result;
}

interface MessageBubbleProps {
  message: Message;
  streaming?: boolean;
  showAvatar?: boolean;
  isGrouped?: boolean;
  optimisticStatus?: OptimisticStatus;
  onRetry?: () => void;
  onRemove?: () => void;
  isRead?: boolean;
  isOwn?: boolean;
  onReply?: (message: Message) => void;
}

function formatTimestamp(dateStr: string): string {
  const d = new Date(dateStr);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const msgDate = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');

  if (msgDate.getTime() === today.getTime()) {
    return `${hh}:${mm}`;
  }
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${month}-${day} ${hh}:${mm}`;
}

export const MessageBubble: React.FC<MessageBubbleProps> = ({
  message,
  streaming = false,
  showAvatar = true,
  isGrouped = false,
  optimisticStatus,
  onRetry,
  onRemove,
  isRead,
  isOwn,
  onReply,
}) => {
  const isUser = message.role === 'user';
  const isSystem = message.role === 'system';
  const isOptimisticSending = optimisticStatus === 'sending';
  const isOptimisticFailed = optimisticStatus === 'failed';

  if (isSystem) {
    return (
      <div className={styles.systemMessage}>
        <Text type="secondary" style={{ fontSize: 12 }}>
          {message.content}
        </Text>
      </div>
    );
  }

  return (
    <div
      className={`${styles.bubble} ${isUser ? styles.bubbleUser : styles.bubbleAssistant} ${isGrouped ? styles.bubbleGrouped : ''}`}
    >
      {!isUser && showAvatar && (
        <Avatar
          size={32}
          icon={<UserOutlined />}
          style={{ backgroundColor: '#1677ff', flexShrink: 0, marginRight: 10, marginTop: 2 }}
        >
          {message.content?.charAt(0)?.toUpperCase() ?? ''}
        </Avatar>
      )}
      {!isUser && !showAvatar && <div style={{ width: 42, flexShrink: 0 }} />}
      {!isSystem && onReply && (
        <Tooltip title="回复">
          <Button
            type="text"
            size="small"
            icon={<MessageOutlined />}
            className={styles.replyBtn}
            onClick={() => onReply(message)}
            style={{ position: 'absolute', top: 0 }}
          />
        </Tooltip>
      )}
      <div className={`${styles.content} ${isUser ? styles.contentUser : styles.contentAssistant}`}>
        {!isUser && showAvatar && (
          <div className={styles.meta}>
            <Text type="secondary" style={{ fontSize: 12, fontWeight: 500 }}>Agent</Text>
          </div>
        )}
        <div
          className={`${styles.inner} ${
            isOptimisticFailed
              ? styles.innerFailed
              : isOptimisticSending
                ? styles.innerSending
                : isUser
                  ? styles.innerUser
                  : styles.innerAssistant
          }`}
        >
          {message.reply_to && (
            <div className={styles.replyQuote}>
              <span className={styles.replyQuoteSender}>
                {message.reply_to.role === 'user' ? '你' : 'Agent'}
              </span>
              {message.reply_to.content}
            </div>
          )}
          {message.attachments && message.attachments.length > 0 && (
            <MessageAttachmentView attachments={message.attachments} />
          )}
          {message.content && (
            <div dangerouslySetInnerHTML={{ __html: renderMarkdown(message.content) }} />
          )}
          {streaming && <span className={styles.streamingCursor} />}
          {isOptimisticSending && (
            <Spin size="small" style={{ marginLeft: 8 }} />
          )}
        </div>
        {isOptimisticFailed && (
          <div className={styles.failedActions}>
            <Button
              type="link"
              size="small"
              icon={<ReloadOutlined />}
              onClick={onRetry}
              style={{ color: '#ff4d4f', padding: 0, height: 'auto', fontSize: 12 }}
            >
              重试
            </Button>
            <Button
              type="link"
              size="small"
              icon={<CloseOutlined />}
              onClick={onRemove}
              style={{ color: '#999', padding: 0, height: 'auto', fontSize: 12 }}
            />
          </div>
        )}
        <div
          className={`${styles.timestamp} ${isUser ? styles.timestampUser : styles.timestampAssistant}`}
        >
          <Text type="secondary" style={{ fontSize: 11 }}>
            {formatTimestamp(message.created_at)}
          </Text>
          {isOwn && isUser && (
            <Text type="secondary" style={{ fontSize: 11, marginLeft: 4 }}>
              {isRead ? '已读' : '未读'}
            </Text>
          )}
        </div>
      </div>
    </div>
  );
};
