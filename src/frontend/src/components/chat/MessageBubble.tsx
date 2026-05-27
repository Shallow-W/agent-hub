import React, { useState } from 'react';
import { Avatar, Typography, Spin, Button, Tooltip } from 'antd';
import {
  CloseOutlined,
  DownOutlined,
  MessageOutlined,
  ReloadOutlined,
  RollbackOutlined,
  UpOutlined,
} from '@ant-design/icons';
import type { Message } from '@/types/message';
import { getAvatarColor } from '@/utils/avatarColor';
import type { OptimisticStatus } from '@/types/message';
import { MessageAttachmentView } from './MessageAttachmentView';
import styles from './MessageBubble.module.css';

const { Text } = Typography;
const COLLAPSE_CHAR_LIMIT = 280;
const COLLAPSE_LINE_LIMIT = 6;

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
  isOwn?: boolean;
  onReply?: (message: Message) => void;
  onRecall?: (messageId: string) => void;
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
  isOwn = false,
  onReply,
  onRecall,
}) => {
  const [expanded, setExpanded] = useState(false);
  const isSystem = message.role === 'system';
  const isOptimisticSending = optimisticStatus === 'sending';
  const isOptimisticFailed = optimisticStatus === 'failed';
  const displayName = message.username || (isOwn ? '我' : (message.role === 'user' ? '用户' : '助手'));
  const avatarLetter = message.username?.charAt(0)?.toUpperCase() || '?';
  const contentLength = message.content?.length ?? 0;
  const lineCount = message.content?.split('\n').length ?? 0;
  const shouldCollapse = contentLength > COLLAPSE_CHAR_LIMIT || lineCount > COLLAPSE_LINE_LIMIT;
  const collapsed = shouldCollapse && !expanded;
  const canRecall = isOwn && onRecall && (Date.now() - new Date(message.created_at).getTime()) < 2 * 60 * 1000;

  if (isSystem) {
    return (
      <div className={styles.systemMessage}>
        <Text type="secondary" className={styles.systemText}>
          {message.content}
        </Text>
      </div>
    );
  }

  return (
    <div
      className={`${styles.bubble} ${isOwn ? styles.bubbleUser : styles.bubbleAssistant} ${isGrouped ? styles.bubbleGrouped : ''}`}
    >
      {showAvatar && (
        <Avatar
          size={24}
          style={{ backgroundColor: getAvatarColor(message.username || message.role) }}
          className={styles.chatAvatar}
        >
          {avatarLetter}
        </Avatar>
      )}
      {!showAvatar && <div className={styles.avatarSpacer} />}
      {!isSystem && onReply && (
        <Tooltip title="回复">
          <Button
            type="text"
            size="small"
            icon={<MessageOutlined />}
            className={styles.replyBtn}
            onClick={() => onReply(message)}
          />
        </Tooltip>
      )}
      {canRecall && (
        <Tooltip title="撤回">
          <Button
            type="text"
            size="small"
            icon={<RollbackOutlined />}
            className={styles.replyBtn}
            onClick={() => onRecall!(message.id)}
          />
        </Tooltip>
      )}
      <div className={styles.content}>
        {showAvatar && (
          <div className={styles.meta}>
            <Text className={styles.agentLabel}>{displayName}</Text>
            <Text type="secondary" className={styles.metaTime}>
              {formatTimestamp(message.created_at)}
            </Text>
          </div>
        )}
        <div
          className={`${styles.inner} ${collapsed ? styles.innerCollapsed : ''} ${
            isOptimisticFailed
              ? styles.innerFailed
              : isOptimisticSending
                ? styles.innerSending
                : isOwn
                  ? styles.innerUser
                  : styles.innerAssistant
          }`}
        >
          {message.reply_to_message && !message.reply_to_message.deleted_at && (
            <div className={styles.replyQuote}>
              <span className={styles.replyQuoteSender}>
                {message.reply_to_message.sender_id ? message.reply_to_message.username || '用户' : 'Agent'}
              </span>
              {message.reply_to_message.content}
            </div>
          )}
          {message.attachments && message.attachments.length > 0 && (
            <MessageAttachmentView attachments={message.attachments} />
          )}
          {message.content && (
            <div
              className={styles.markdownBody}
              dangerouslySetInnerHTML={{ __html: renderMarkdown(message.content) }}
            />
          )}
          {collapsed && <div className={styles.fadeMask} />}
          {streaming && <span className={styles.streamingCursor} />}
          {isOptimisticSending && (
            <Spin size="small" className={styles.sendingSpin} />
          )}
        </div>
        {shouldCollapse && (
          <button
            className={styles.expandToggle}
            type="button"
            onClick={() => setExpanded((value) => !value)}
          >
            {expanded ? (
              <>
                收起内容
                <UpOutlined />
              </>
            ) : (
              <>
                展开完整内容
                <DownOutlined />
              </>
            )}
          </button>
        )}
        {isOptimisticFailed && (
          <div className={styles.failedActions}>
            <Button
              type="link"
              size="small"
              icon={<ReloadOutlined />}
              onClick={onRetry}
              className={styles.retryBtn}
            >
              重试
            </Button>
            <Button
              type="link"
              size="small"
              icon={<CloseOutlined />}
              onClick={onRemove}
              className={styles.removeBtn}
            />
          </div>
        )}
      </div>
    </div>
  );
};
