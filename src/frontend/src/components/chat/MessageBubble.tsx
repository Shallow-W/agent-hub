import React, { useState, useMemo, type ReactNode } from 'react';
import { Avatar, Typography, Spin, Button, Tooltip, Dropdown, message as antMessage } from 'antd';
import type { MenuProps } from 'antd';
import {
  CheckOutlined,
  CloseOutlined,
  CopyOutlined,
  DownOutlined,
  ForwardOutlined,
  MessageOutlined,
  ReloadOutlined,
  RollbackOutlined,
  UpOutlined,
} from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import type { Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
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

/** Highlight code string, returning highlighted HTML. Falls back to plain text. */
function highlightCode(code: string, lang?: string): string {
  const trimmed = code.replace(/\n$/, '');
  try {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(trimmed, { language: lang }).value;
    }
    return hljs.highlightAuto(trimmed).value;
  } catch {
    return trimmed;
  }
}

// ── ReactMarkdown custom components ──

const MENTION_RE = /(^|\s)@([\p{L}\p{N}_\-.]{2,20})(?=\s|$)/gu;

/** Split text nodes so @mentions get highlighted spans. */
function renderTextWithMentions(text: string): ReactNode[] {
  const parts: ReactNode[] = [];
  let lastIndex = 0;
  MENTION_RE.lastIndex = 0;
  let match: RegExpExecArray | null;
  let key = 0;
  while ((match = MENTION_RE.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index));
    }
    if (match[1]) parts.push(match[1]);
    parts.push(
      <span key={`m${key++}`} className={styles.mention}>
        @{match[2]}
      </span>,
    );
    lastIndex = MENTION_RE.lastIndex;
  }
  if (lastIndex < text.length) parts.push(text.slice(lastIndex));
  return parts;
}

/** Walk ReactNode tree, split string leaves for @mention highlighting. */
function renderChildrenWithMentions(children: ReactNode): ReactNode {
  if (typeof children === 'string') {
    const parts = renderTextWithMentions(children);
    return parts.length === 1 ? parts[0] : <>{parts}</>;
  }
  if (Array.isArray(children)) {
    return <>{children.map((c, i) => <React.Fragment key={i}>{renderChildrenWithMentions(c)}</React.Fragment>)}</>;
  }
  return children;
}

/** Fenced code block with language header and copy button. */
const CodeBlock: React.FC<{ className?: string; children?: ReactNode }> = ({
  className,
  children,
}) => {
  const lang = className?.replace('language-', '') || '';
  const codeStr = String(children).replace(/\n$/, '');
  const [copied, setCopied] = useState(false);

  const displayLang = lang ? (LANG_DISPLAY[lang] || lang) : '';
  const highlighted = highlightCode(codeStr, lang || undefined);

  const handleCopy = () => {
    navigator.clipboard.writeText(codeStr).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }).catch(() => { /* clipboard unavailable */ });
  };

  return (
    <div className={styles.codeBlockWrapper}>
      <div className={styles.codeHeader}>
        <span>{displayLang}</span>
        <button
          className={`${styles.codeCopyBtn} ${copied ? styles.codeCopyBtnCopied : ''}`}
          type="button"
          title="复制代码"
          onClick={handleCopy}
        >
          <span className={styles.codeCopyIcon}>
            {copied ? <CheckOutlined /> : <CopyOutlined />}
          </span>
          <span className={styles.codeCopyText}>{copied ? '已复制' : '复制'}</span>
        </button>
      </div>
      <pre className={styles.codeBlock}>
        <code dangerouslySetInnerHTML={{ __html: highlighted }} />
      </pre>
    </div>
  );
};

const markdownComponents: Components = {
  code({ className, children, ...rest }) {
    const isBlock = className?.startsWith('language-');
    if (isBlock) {
      return <CodeBlock className={className}>{children}</CodeBlock>;
    }
    return (
      <code className={styles.inlineCode} {...rest}>
        {children}
      </code>
    );
  },
  pre({ children }) {
    // Let the code component handle the wrapper; strip the extra <pre>
    return <>{children}</>;
  },
  a({ href, children, ...rest }) {
    const safeHref =
      href && (/^https?:\/\//i.test(href) || /^mailto:/i.test(href))
        ? href
        : '#';
    return (
      <a href={safeHref} target="_blank" rel="noopener noreferrer" {...rest}>
        {children}
      </a>
    );
  },
  p({ children }) {
    return <p>{renderChildrenWithMentions(children)}</p>;
  },
  li({ children }) {
    return <li>{renderChildrenWithMentions(children)}</li>;
  },
  td({ children }) {
    return <td>{renderChildrenWithMentions(children)}</td>;
  },
};

/** Renders markdown content with full GFM support. */
const MarkdownRenderer: React.FC<{ content: string }> = ({ content }) => {
  return (
    <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents}>
      {content}
    </ReactMarkdown>
  );
};

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
              <div className={styles.markdownBody}>
                <MarkdownRenderer content={message.content ?? ''} />
              </div>
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
