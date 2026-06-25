import React, { useState, useCallback, useRef, useEffect, useMemo } from 'react';
import { Input, Button, Tooltip, Spin } from 'antd';
import { message } from '@/utils/message';
import {
  CloseOutlined,
  LinkOutlined,
  RobotOutlined,
  SendOutlined,
  UpOutlined,
  DownOutlined,
} from '@ant-design/icons';
import { useMessages } from '@/hooks/useMessages';
import { useMessageStore } from '@/store/messageStore';
import { useWsStore } from '@/store/wsStore';
import { useConversationStore } from '@/store/conversationStore';
import { useAgentStore } from '@/store/agentStore';
import { uploadFile } from '@/api/upload';
import { getGroupMembers } from '@/api/group';
import { getConversationAgents } from '@/api/conversation';
import { getGroupKnowledgeBases } from '@/api/knowledge';
import { truncateGraphemes } from '@/utils/truncateText';
import type { GroupMember } from '@/types/group';
import type { ConversationAgent } from '@/types/conversation';
import type { GroupKnowledgeBase } from '@/types/knowledge';
import type { TextAreaRef } from 'antd/es/input/TextArea';
import type { AttachmentPayload } from '@/types/attachment';
import type { Message, ReplyToPreview } from '@/types/message';
import { AttachmentPreview, type PendingAttachment } from './AttachmentPreview';
import styles from './ChatInput.module.css';
import replyStyles from './ChatInput.module.css';

const { TextArea } = Input;

const ACCEPTED_TYPES =
  '.jpg,.jpeg,.png,.gif,.webp,.pdf,.pptx,.ppt,.docx,.doc,.xlsx,.xls,.txt,.md,.csv';
const MAX_FILE_SIZE = 50 * 1024 * 1024; // 50MB
const REPLY_PREVIEW_LIMIT = 50;

type MentionTarget =
  | { id: string; label: string; mentionLabel: string; kind: 'user'; user: GroupMember }
  | { id: string; label: string; mentionLabel: string; kind: 'agent'; agent: ConversationAgent };

function toMentionLabel(label: string): string {
  return label.replace(/\s+/g, '');
}

function truncatePreview(text: string, maxLength = REPLY_PREVIEW_LIMIT): string {
  return truncateGraphemes(text, maxLength);
}

interface KBTarget {
  username: string;
  kbName: string;
  kbId: string;
  visibility: string;
}

interface ChatInputProps {
  conversationId: string;
  replyTo?: Message | null;
  onCancelReply?: () => void;
  /**
   * 把内部 processFiles 暴露给父级（ChatWindow），让整个聊天窗口的拖放都能复用同一套
   * 校验 + 上传逻辑。传 null 表示注销（卸载时）。
   */
  onRegisterProcessFiles?: (handler: ((files: FileList | File[]) => void) | null) => void;
}

export const ChatInput: React.FC<ChatInputProps> = ({
  conversationId,
  replyTo,
  onCancelReply,
  onRegisterProcessFiles,
}) => {
  const [expanded, setExpanded] = useState(false);
  const [value, setValue] = useState('');
  const [pendingFiles, setPendingFiles] = useState<PendingAttachment[]>([]);
  const { send } = useMessages(conversationId);
  // PR3：流式状态来自 messages 数组里是否存在 status='streaming' 的 message。
  // 不再使用已删除的 streamingContent map。
  const isStreaming = useMessageStore(
    (s) => (s.messages[conversationId] ?? []).some((m) => m.status === 'streaming'),
  );
  const wsClient = useWsStore((s) => s.wsClient);
  const agentTyping = useWsStore((s) => conversationId ? (s.agentTyping[conversationId] ?? false) : false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [sending, setSending] = useState(false);
  const [mentionVisible, setMentionVisible] = useState(false);
  const [mentionQuery, setMentionQuery] = useState('');
  const [mentionIndex, setMentionIndex] = useState(0);
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [agentMembers, setAgentMembers] = useState<ConversationAgent[]>([]);
  const [mentionTargetsLoaded, setMentionTargetsLoaded] = useState(false);
  const [mentionStart, setMentionStart] = useState(-1); // cursor position where @ was typed
  const textareaRef = useRef<TextAreaRef>(null);

  // KB reference state
  const [kbVisible, setKbVisible] = useState(false);
  const [kbQuery, setKbQuery] = useState('');
  const [kbIndex, setKbIndex] = useState(0);
  const [kbStart, setKbStart] = useState(-1);
  const [knowledgeBases, setKnowledgeBases] = useState<GroupKnowledgeBase[]>([]);
  const [kbLoaded, setKbLoaded] = useState(false);

  const conversation = useConversationStore((s) =>
    s.conversations.find((c) => c.id === conversationId),
  );
  const boundAgentId = useConversationStore((s) => s.directAgentChats[conversationId]);
  const bindDirectAgentChat = useConversationStore((s) => s.bindDirectAgentChat);
  const unbindDirectAgentChat = useConversationStore((s) => s.unbindDirectAgentChat);
  const directAgentId = conversation?.type === 'agent' ? conversation.peer_id : boundAgentId;
  const isGroup = conversation?.type === 'group';
  const globalAgents = useAgentStore((s) => s.agents);

  const fetchMentionTargets = useCallback(async () => {
    if (!isGroup) return { members, agentMembers };
    const [nextMembers, nextAgents] = await Promise.all([
      getGroupMembers(conversationId),
      getConversationAgents(conversationId),
    ]);
    const safeMembers = nextMembers ?? [];
    const safeAgents = nextAgents ?? [];
    setMembers(safeMembers);
    setAgentMembers(safeAgents);
    setMentionTargetsLoaded(true);
    return { members: safeMembers, agentMembers: safeAgents };
  }, [agentMembers, conversationId, isGroup, members]);

  const loadMentionTargets = useCallback(() => {
    if (!isGroup || mentionTargetsLoaded) return;
    fetchMentionTargets().catch((err) => console.error('Failed to load mention targets:', err));
  }, [fetchMentionTargets, isGroup, mentionTargetsLoaded]);

  const fetchKnowledgeBases = useCallback(async () => {
    if (!isGroup) return;
    try {
      const kbs = await getGroupKnowledgeBases(conversationId);
      setKnowledgeBases(kbs ?? []);
      setKbLoaded(true);
    } catch (err) {
      console.error('Failed to load knowledge bases:', err);
    }
  }, [conversationId, isGroup]);

  const loadKnowledgeBases = useCallback(() => {
    if (!isGroup || kbLoaded) return;
    fetchKnowledgeBases();
  }, [fetchKnowledgeBases, isGroup, kbLoaded]);

  useEffect(() => {
    setMembers([]);
    setAgentMembers([]);
    setMentionTargetsLoaded(false);
    setKnowledgeBases([]);
    setKbLoaded(false);
    setKbVisible(false);
  }, [conversationId]);

  // Proactively load agent names when there's an active target (for the target bar display)
  useEffect(() => {
    if (isGroup && boundAgentId && !mentionTargetsLoaded) {
      loadMentionTargets();
    }
  }, [isGroup, boundAgentId, mentionTargetsLoaded, loadMentionTargets]);

  // Resolve the display name for the currently targeted agent
  const targetAgentName = useMemo(() => {
    if (!directAgentId) return null;
    // Group chat: check loaded group agents first, then fall back to global list
    const fromGroup = agentMembers.find((a) => a.agent_id === directAgentId)?.name;
    if (fromGroup) return fromGroup;
    const fromGlobal = globalAgents.find((a) => a.id === directAgentId)?.name;
    if (fromGlobal) return fromGlobal;
    // Direct agent chat: peer_name is always available
    if (!isGroup) return conversation?.peer_name ?? null;
    return null;
  }, [directAgentId, agentMembers, globalAgents, isGroup, conversation]);

  // Typing broadcast state
  const typingTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isTypingRef = useRef(false);
  const lastTypingSentRef = useRef(0);

  const sendTypingStart = useCallback(() => {
    const now = Date.now();
    if (now - lastTypingSentRef.current < 300) return;
    lastTypingSentRef.current = now;
    wsClient?.send(JSON.stringify({
      type: 'user.typing_start',
      data: { conversation_id: conversationId },
    }));
    isTypingRef.current = true;
  }, [wsClient, conversationId]);

  const sendTypingStop = useCallback(() => {
    if (!isTypingRef.current) return;
    wsClient?.send(JSON.stringify({
      type: 'user.typing_stop',
      data: { conversation_id: conversationId },
    }));
    isTypingRef.current = false;
  }, [wsClient, conversationId]);

  useEffect(() => {
    return () => {
      if (typingTimerRef.current) clearTimeout(typingTimerRef.current);
      sendTypingStop();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [conversationId]);

  const handleInputChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const val = e.target.value;
    setValue(val);
    sendTypingStart();
    if (typingTimerRef.current) clearTimeout(typingTimerRef.current);
    typingTimerRef.current = setTimeout(() => {
      sendTypingStop();
    }, 2000);

    // Detect @ mention and # KB reference triggers in group chats
    if (isGroup) {
      const el = e.target as HTMLTextAreaElement;
      const cursorPos = el.selectionStart;
      const textBeforeCursor = val.slice(0, cursorPos);

      // @ takes priority
      const atMatch = textBeforeCursor.match(/@(\S*)$/);
      if (atMatch) {
        const query = atMatch[1] ?? '';
        setMentionQuery(query);
        setMentionStart(cursorPos - query.length - 1); // position of @
        setMentionIndex(0);
        if (!mentionVisible) {
          setKbVisible(false);
          setMentionVisible(true);
          loadMentionTargets();
        }
        return;
      }

      // # KB reference (only when @ not matched)
      const hashMatch = textBeforeCursor.match(/#(\S*)$/);
      if (hashMatch) {
        const query = hashMatch[1] ?? '';
        setKbQuery(query);
        setKbStart(cursorPos - query.length - 1); // position of #
        setKbIndex(0);
        if (!kbVisible) {
          setMentionVisible(false);
          setKbVisible(true);
          loadKnowledgeBases();
        }
        return;
      }

      // Neither matched — close both
      if (mentionVisible) setMentionVisible(false);
      if (kbVisible) setKbVisible(false);
    }
  }, [sendTypingStart, sendTypingStop, isGroup, mentionVisible, kbVisible, loadMentionTargets, loadKnowledgeBases]);

  // 文件入库通用逻辑：校验大小 → 入 pendingFiles → 逐个上传。
  // input onChange 与拖拽 onDrop 共用，避免两份逻辑漂移。
  const processFiles = useCallback((files: FileList | File[]) => {
    const list = Array.from(files);
    if (list.length === 0) return;

    const newItems: PendingAttachment[] = [];
    list.forEach((f, i) => {
      if (f.size > MAX_FILE_SIZE) {
        message.error(`${f.name} 超过 50MB 限制`);
        return;
      }
      newItems.push({ uid: `${Date.now()}_${i}_${f.name}`, file: f, status: 'uploading' });
    });
    if (newItems.length === 0) return;
    setPendingFiles((prev) => [...prev, ...newItems]);

    // Upload each file
    newItems.forEach(async (item) => {
      try {
        const payload = await uploadFile(item.file);
        setPendingFiles((prev) =>
          prev.map((p) => (p.uid === item.uid ? { ...p, status: 'done', payload } : p)),
        );
      } catch {
        setPendingFiles((prev) =>
          prev.map((p) => (p.uid === item.uid ? { ...p, status: 'error', error: '上传失败' } : p)),
        );
      }
    });
  }, []);

  const handleFileSelect = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (!files) return;
    processFiles(files);
    // Reset input so same file can be re-selected
    if (fileInputRef.current) fileInputRef.current.value = '';
  }, [processFiles]);

  // 把 processFiles 注册给父级（ChatWindow），让整个聊天窗口的拖放复用同一上传逻辑。
  useEffect(() => {
    onRegisterProcessFiles?.(processFiles);
    return () => onRegisterProcessFiles?.(null);
  }, [onRegisterProcessFiles, processFiles]);

  const handleRemoveFile = useCallback((uid: string) => {
    setPendingFiles((prev) => prev.filter((p) => p.uid !== uid));
  }, []);

  const mentionTargets: MentionTarget[] = [
    ...members
      .filter((m) => !!m.username)
      .map((m) => {
        const label = m.username ?? 'unknown';
        return { id: m.user_id, label, mentionLabel: toMentionLabel(label), kind: 'user' as const, user: m };
      }),
    ...agentMembers.map((agent) => ({
      id: agent.agent_id,
      label: agent.name,
      mentionLabel: toMentionLabel(agent.name),
      kind: 'agent' as const,
      agent,
    })),
  ];

  const filteredTargets = mentionTargets.filter(
    (target) => (
      target.label.toLowerCase().includes(mentionQuery.toLowerCase()) ||
      target.mentionLabel.toLowerCase().includes(mentionQuery.toLowerCase())
    ),
  );

  const insertMention = useCallback((target: MentionTarget) => {
    const before = value.slice(0, mentionStart);
    const after = value.slice(mentionStart + mentionQuery.length + 1); // +1 for @
    // Append " #" to auto-trigger KB selection after choosing an agent
    const hashPos = mentionStart + target.mentionLabel.length + 2; // position of the new #
    const newValue = `${before}@${target.mentionLabel} #${after}`;
    setValue(newValue);
    setMentionVisible(false);

    // Auto-trigger KB selection
    setKbQuery('');
    setKbIndex(0);
    setKbStart(hashPos);
    setKbVisible(true);
    loadKnowledgeBases();

    // Focus back on textarea
    setTimeout(() => textareaRef.current?.focus(), 0);
  }, [value, mentionStart, mentionQuery, loadKnowledgeBases]);

  const hasMention = useCallback((content: string, label: string) => {
    const escaped = label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    return new RegExp(`(^|\\s)@${escaped}(?=\\s|$)`, 'i').test(content);
  }, []);

  // KB targets with dedup
  const kbTargets = useMemo(() => {
    const seen = new Set<string>();
    return knowledgeBases.filter((kb) => {
      const key = `${kb.username}/${kb.name}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  }, [knowledgeBases]);

  const filteredKBTargets = useMemo(() => {
    return kbTargets.filter((kb) =>
      kb.name.toLowerCase().includes(kbQuery.toLowerCase()) ||
      kb.username.toLowerCase().includes(kbQuery.toLowerCase())
    );
  }, [kbTargets, kbQuery]);

  const insertKB = useCallback((target: KBTarget | null) => {
    const before = value.slice(0, kbStart);
    const after = value.slice(kbStart + kbQuery.length + 1); // +1 for #
    if (target === null) {
      // "不使用知识库" — remove the # trigger text
      setValue(`${before}${after}`);
    } else {
      // Insert {{username/kbname}}
      const ref = `{{${target.username}/${target.kbName}}}`;
      setValue(`${before}${ref} ${after}`);
    }
    setKbVisible(false);
    setTimeout(() => textareaRef.current?.focus(), 0);
  }, [value, kbStart, kbQuery]);

  const handleSubmit = useCallback(async () => {
    const trimmed = value.trim();
    const attachments: AttachmentPayload[] = pendingFiles
      .filter((p) => p.status === 'done' && p.payload)
      .map((p) => p.payload!);

    if (!trimmed && !attachments.length) return;
    if (isStreaming) return;

    // Extract mentions from content for group chats
    let mentions: string[] | undefined;
    let mentionedAgentId: string | undefined;
    if (isGroup) {
      const targetLists = trimmed.includes('@') && !mentionTargetsLoaded
        ? await fetchMentionTargets()
        : { members, agentMembers };
      const userMentions = targetLists.members
        .filter((member) => member.username && hasMention(trimmed, toMentionLabel(member.username)))
        .map((member) => member.user_id);
      mentions = userMentions.length > 0 ? userMentions : undefined;
      mentionedAgentId = targetLists.agentMembers.find((agent) => hasMention(trimmed, toMentionLabel(agent.name)))?.agent_id;
    }

    setSending(true);
    if (typingTimerRef.current) clearTimeout(typingTimerRef.current);
    sendTypingStop();

    // 立即清空输入框，给用户即时反馈
    setValue('');
    setPendingFiles([]);
    onCancelReply?.();

    try {
      const replyPreview: ReplyToPreview | undefined = replyTo
        ? {
            id: replyTo.id,
            content: replyTo.content ?? '',
            sender_id: replyTo.sender_id,
            username: replyTo.username,
            deleted_at: null,
          }
        : undefined;
      // Group chats: don't pass agentId — routing handled by backend mention parsing.
      // Agent/single chats: pass the resolved agentId for direct dispatch.
      const targetAgentId = isGroup ? undefined : (mentionedAgentId ?? directAgentId);
      await send(
        trimmed,
        attachments.length ? attachments : undefined,
        replyTo?.id,
        replyPreview,
        mentions,
        targetAgentId,
      );
      // Persist the @mentioned agent as sticky target for subsequent messages
      if (isGroup && mentionedAgentId) {
        bindDirectAgentChat(conversationId, mentionedAgentId);
      }
    } catch {
      // 发送失败时恢复输入内容，方便用户重试
      setValue(trimmed);
      message.error('发送失败，请稍后重试');
    } finally {
      setSending(false);
    }
  }, [value, pendingFiles, isStreaming, send, sendTypingStop, replyTo, onCancelReply, isGroup, mentionTargetsLoaded, fetchMentionTargets, members, agentMembers, hasMention, directAgentId, bindDirectAgentChat, conversationId]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      // KB dropdown navigation (takes priority when visible)
      if (kbVisible) {
        const total = filteredKBTargets.length + 1; // +1 for "不使用知识库"
        if (e.key === 'ArrowDown') {
          e.preventDefault();
          setKbIndex((i) => (i + 1) % total);
          return;
        }
        if (e.key === 'ArrowUp') {
          e.preventDefault();
          setKbIndex((i) => (i - 1 + total) % total);
          return;
        }
        if (e.key === 'Enter' || e.key === 'Tab') {
          e.preventDefault();
          if (kbIndex === 0) {
            insertKB(null);
          } else {
            const target = filteredKBTargets[kbIndex - 1];
            if (target) insertKB({ username: target.username, kbName: target.name, kbId: target.id, visibility: target.visibility });
          }
          return;
        }
        if (e.key === 'Escape') {
          e.preventDefault();
          setKbVisible(false);
          return;
        }
      }

      // Mention dropdown navigation
      if (mentionVisible && filteredTargets.length > 0) {
        if (e.key === 'ArrowDown') {
          e.preventDefault();
          setMentionIndex((i) => (i + 1) % filteredTargets.length);
          return;
        }
        if (e.key === 'ArrowUp') {
          e.preventDefault();
          setMentionIndex((i) => (i - 1 + filteredTargets.length) % filteredTargets.length);
          return;
        }
        if (e.key === 'Enter' || e.key === 'Tab') {
          e.preventDefault();
          insertMention(filteredTargets[mentionIndex]!);
          return;
        }
        if (e.key === 'Escape') {
          e.preventDefault();
          setMentionVisible(false);
          return;
        }
      }

      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit, mentionVisible, kbVisible, filteredTargets, filteredKBTargets, mentionIndex, kbIndex, insertMention, insertKB],
  );

  const canSend = (value.trim() || pendingFiles.some((p) => p.status === 'done')) && !isStreaming;

  // 点击下拉列表外部关闭 mention 和 KB 下拉
  useEffect(() => {
    if (!mentionVisible && !kbVisible) return;
    const handleClickOutside = (e: MouseEvent) => {
      const target = e.target as HTMLElement;
      if (!target.closest(`.${styles.mentionDropdown}`)) {
        setMentionVisible(false);
        setKbVisible(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [mentionVisible, kbVisible]);

  return (
    <div className={styles.container}>
      {(isStreaming || agentTyping) && (
        <div className={styles.typingIndicator}>
          <Spin size="small" />
          <span>{targetAgentName ?? (conversation?.peer_name ?? 'Agent')} 正在思考...</span>
        </div>
      )}
      {isGroup && boundAgentId && (
        <div className={styles.targetBar}>
          <RobotOutlined className={styles.targetBarIcon} />
          <span className={styles.targetBarName}>{targetAgentName ?? 'Agent'}</span>
          <Tooltip title="停止@此智能体" mouseEnterDelay={0.8}>
            <Button
              type="text"
              size="small"
              icon={<CloseOutlined />}
              className={styles.targetBarClear}
              onClick={() => unbindDirectAgentChat(conversationId)}
            />
          </Tooltip>
        </div>
      )}
      {replyTo && (
        <div className={replyStyles.replyBar}>
          <div className={replyStyles.replyBarContent}>
            <div className={replyStyles.replyBarLabel}>
              回复 {replyTo.username || (replyTo.role === 'user' ? '用户' : '助手')}
            </div>
            <div className={replyStyles.replyBarText}>{truncatePreview(replyTo.content)}</div>
          </div>
          <Button
            type="text"
            size="small"
            icon={<CloseOutlined />}
            onClick={onCancelReply}
          />
        </div>
      )}
      <AttachmentPreview items={pendingFiles} onRemove={handleRemoveFile} />
      <div className={styles.inputRow}>
        <Tooltip title="添加附件">
          <Button
            type="text"
            icon={<LinkOutlined />}
            className={styles.attachBtn}
            onClick={() => fileInputRef.current?.click()}
          />
        </Tooltip>
        <input
          ref={fileInputRef}
          type="file"
          accept={ACCEPTED_TYPES}
          multiple
          onChange={handleFileSelect}
          className={styles.fileInput}
        />
        <TextArea
          ref={textareaRef}
          value={value}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          placeholder="发送至当前对话"
          autoSize={{ minRows: expanded ? 8 : 1, maxRows: expanded ? 20 : 4 }}
          className={styles.textarea}
        />
        <Tooltip title={expanded ? '收起输入框' : '展开输入框'}>
          <Button
            type="text"
            icon={expanded ? <DownOutlined /> : <UpOutlined />}
            className={styles.expandBtn}
            onClick={() => setExpanded(!expanded)}
          />
        </Tooltip>
        <Button
          type="primary"
          shape="default"
          icon={<SendOutlined />}
          onClick={handleSubmit}
          loading={sending}
          disabled={!canSend}
          className={styles.sendBtn}
        />
      </div>
      {mentionVisible && filteredTargets.length > 0 && (
        <div className={styles.mentionDropdown}>
          {filteredTargets.map((target, i) => (
            <button
              key={`${target.kind}-${target.id}`}
              className={`${styles.mentionItem} ${i === mentionIndex ? styles.mentionItemActive : ''}`}
              type="button"
              onMouseDown={(e) => {
                e.preventDefault();
                insertMention(target);
              }}
            >
              @{target.mentionLabel}
              {target.kind === 'agent' ? ' · Agent' : ''}
            </button>
          ))}
        </div>
      )}
      {kbVisible && (
        <div className={styles.mentionDropdown}>
          {/* "不使用知识库" option at index 0 */}
          <button
            className={`${styles.mentionItem} ${0 === kbIndex ? styles.mentionItemActive : ''}`}
            style={{ color: 'var(--color-text-tertiary)' }}
            type="button"
            onMouseDown={(e) => {
              e.preventDefault();
              insertKB(null);
            }}
          >
            不使用知识库
          </button>
          {filteredKBTargets.length > 0 ? (
            filteredKBTargets.map((kb, i) => {
              const idx = i + 1; // offset by 1 for "不使用" option
              return (
                <button
                  key={`${kb.username}/${kb.name}`}
                  className={`${styles.mentionItem} ${idx === kbIndex ? styles.mentionItemActive : ''}`}
                  type="button"
                  onMouseDown={(e) => {
                    e.preventDefault();
                    insertKB({ username: kb.username, kbName: kb.name, kbId: kb.id, visibility: kb.visibility });
                  }}
                >
                  <span style={{ fontWeight: 500 }}>{kb.username}/{kb.name}</span>
                  {kb.visibility === 'public' ? ' · 公开' : ' · 私有'}
                  <span style={{ color: 'var(--color-text-tertiary)', fontSize: 12, marginLeft: 6 }}>
                    ({kb.file_count} 个文件)
                  </span>
                </button>
              );
            })
          ) : (
            <div className={styles.mentionItem} style={{ color: 'var(--color-text-tertiary)', cursor: 'default' }}>
              {kbLoaded ? '没有可用的知识库' : '加载中...'}
            </div>
          )}
        </div>
      )}
    </div>
  );
};
