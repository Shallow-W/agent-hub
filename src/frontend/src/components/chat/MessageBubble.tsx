import React, { useState, useMemo } from 'react';
import { Avatar, Typography, Spin, Button, Tooltip } from 'antd';
import {
  CloseOutlined,
  DownOutlined,
  MessageOutlined,
  ReloadOutlined,
  RollbackOutlined,
  UpOutlined,
} from '@ant-design/icons';
import type { Message, OptimisticStatus } from '@/types/message';
import type { MessageAttachment } from '@/types/attachment';
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
  // 先抽出代码块，避免后续转义破坏格式。
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

  // 再抽出行内代码，和普通文本分开处理。
  const inlineCodes: string[] = [];
  result = result.replace(/`([^`]+)`/g, (_match, code) => {
    const idx = inlineCodes.length;
    inlineCodes.push(`<code class="${styles.inlineCode}">${escapeHtml(code)}</code>`);
    return `\x00INLINE${idx}\x00`;
  });

  // 转义代码之外的 HTML，防止消息内容注入。
  result = escapeHtml(result);

  // 应用轻量 Markdown 规则。
  result = result.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
  result = result.replace(/\*(.+?)\*/g, '<em>$1</em>');
  result = result.replace(/\[([^\]]+)\]\(([^()\s]+(?:\([^()]*\)[^()\s]*)*)\)/g, (_match, text, href) => {
    const safeHref = /^https?:\/\//i.test(href) || /^mailto:/i.test(href) ? href : '#';
    return `<a href="${safeHref}" target="_blank" rel="noopener noreferrer">${text}</a>`;
  });

  // 保留消息里的换行。
  result = result.replace(/\n/g, '<br/>');

  // Highlight @mentions — before restoring code blocks to avoid matching inside them
  result = result.replace(/(^|\s)@([a-zA-Z0-9_一-龥-]{2,20})(?=\s|$)/g,
    `$1<span class="${styles.mention}">@$2</span>`);

  // 最后恢复前面暂存的代码内容。
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

const MessageBubbleInner: React.FC<MessageBubbleProps> = ({
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
  const agentName = (() => {
    if (message.role !== 'assistant' || !message.artifacts_json) return null;
    try { return (JSON.parse(message.artifacts_json) as { agent_name?: string }).agent_name ?? null; } catch { return null; }
  })();
  const displayName = message.username || agentName || (isOwn ? '我' : (message.role === 'user' ? '用户' : '助手'));
  const avatarLetter = message.username?.charAt(0)?.toUpperCase() || '?';
  const renderedContent = useMemo(() => renderMarkdown(message.content ?? ''), [message.content]);
  const contentLength = message.content?.length ?? 0;
  const lineCount = message.content?.split('\n').length ?? 0;
  const shouldCollapse = contentLength > COLLAPSE_CHAR_LIMIT || lineCount > COLLAPSE_LINE_LIMIT;
  const collapsed = shouldCollapse && !expanded;
  // 这里只做前端提示，撤回窗口仍由服务端校验。
  const canRecall = isOwn && onRecall && (Date.now() - new Date(message.created_at).getTime()) < 3 * 60 * 1000;

  // 乐观消息的 pendingAttachments 需要转换为 MessageAttachment 格式才能渲染
  const displayAttachments = useMemo((): MessageAttachment[] => {
    if (message.attachments && message.attachments.length > 0) return message.attachments;
    const pending = (message as Message & { pendingAttachments?: unknown[] }).pendingAttachments;
    if (!pending || !Array.isArray(pending) || pending.length === 0) return [];
    return pending.map((p, i) => ({
      id: `pending_${i}`,
      message_id: '',
      file_name: (p as Record<string, unknown>).file_name as string,
      mime_type: (p as Record<string, unknown>).mime_type as string,
      file_size: (p as Record<string, unknown>).file_size as number,
      file_path: (p as Record<string, unknown>).file_path as string,
      thumbnail_path: ((p as Record<string, unknown>).thumbnail_path as string) ?? null,
      width: ((p as Record<string, unknown>).width as number) ?? 0,
      height: ((p as Record<string, unknown>).height as number) ?? 0,
      created_at: new Date().toISOString(),
    }));
  }, [message.attachments, (message as Message & { pendingAttachments?: unknown[] }).pendingAttachments]);

  if (isSystem) {
    return (
      <div className={styles.systemMessage}>
        <span className={styles.systemText}>
          {message.content}
        </span>
      </div>
    );
  }

  return (
    <div
      className={`${styles.bubble} ${isOwn ? styles.bubbleUser : styles.bubbleAssistant} ${isGrouped ? styles.bubbleGrouped : ''}`}
      data-message-id={message.id}
    >
      {showAvatar && (
        <Avatar
          size={24}
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
            className={`${styles.replyBtn} ${styles.recallBtn}`}
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
                {escapeHtml(message.reply_to_message.sender_id ? message.reply_to_message.username || '用户' : '助手')}
              </span>
              {escapeHtml(
                (message.reply_to_message.content ?? '').length > 50
                  ? (message.reply_to_message.content ?? '').slice(0, 50) + '...'
                  : (message.reply_to_message.content ?? ''),
              )}
            </div>
          )}
          {displayAttachments.length > 0 && (
            <MessageAttachmentView attachments={displayAttachments} />
          )}
          {message.content && (
            <div
              className={styles.markdownBody}
              dangerouslySetInnerHTML={{ __html: renderedContent }}
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

export const MessageBubble = React.memo(MessageBubbleInner);
