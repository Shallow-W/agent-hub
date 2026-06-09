import React, { useState, useMemo, type ReactNode } from 'react';
import { Avatar, Typography, Spin, Button, Tooltip, Dropdown } from 'antd';
import { message as antMessage } from '@/utils/message';
import type { MenuProps } from 'antd';
import {
  CloseOutlined,
  CopyOutlined,
  DownOutlined,
  ForwardOutlined,
  MessageOutlined,
  PushpinOutlined,
  ReloadOutlined,
  RollbackOutlined,
  UpOutlined,
} from '@ant-design/icons';
import ReactMarkdown from 'react-markdown';
import type { Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { useAuthStore } from '@/store/authStore';
import { useAgentStore } from '@/store/agentStore';
import type { Message, OptimisticStatus, Artifact, MessageArtifacts } from '@/types/message';
import type { MessageAttachment } from '@/types/attachment';
import type { ConversationAgent } from '@/types/conversation';
import { truncateGraphemes } from '@/utils/truncateText';
import { MessageAttachmentView } from './MessageAttachmentView';
import { CodeBlock, extractText } from './CodeBlock';
import { ArtifactCard } from './ArtifactCard';
import { DeployStatusCard } from './DeployStatusCard';
import { escapeHtml } from './highlight';
import { resolveAgentAvatar, resolveUserAvatar } from '@/components/agent/agentPresentation';
import styles from './MessageBubble.module.css';

const { Text } = Typography;
const COLLAPSE_CHAR_LIMIT = 500;
const COLLAPSE_LINE_LIMIT = 12;
const REPLY_PREVIEW_LIMIT = 50;

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

/** Process top-level string leaves for @mention highlighting — does NOT recurse into React elements. */
function renderChildrenWithMentions(children: ReactNode): ReactNode {
  if (typeof children === 'string') {
    const parts = renderTextWithMentions(children);
    return parts.length === 1 ? parts[0] : <>{parts}</>;
  }
  if (Array.isArray(children)) {
    return <>{children.map((c, i) => {
      if (typeof c === 'string') {
        const parts = renderTextWithMentions(c);
        return <React.Fragment key={i}>{parts.length === 1 ? parts[0] : <>{parts}</>}</React.Fragment>;
      }
      return <React.Fragment key={i}>{c}</React.Fragment>;
    })}</>;
  }
  // Non-string, non-array nodes (elements, null, undefined, numbers, booleans)
  // pass through unchanged — mentions only highlight in text leaves.
  return children;
}

function truncatePreview(text: string, maxLength = REPLY_PREVIEW_LIMIT): string {
  return truncateGraphemes(text, maxLength);
}

/**
 * 从 code 产物列表构建「内容 → root_id」纯查找表。
 * 尾部换行符剥离后作为 key，重复内容首个产物胜出（极少见，可接受）。
 * 纯函数，零渲染副作用，React StrictMode 双调用安全。
 */
function buildContentRootMap(codeArtifacts: Artifact[]): Map<string, string> {
  const map = new Map<string, string>();
  for (const art of codeArtifacts) {
    if (!art.root_id || art.content == null) continue;
    const key = art.content.replace(/\n$/, '');
    if (!map.has(key)) map.set(key, art.root_id); // 重复内容首个胜出
  }
  return map;
}

function codeLanguage(className?: string): string {
  return className?.replace(/^language-/, '').toLowerCase() ?? '';
}

function looksLikeMarkdownDocument(text: string): boolean {
  const src = text.trim();
  if (src.length < 40) return false;
  const headingMatches = src.match(/^#{1,3}\s+\S.+$/gm) || [];
  if (headingMatches.length === 0) return false;
  if (headingMatches.length >= 2) return true;
  return /(^|\n)(?:[-*]\s+\S|\|.+\||```)/.test(src);
}

function markdownText(children: ReactNode): string {
  if (children == null || typeof children === 'boolean') return '';
  if (typeof children === 'string' || typeof children === 'number') return String(children);
  if (Array.isArray(children)) return children.map(markdownText).join('');
  if (React.isValidElement<{ children?: ReactNode }>(children)) {
    return markdownText(children.props.children);
  }
  return '';
}

/** 基于本条消息的 code 产物构建 markdown 组件，使围栏代码块能接通版本能力。 */
function unwrapMarkdownDocumentFence(content: string): string {
  const normalized = content.replace(/\r\n/g, '\n');
  const lines = normalized.split('\n');
  const first = lines.findIndex((line) => line.trim() !== '');
  let last = lines.length - 1;
  while (last >= 0 && (lines[last] ?? '').trim() === '') last -= 1;

  if (first < 0 || last <= first) return content;
  const firstLine = lines[first] ?? '';
  const lastLine = lines[last] ?? '';
  const opener = firstLine.match(/^ {0,3}`{3,}\s*(markdown|md)\s*$/i);
  if (!opener || !/^ {0,3}`{3,}\s*$/.test(lastLine)) return content;
  return lines.slice(first + 1, last).join('\n').replace(/\s+$/, '');
}

function markdownDocumentContent(content: string): string {
  const unwrapped = unwrapMarkdownDocumentFence(content);
  if (unwrapped !== content) return unwrapped;

  const normalized = content.replace(/\r\n/g, '\n');
  const lines = normalized.split('\n');
  for (let i = 0; i < lines.length; i += 1) {
    if (!/^ {0,3}`{3,}\s*(markdown|md)\s*$/i.test(lines[i] ?? '')) continue;
    for (let j = lines.length - 1; j > i; j -= 1) {
      if (!/^ {0,3}`{3,}\s*$/.test(lines[j] ?? '')) continue;
      const candidate = lines.slice(i + 1, j).join('\n').replace(/\s+$/, '');
      if (looksLikeMarkdownDocument(candidate)) return candidate;
      break;
    }
  }
  const headingStart = normalized.search(/^#{1,3}\s+\S.+$/m);
  if (headingStart > 0) {
    const candidate = normalized.slice(headingStart).replace(/\s+$/, '');
    if (looksLikeMarkdownDocument(candidate)) return candidate;
  }
  return content;
}

function buildMarkdownComponents(codeArtifacts: Artifact[]): Components {
  // 预构建查找表，纯计算，无 mutation，StrictMode 双调用安全。
  const contentRootMap = buildContentRootMap(codeArtifacts);
  return {
    code({ className, children, node, ...rest }) {
      const isBlock = className?.startsWith('language-');
      if (isBlock) {
        const ct = extractText(children);
        const lang = codeLanguage(className);
        if ((lang === 'markdown' || lang === 'md') && looksLikeMarkdownDocument(ct)) {
          return (
            <div className={styles.embeddedMarkdownDocument}>
              <ReactMarkdown remarkPlugins={[remarkGfm]} components={embeddedDocumentComponents}>
                {markdownDocumentContent(ct.trim())}
              </ReactMarkdown>
            </div>
          );
        }
        const rootId = contentRootMap.get(ct.replace(/\n$/, ''));
        return (
          <CodeBlock className={className} expandable artifactRootId={rootId}>
            {children}
          </CodeBlock>
        );
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
    ...sharedMarkdownComponents,
  };
}

const sharedMarkdownComponents: Components = {
  a({ href, children, node, ...rest }) {
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

const embeddedDocumentComponents: Components = {
  ...sharedMarkdownComponents,
  code({ children }) {
    return <span className={styles.documentCodeText}>{children}</span>;
  },
  pre({ children }) {
    const text = markdownText(children);
    if (looksLikeMarkdownDocument(text)) {
      return (
        <ReactMarkdown remarkPlugins={[remarkGfm]} components={embeddedDocumentComponents}>
          {markdownDocumentContent(text)}
        </ReactMarkdown>
      );
    }
    return <div className={styles.documentPlainText}>{text}</div>;
  },
};

/** Renders markdown content with full GFM support. */
const REMARK_PLUGINS = [remarkGfm];
const MarkdownRenderer: React.FC<{ content: string; codeArtifacts: Artifact[] }> = ({
  content,
  codeArtifacts,
}) => {
  // 每次渲染重新构建 components（含查找表），纯计算，无 mutation，StrictMode 安全。
  const components = buildMarkdownComponents(codeArtifacts);
  return (
    <ReactMarkdown remarkPlugins={REMARK_PLUGINS} components={components}>
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
  onTogglePin?: (message: Message) => void;
  conversationAgents?: ConversationAgent[];
  replyCount?: number;
  onOpenThread?: (message: Message) => void;
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

function fallbackAttachmentName(value: unknown, filePath: unknown): string {
  if (typeof value === 'string' && value.trim()) return value.trim();
  if (typeof filePath === 'string' && filePath.trim()) {
    const normalized = filePath.replace(/\\/g, '/');
    return decodeURIComponent(normalized.split('/').pop() || '未命名文件');
  }
  return '未命名文件';
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
  onTogglePin,
  conversationAgents = [],
  replyCount = 0,
  onOpenThread,
}) => {
  const [expanded, setExpanded] = useState(false);
  const isSystem = message.role === 'system';
  const isOptimisticSending = optimisticStatus === 'sending';
  const isOptimisticFailed = optimisticStatus === 'failed';

  // 从 artifacts_json 解析 agent 元信息（{agent_id, agent_name, cli_tool, deployment?}）。
  const agentMeta = useMemo((): MessageArtifacts => {
    if (message.role !== 'assistant' || !message.artifacts_json) return {};
    try { return JSON.parse(message.artifacts_json) as MessageArtifacts; } catch { return {}; }
  }, [message.role, message.artifacts_json]);
  const agentName = agentMeta.agent_name ?? null;
  const deployment = agentMeta.deployment ?? null;
  const conversationAgentRole = useMemo(() => {
    if (!agentMeta.agent_id) return null;
    return conversationAgents.find((agent) => agent.agent_id === agentMeta.agent_id)?.role ?? null;
  }, [agentMeta.agent_id, conversationAgents]);
  const agentBadgeLabel = conversationAgentRole === 'orchestrator'
    ? 'Orchestrator agent'
    : conversationAgentRole === 'worker'
      ? 'Worker agent'
      : 'Agent';

  // 用 agent_id 从 store 查找完整 agent（含手动选定的 avatar 字段）。
  // selector 取稳定值（agents 数组），React.memo 避免不必要重渲染。
  const agents = useAgentStore((s) => s.agents);
  const storeAgent = useMemo(
    () => (agentMeta.agent_id ? agents.find((a) => a.id === agentMeta.agent_id) : undefined),
    [agents, agentMeta.agent_id],
  );

  const displayName = message.username || agentName || (isOwn ? '我' : (message.role === 'user' ? '用户' : '助手'));

  // 头像来源优先级：
  //   1. assistant + store 里找到完整 agent → resolveAgentAvatar(agent)（honors agent.avatar）
  //   2. assistant + 未找到（历史消息/列表未加载）→ resolveAgentAvatar({id: agent_id, name: agent_name}) 哈希兜底
  //   3. 自己（当前登录用户，含 avatar）
  //   4. 其他用户（按 sender_id/username 稳定哈希默认）
  const avatarSrc = useMemo((): string | undefined => {
    if (message.role === 'assistant' && agentName) {
      if (storeAgent) return resolveAgentAvatar(storeAgent);
      return resolveAgentAvatar({ id: agentMeta.agent_id ?? agentName, name: agentName });
    }
    if (message.role === 'assistant') return undefined;
    if (isOwn) {
      const me = useAuthStore.getState().user;
      return me ? resolveUserAvatar(me) : undefined;
    }
    return resolveUserAvatar({ id: message.sender_id, username: message.username });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [message.role, message.sender_id, message.username, agentName, agentMeta.agent_id, storeAgent, isOwn]);

  const avatarLetter = agentName
    ? 'AI'
    : (message.username?.charAt(0)?.toUpperCase()
        || (isOwn ? (useAuthStore.getState().user?.username?.charAt(0)?.toUpperCase() || '?') : '?'));
  // 代码块回到正文原位（散文↔代码交错），仅 webpage 产物走底部卡片
  const displayContent = message.content ?? '';
  // 卡片类产物（webpage/document）每个血缘只渲染最新版本，避免历史版本产生重复卡片。
  // （后端返回全部版本以支撑代码块的内容匹配，故此处需按 root_id 去重取最新。）
  const cardArtifacts = useMemo(() => {
    const all = message.artifacts?.filter((a) => a.type !== 'code') ?? [];
    const latest = new Map<string, Artifact>();
    for (const a of all) {
      const key = a.root_id || a.id || '';
      const prev = latest.get(key);
      if (!prev || a.version > prev.version) latest.set(key, a);
    }
    return Array.from(latest.values());
  }, [message.artifacts]);
  // 仅 code 产物参与内联代码块的内容匹配（接通版本能力）；保留全部版本，
  // 使消息 markdown 里的原始代码块总能匹配到对应版本的 root_id。
  const codeArtifacts = useMemo(
    () => message.artifacts?.filter((a) => a.type === 'code') ?? [],
    [message.artifacts],
  );
  const contentLength = displayContent.length;
  const lineCount = displayContent.split('\n').length;
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
    ...(onTogglePin
      ? [{
          key: 'pin' as const,
          icon: <PushpinOutlined />,
          label: message.pinned ? '取消 Pin' : 'Pin 到上下文黑板',
          onClick: () => onTogglePin(message),
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
      file_name: fallbackAttachmentName(
        (p as Record<string, unknown>).file_name,
        (p as Record<string, unknown>).file_path,
      ),
      mime_type: (p as Record<string, unknown>).mime_type as string,
      file_size: (p as Record<string, unknown>).file_size as number,
      file_path: (p as Record<string, unknown>).file_path as string,
      thumbnail_path: ((p as Record<string, unknown>).thumbnail_path as string) ?? null,
      url: (p as Record<string, unknown>).url as string | undefined,
      thumbnail_url: ((p as Record<string, unknown>).thumbnail_url as string | null | undefined) ?? null,
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
            size={36}
            className={styles.chatAvatar}
            src={avatarSrc}
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
        {!isSystem && onTogglePin && (
          <Tooltip title={message.pinned ? '取消 Pin' : 'Pin 到上下文黑板'}>
            <Button
              type="text"
              size="small"
              icon={<PushpinOutlined />}
              className={`${styles.replyBtn} ${styles.pinBtn} ${message.pinned ? styles.pinBtnActive : ''}`}
              onClick={() => onTogglePin(message)}
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
                <span className={styles.agentBadge}>{agentBadgeLabel}</span>
              )}
              {message.pinned && (
                <Tooltip title="已 Pin 到上下文黑板">
                  <PushpinOutlined className={styles.pinBadge} />
                </Tooltip>
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
                  {escapeHtml(message.reply_to_message.username || (message.reply_to_message.sender_id ? '用户' : '助手'))}
                </span>
                {escapeHtml(truncatePreview(message.reply_to_message.content ?? ''))}
              </div>
            )}
            {displayAttachments.length > 0 && (
              <MessageAttachmentView attachments={displayAttachments} />
            )}
            {displayContent && (
              <div className={styles.markdownBody}>
                <MarkdownRenderer content={displayContent} codeArtifacts={codeArtifacts} />
              </div>
            )}
            {cardArtifacts.length > 0 && (
              <ArtifactCard artifacts={cardArtifacts} agentName={agentName} />
            )}
            {deployment && (
              <div className={styles.deployCard}>
                <DeployStatusCard deployment={deployment} />
              </div>
            )}
            {collapsed && <div className={styles.fadeMask} />}
            {streaming && <span className={styles.streamingCursor} />}
            {isOptimisticSending && (
              <Spin size="small" className={styles.sendingSpin} />
            )}
          </div>
          {replyCount > 0 && onOpenThread && (
            <button
              className={styles.threadBtn}
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onOpenThread(message);
              }}
            >
              <MessageOutlined />
              {replyCount} 条回复
            </button>
          )}
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
