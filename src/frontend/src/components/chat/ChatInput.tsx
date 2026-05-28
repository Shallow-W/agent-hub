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
import { uploadFile } from '@/api/upload';
import type { AttachmentPayload } from '@/types/attachment';
import type { Message } from '@/types/message';
import { AttachmentPreview, type PendingAttachment } from './AttachmentPreview';
import styles from './ChatInput.module.css';
import replyStyles from './ChatInput.module.css';

const { TextArea } = Input;

const ACCEPTED_TYPES = '.jpg,.jpeg,.png,.gif,.webp,.pdf';
const MAX_FILE_SIZE = 50 * 1024 * 1024; // 50MB

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
    setValue(e.target.value);
    sendTypingStart();
    if (typingTimerRef.current) clearTimeout(typingTimerRef.current);
    typingTimerRef.current = setTimeout(() => {
      sendTypingStop();
    }, 2000);
  }, [sendTypingStart, sendTypingStop]);

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

  const handleSubmit = useCallback(async () => {
    const trimmed = value.trim();
    const attachments: AttachmentPayload[] = pendingFiles
      .filter((p) => p.status === 'done' && p.payload)
      .map((p) => p.payload!);

    if (!trimmed && !attachments.length) return;
    if (isStreaming) return;

    setSending(true);
    if (typingTimerRef.current) clearTimeout(typingTimerRef.current);
    sendTypingStop();
    try {
      await send(trimmed, attachments.length ? attachments : undefined, replyTo?.id);
      setValue('');
      setPendingFiles([]);
    } finally {
      setSending(false);
    }
  }, [value, pendingFiles, isStreaming, send, sendTypingStop, replyTo]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit],
  );

  const [sending, setSending] = useState(false);
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
    </div>
  );
};
