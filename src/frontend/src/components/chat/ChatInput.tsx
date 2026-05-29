import React, { useState, useCallback, useRef, useEffect } from 'react';
import { Input, Button, Tooltip, Spin, message } from 'antd';
import {
  CloseOutlined,
  LinkOutlined,
  SendOutlined,
  UpOutlined,
  DownOutlined,
} from '@ant-design/icons';
import { useMessages } from '@/hooks/useMessages';
import { useWsStore } from '@/store/wsStore';
import { useConversationStore } from '@/store/conversationStore';
import { uploadFile } from '@/api/upload';
import { getGroupMembers } from '@/api/group';
import { getConversationAgents } from '@/api/conversation';
import type { GroupMember } from '@/types/group';
import type { ConversationAgent } from '@/types/conversation';
import type { TextAreaRef } from 'antd/es/input/TextArea';
import type { AttachmentPayload } from '@/types/attachment';
import type { Message, ReplyToPreview } from '@/types/message';
import { AttachmentPreview, type PendingAttachment } from './AttachmentPreview';
import styles from './ChatInput.module.css';
import replyStyles from './ChatInput.module.css';

const { TextArea } = Input;

const ACCEPTED_TYPES = '.jpg,.jpeg,.png,.gif,.webp,.pdf';
const MAX_FILE_SIZE = 50 * 1024 * 1024; // 50MB

type MentionTarget =
  | { id: string; label: string; kind: 'user'; user: GroupMember }
  | { id: string; label: string; kind: 'agent'; agent: ConversationAgent };

interface ChatInputProps {
  conversationId: string;
  replyTo?: Message | null;
  onCancelReply?: () => void;
}

export const ChatInput: React.FC<ChatInputProps> = ({ conversationId, replyTo, onCancelReply }) => {
  const [expanded, setExpanded] = useState(false);
  const [value, setValue] = useState('');
  const [pendingFiles, setPendingFiles] = useState<PendingAttachment[]>([]);
  const { send, streamingContent } = useMessages(conversationId);
  const isStreaming = (streamingContent ?? '').length > 0;
  const wsClient = useWsStore((s) => s.wsClient);
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

  const conversation = useConversationStore((s) =>
    s.conversations.find((c) => c.id === conversationId),
  );
  const boundAgentId = useConversationStore((s) => s.directAgentChats[conversationId]);
  const directAgentId = conversation?.type === 'agent' ? conversation.peer_id : boundAgentId;
  const isGroup = conversation?.type === 'group';

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

  useEffect(() => {
    setMembers([]);
    setAgentMembers([]);
    setMentionTargetsLoaded(false);
  }, [conversationId]);

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
      data: { conversationId },
    }));
    isTypingRef.current = true;
  }, [wsClient, conversationId]);

  const sendTypingStop = useCallback(() => {
    if (!isTypingRef.current) return;
    wsClient?.send(JSON.stringify({
      type: 'user.typing_stop',
      data: { conversationId },
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

    // Detect @ mention trigger in group chats
    if (isGroup) {
      const el = e.target as HTMLTextAreaElement;
      const cursorPos = el.selectionStart;
      const textBeforeCursor = val.slice(0, cursorPos);
      const atMatch = textBeforeCursor.match(/@(\S*)$/);
      if (atMatch) {
        const query = atMatch[1] ?? '';
        setMentionQuery(query);
        setMentionStart(cursorPos - query.length - 1); // position of @
        setMentionIndex(0);
        if (!mentionVisible) {
          setMentionVisible(true);
          loadMentionTargets();
        }
      } else if (mentionVisible) {
        setMentionVisible(false);
      }
    }
  }, [sendTypingStart, sendTypingStop, isGroup, mentionVisible, loadMentionTargets]);

  const handleFileSelect = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (!files) return;

    const newItems: PendingAttachment[] = [];
    for (let i = 0; i < files.length; i++) {
      const f = files[i]!;
      if (f.size > MAX_FILE_SIZE) {
        message.error(`${f.name} 超过 50MB 限制`);
        continue;
      }
      newItems.push({ uid: `${Date.now()}_${i}`, file: f, status: 'uploading' });
    }
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

    // Reset input so same file can be re-selected
    if (fileInputRef.current) fileInputRef.current.value = '';
  }, []);

  const handleRemoveFile = useCallback((uid: string) => {
    setPendingFiles((prev) => prev.filter((p) => p.uid !== uid));
  }, []);

  const mentionTargets: MentionTarget[] = [
    ...members
      .filter((m) => !!m.username)
      .map((m) => ({ id: m.user_id, label: m.username ?? 'unknown', kind: 'user' as const, user: m })),
    ...agentMembers.map((agent) => ({ id: agent.agent_id, label: agent.name, kind: 'agent' as const, agent })),
  ];

  const filteredTargets = mentionTargets.filter(
    (target) => target.label.toLowerCase().includes(mentionQuery.toLowerCase()),
  );

  const insertMention = useCallback((target: MentionTarget) => {
    const before = value.slice(0, mentionStart);
    const after = value.slice(mentionStart + mentionQuery.length + 1); // +1 for @
    const newValue = `${before}@${target.label} ${after}`;
    setValue(newValue);
    setMentionVisible(false);
    // Focus back on textarea
    setTimeout(() => textareaRef.current?.focus(), 0);
  }, [value, mentionStart, mentionQuery]);

  const hasMention = useCallback((content: string, label: string) => {
    const escaped = label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    return new RegExp(`(^|\\s)@${escaped}(?=\\s|$)`, 'i').test(content);
  }, []);

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
        .filter((member) => member.username && hasMention(trimmed, member.username))
        .map((member) => member.user_id);
      mentions = userMentions.length > 0 ? userMentions : undefined;
      mentionedAgentId = targetLists.agentMembers.find((agent) => hasMention(trimmed, agent.name))?.agent_id;
    }

    setSending(true);
    if (typingTimerRef.current) clearTimeout(typingTimerRef.current);
    sendTypingStop();
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
      await send(
        trimmed,
        attachments.length ? attachments : undefined,
        replyTo?.id,
        replyPreview,
        mentions,
        mentionedAgentId ?? directAgentId,
      );
      setValue('');
      setPendingFiles([]);
      onCancelReply?.();
    } catch {
      message.error('发送失败，请稍后重试');
    } finally {
      setSending(false);
    }
  }, [value, pendingFiles, isStreaming, send, sendTypingStop, replyTo, onCancelReply, isGroup, mentionTargetsLoaded, fetchMentionTargets, members, agentMembers, hasMention, directAgentId]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
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
    [handleSubmit, mentionVisible, filteredTargets, mentionIndex, insertMention],
  );

  const canSend = (value.trim() || pendingFiles.some((p) => p.status === 'done')) && !isStreaming;

  return (
    <div className={styles.container}>
      {isStreaming && (
        <div className={styles.typingIndicator}>
          <Spin size="small" />
          <span>Agent 正在输入</span>
        </div>
      )}
      {replyTo && (
        <div className={replyStyles.replyBar}>
          <div className={replyStyles.replyBarContent}>
            <div className={replyStyles.replyBarLabel}>
              回复 {replyTo.username || (replyTo.role === 'user' ? '用户' : '助手')}
            </div>
            <div className={replyStyles.replyBarText}>{replyTo.content.length > 50 ? replyTo.content.slice(0, 50) + '...' : replyTo.content}</div>
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
              @{target.label}
              {target.kind === 'agent' ? ' · Agent' : ''}
            </button>
          ))}
        </div>
      )}
    </div>
  );
};
