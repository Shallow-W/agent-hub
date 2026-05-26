import React, { useState, useCallback, useRef, useEffect } from 'react';
import { Input, Button, Tooltip, Spin } from 'antd';
import { SendOutlined, PaperClipOutlined } from '@ant-design/icons';
import { useMessages } from '@/hooks/useMessages';
import { useWsStore } from '@/store/wsStore';
import styles from './ChatInput.module.css';

const { TextArea } = Input;

interface ChatInputProps {
  conversationId: string;
}

export const ChatInput: React.FC<ChatInputProps> = ({ conversationId }) => {
  const [value, setValue] = useState('');
  const { send, streamingContent } = useMessages(conversationId);
  const isStreaming = (streamingContent ?? '').length > 0;
  const wsClient = useWsStore((s) => s.wsClient);

  // Typing broadcast state
  const typingTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isTypingRef = useRef(false);
  const lastTypingSentRef = useRef(0);

  const sendTypingStart = useCallback(() => {
    const now = Date.now();
    // Debounce: don't send more than once per 300ms
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

  // Cleanup on unmount or conversation change
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

    // Reset stop timer: send typing_stop after 2s of inactivity
    if (typingTimerRef.current) clearTimeout(typingTimerRef.current);
    typingTimerRef.current = setTimeout(() => {
      sendTypingStop();
    }, 2000);
  }, [sendTypingStart, sendTypingStop]);

  const handleSubmit = useCallback(async () => {
    const trimmed = value.trim();
    if (!trimmed || isStreaming) return;
    setValue('');
    // Stop typing indicator on send
    if (typingTimerRef.current) clearTimeout(typingTimerRef.current);
    sendTypingStop();
    await send(trimmed);
  }, [value, isStreaming, send, sendTypingStop]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit],
  );

  const charCount = value.length;

  return (
    <div className={styles.container}>
      {isStreaming && (
        <div className={styles.typingIndicator}>
          <Spin size="small" />
          <span>Agent 正在输入</span>
        </div>
      )}
      <div className={styles.inputRow}>
        <TextArea
          value={value}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
          autoSize={{ minRows: 1, maxRows: 4 }}
          className={styles.textarea}
        />
        <Button
          type="primary"
          shape="default"
          icon={<SendOutlined />}
          onClick={handleSubmit}
          disabled={!value.trim() || isStreaming}
          style={{ flexShrink: 0, height: 44, width: 44 }}
        />
      </div>
      <div className={styles.toolbar}>
        <Tooltip title="附件">
          <Button type="text" icon={<PaperClipOutlined />} size="small" />
        </Tooltip>
        <span
          className={`${styles.charCount} ${charCount > 0 ? styles.charCountVisible : ''}`}
        >
          {charCount}
        </span>
      </div>
    </div>
  );
};
