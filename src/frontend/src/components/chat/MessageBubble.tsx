import React, { useState, useMemo, useCallback } from 'react';
import { Avatar, Typography, Spin, Button, Tooltip, Dropdown, message as antMessage } from 'antd';
import type { MenuProps } from 'antd';
import {
  CloseOutlined,
  CopyOutlined,
  DownOutlined,
  ForwardOutlined,
  MessageOutlined,
  ReloadOutlined,
  RollbackOutlined,
  UpOutlined,
} from '@ant-design/icons';
import hljs from 'highlight.js/lib/core';
import javascript from 'highlight.js/lib/languages/javascript';
import typescript from 'highlight.js/lib/languages/typescript';
import go from 'highlight.js/lib/languages/go';
import python from 'highlight.js/lib/languages/python';
import rust from 'highlight.js/lib/languages/rust';
import bash from 'highlight.js/lib/languages/bash';
import json from 'highlight.js/lib/languages/json';
import yaml from 'highlight.js/lib/languages/yaml';
import markdown from 'highlight.js/lib/languages/markdown';
import xml from 'highlight.js/lib/languages/xml';
import cssLang from 'highlight.js/lib/languages/css';
import sql from 'highlight.js/lib/languages/sql';
import { useAuthStore } from '@/store/authStore';
import type { Message, OptimisticStatus } from '@/types/message';
import type { MessageAttachment } from '@/types/attachment';
import { MessageAttachmentView } from './MessageAttachmentView';
import styles from './MessageBubble.module.css';

// Register only the languages we need — keeps bundle small
hljs.registerLanguage('javascript', javascript);
hljs.registerLanguage('js', javascript);
hljs.registerLanguage('typescript', typescript);
hljs.registerLanguage('ts', typescript);
hljs.registerLanguage('go', go);
hljs.registerLanguage('golang', go);
hljs.registerLanguage('python', python);
hljs.registerLanguage('py', python);
hljs.registerLanguage('rust', rust);
hljs.registerLanguage('bash', bash);
hljs.registerLanguage('sh', bash);
hljs.registerLanguage('shell', bash);
hljs.registerLanguage('zsh', bash);
hljs.registerLanguage('json', json);
hljs.registerLanguage('yaml', yaml);
hljs.registerLanguage('yml', yaml);
hljs.registerLanguage('markdown', markdown);
hljs.registerLanguage('md', markdown);
hljs.registerLanguage('html', xml);
hljs.registerLanguage('xml', xml);
hljs.registerLanguage('css', cssLang);
hljs.registerLanguage('sql', sql);

// Friendly display names for language tags
const LANG_DISPLAY: Record<string, string> = {
  js: 'JavaScript', ts: 'TypeScript', golang: 'Go', py: 'Python',
  sh: 'Shell', shell: 'Shell', zsh: 'Shell', yml: 'YAML', md: 'Markdown',
};

// Inline SVGs for copy buttons (rendered inside dangerouslySetInnerHTML)
const COPY_ICON_SVG = '<svg viewBox="64 64 896 896" focusable="false" width="1em" height="1em" fill="currentColor" aria-hidden="true"><path d="M832 64H296c-4.4 0-8 3.6-8 8v56c0 4.4 3.6 8 8 8h496v688c0 4.4 3.6 8 8 8h56c4.4 0 8-3.6 8-8V144c0-44.2-35.8-80-80-80z"/><path d="M704 816H168c-4.4 0-8-3.6-8-8V168c0-4.4 3.6-8 8-8h536c4.4 0 8 3.6 8 8v640c0 4.4-3.6 8-8 8zm168-8V144c0-44.2-35.8-80-80-80H168c-44.2 0-80 35.8-80 80v664c0 44.2 35.8 80 80 80h624c44.2 0 80-35.8 80-80z"/></svg>';
const CHECK_ICON_SVG = '<svg viewBox="64 64 896 896" focusable="false" width="1em" height="1em" fill="currentColor" aria-hidden="true"><path d="M912 190h-69.9c-9.8 0-19.1 4.5-25.1 12.2L404.7 724.5 207 474a32 32 0 0 0-25.1-12.2H112c-6.7 0-10.4 7.7-6.3 12.9l281.9 358.9c12.8 16.2 37.4 16.2 50.3 0l508.1-643.7c4-5.2.4-12.9-6.3-12.9z"/></svg>';

const { Text } = Typography;
const COLLAPSE_CHAR_LIMIT = 500;
const COLLAPSE_LINE_LIMIT = 12;

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

/** Highlight code string, returning highlighted HTML. Falls back to escaped text. */
function highlightCode(code: string, lang?: string): string {
  const trimmed = code.replace(/\n$/, '');
  try {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(trimmed, { language: lang }).value;
    }
    return hljs.highlightAuto(trimmed).value;
  } catch {
    return escapeHtml(trimmed);
  }
}

function renderMarkdown(text: string): string {
  // 1. Extract fenced code blocks first to protect them from other transforms.
  const codeBlocks: string[] = [];
  let result = text.replace(/```(\w*)\n?([\s\S]*?)```/g, (_match, lang, code) => {
    const idx = codeBlocks.length;
    const highlighted = highlightCode(code, lang || undefined);
    const displayLang = lang ? (LANG_DISPLAY[lang] || lang) : '';
    const copyBtn = `<button class="${styles.codeCopyBtn}" data-code-idx="${idx}" type="button" title="复制代码"><span class="${styles.codeCopyIcon}">${COPY_ICON_SVG}</span><span class="${styles.codeCopyText}">复制</span></button>`;
    const header = `<div class="${styles.codeHeader}"><span>${escapeHtml(displayLang)}</span>${copyBtn}</div>`;
    const block = `<pre class="${styles.codeBlock}"><code>${highlighted}</code></pre>`;

    codeBlocks.push(
      `<div class="${styles.codeBlockWrapper}">${header}${block}</div>`,
    );
    return `\x00CODEBLOCK${idx}\x00`;
  });

  // 2. Extract inline code spans.
  const inlineCodes: string[] = [];
  result = result.replace(/`([^`]+)`/g, (_match, code) => {
    const idx = inlineCodes.length;
    inlineCodes.push(`<code class="${styles.inlineCode}">${escapeHtml(code)}</code>`);
    return `\x00INLINE${idx}\x00`;
  });

  // 3. Escape HTML outside of code.
  result = escapeHtml(result);

  // 4. Markdown: bold, italic, explicit links.
  result = result.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
  result = result.replace(/\*(.+?)\*/g, '<em>$1</em>');
  result = result.replace(/\[([^\]]+)\]\(([^()\s]+(?:\([^()]*\)[^()\s]*)*)\)/g, (_match, linkText, href) => {
    const safeHref = /^https?:\/\//i.test(href) || /^mailto:/i.test(href) ? href : '#';
    return `<a href="${safeHref}" target="_blank" rel="noopener noreferrer">${linkText}</a>`;
  });

  // 5. Auto-link bare URLs that are not already inside href=""
  result = result.replace(
    /(?<![="])(https?:\/\/[^\s<>&"')\]]+)/g,
    (_match, url) => `<a href="${url}" target="_blank" rel="noopener noreferrer">${url}</a>`,
  );

  // 6. Preserve line breaks.
  result = result.replace(/\n/g, '<br/>');

  // 7. Highlight @mentions.
  result = result.replace(/(^|\s)@([\p{L}\p{N}_\-.]{2,20})(?=\s|$)/gu,
    `$1<span class="${styles.mention}">@$2</span>`);

  // 8. Restore code blocks and inline code.
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
  onForward?: (message: Message) => void;
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
  onForward,
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
  const avatarLetter = agentName
    ? 'AI'
    : (message.username?.charAt(0)?.toUpperCase()
        || (isOwn ? (useAuthStore.getState().user?.username?.charAt(0)?.toUpperCase() || '?') : '?'));
  const renderedContent = useMemo(() => renderMarkdown(message.content ?? ''), [message.content]);
  const contentLength = message.content?.length ?? 0;
  const lineCount = message.content?.split('\n').length ?? 0;
  const shouldCollapse = contentLength > COLLAPSE_CHAR_LIMIT || lineCount > COLLAPSE_LINE_LIMIT;
  const collapsed = shouldCollapse && !expanded;
  const canRecall = isOwn && onRecall && (Date.now() - new Date(message.created_at).getTime()) < 3 * 60 * 1000;

  const handleCopy = () => {
    navigator.clipboard.writeText(message.content ?? '').then(() => {
      antMessage.success('已复制');
    }).catch(() => {
      antMessage.error('复制失败');
    });
  };

  const contextMenuItems: MenuProps['items'] = [
    {
      key: 'copy',
      icon: <CopyOutlined />,
      label: '复制',
      onClick: handleCopy,
    },
    ...(onForward
      ? [{
          key: 'forward' as const,
          icon: <ForwardOutlined />,
          label: '转发',
          onClick: () => onForward(message),
        }]
      : []),
    ...(onReply
      ? [{
          key: 'reply' as const,
          icon: <MessageOutlined />,
          label: '回复',
          onClick: () => onReply(message),
        }]
      : []),
    ...(canRecall && onRecall
      ? [{
          key: 'recall' as const,
          icon: <RollbackOutlined />,
          label: '撤回',
          onClick: () => onRecall(message.id),
        }]
      : []),
  ];

  const handleReplyQuoteClick = (e: React.MouseEvent) => {
    e.preventDefault();
    const replyMsgId = message.reply_to_message?.id;
    if (!replyMsgId) return;
    const el = document.querySelector(`[data-message-id="${replyMsgId}"]`);
    if (el instanceof HTMLElement) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' });
      el.style.transition = 'box-shadow 0.3s ease';
      el.style.boxShadow = '0 0 0 3px var(--color-primary)';
      setTimeout(() => { el.style.boxShadow = ''; }, 1500);
    }
  };

  // Handle code-block copy buttons via event delegation
  const handleContentClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    const target = (e.target as HTMLElement).closest<HTMLElement>(`.${styles.codeCopyBtn}`);
    if (!target) return;

    const idx = target.dataset.codeIdx;
    if (idx === undefined) return;

    const wrapper = target.closest(`.${styles.codeBlockWrapper}`);
    const codeEl = wrapper?.querySelector('code');
    if (!codeEl) return;

    const codeText = codeEl.textContent ?? '';
    navigator.clipboard.writeText(codeText).then(() => {
      const iconEl = target.querySelector(`.${styles.codeCopyIcon}`);
      const textEl = target.querySelector(`.${styles.codeCopyText}`);
      if (iconEl) iconEl.innerHTML = CHECK_ICON_SVG;
      if (textEl) textEl.textContent = '已复制';
      target.classList.add(styles.codeCopyBtnCopied!);
      setTimeout(() => {
        if (iconEl) iconEl.innerHTML = COPY_ICON_SVG;
        if (textEl) textEl.textContent = '复制';
        target.classList.remove(styles.codeCopyBtnCopied!);
      }, 2000);
    }).catch(() => { /* clipboard unavailable */ });
  }, []);

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
    <Dropdown menu={{ items: contextMenuItems }} trigger={['contextMenu']}>
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
              {agentName && (
                <span className={styles.agentBadge}>Agent</span>
              )}
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
              <div
                className={styles.replyQuote}
                role="button"
                tabIndex={0}
                title="点击跳转到原消息"
                onClick={handleReplyQuoteClick}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    handleReplyQuoteClick(e as unknown as React.MouseEvent);
                  }
                }}
              >
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
                onClick={handleContentClick}
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
    </Dropdown>
  );
};

export const MessageBubble = React.memo(MessageBubbleInner);
